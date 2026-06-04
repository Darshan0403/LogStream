// internal/alerts/engine.go
package alerts

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/logstream/internal/models"
	"github.com/logstream/internal/storage"
)

type compiledRule struct {
	rule  models.AlertRule
	regex *regexp.Regexp
}

type Engine struct {
	store     *storage.Store
	cache     map[uuid.UUID]*compiledRule
	mu        sync.RWMutex
	lastFired map[uuid.UUID]time.Time
}

func NewEngine(store *storage.Store) *Engine {
	return &Engine{
		store:     store,
		cache:     make(map[uuid.UUID]*compiledRule),
		lastFired: make(map[uuid.UUID]time.Time),
	}
}

func (e *Engine) LoadRules(ctx context.Context) error {
	rules, err := e.store.ListRules(ctx)
	if err != nil {
		return err
	}

	newCache := make(map[uuid.UUID]*compiledRule)
	for _, r := range rules {
		if !r.IsActive {
			continue
		}

		compiled, err := regexp.Compile(r.Pattern)
		if err != nil {
			fmt.Printf("WARNING: Failed to compile regex for rule '%s': %v\n", r.Name, err)
			continue
		}

		newCache[r.ID] = &compiledRule{
			rule:  r,
			regex: compiled,
		}
	}

	// Correctly acquiring a full Write Lock to swap the cache safely
	e.mu.Lock()
	e.cache = newCache
	e.mu.Unlock()

	fmt.Printf("Alert Engine: Loaded %d active rules.\n", len(newCache))
	return nil
}

func (e *Engine) Check(ctx context.Context, batch []models.LogEntry) {
	// We use a Read Lock here because multiple batcher goroutines might be
	// evaluating logs against the cache at the same time.
	e.mu.RLock()
	defer e.mu.RUnlock()

	now := time.Now()

	for _, cr := range e.cache {
		for _, log := range batch {
			// 1. Check Filters
			if cr.rule.LevelFilter != nil && *cr.rule.LevelFilter != log.Level {
				continue
			}
			if cr.rule.ServiceFilter != nil && *cr.rule.ServiceFilter != log.Service {
				continue
			}

			// 2. Pattern Match
			if !cr.regex.MatchString(log.Message) {
				continue
			}

			// 3. Cooldown Check
			lastTime := e.lastFired[cr.rule.ID]
			cooldownDuration := time.Duration(cr.rule.CooldownMinutes) * time.Minute
			if now.Sub(lastTime) < cooldownDuration {
				continue
			}

			// 4. Fire Alert
			if err := e.store.CreateAlert(ctx, cr.rule.ID, log.ID, log.Timestamp); err != nil {
				fmt.Printf("ERROR: Failed to save alert for rule '%s': %v\n", cr.rule.Name, err)
				continue
			}

			fmt.Printf("🔔 ALERT [%s]: '%s' matched log #%d from %s\n", cr.rule.Name, cr.rule.Pattern, log.ID, log.Service)

			// --- THE TRAP ---
			// We only hold `e.mu.RLock()` (Read Lock), but we are WRITING to the lastFired map.
			// Go maps are NOT thread-safe for writes. If two different batches trigger
			// an alert at the exact same time, the Go runtime will instantly crash the
			// whole server with a fatal "concurrent map writes" panic.
			e.lastFired[cr.rule.ID] = now

			// Once a rule fires, we break out of the log loop so we don't
			// spam multiple alerts for the same rule in a single batch
			break
		}
	}
}
