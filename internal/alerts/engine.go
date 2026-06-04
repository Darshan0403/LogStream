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
	store      *storage.Store
	cache      map[uuid.UUID]*compiledRule
	cacheMu    sync.RWMutex // Protects the regex cache
	lastFired  map[uuid.UUID]time.Time
	cooldownMu sync.Mutex // Protects the lastFired map
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

	e.cacheMu.Lock()
	e.cache = newCache
	e.cacheMu.Unlock()

	fmt.Printf("Alert Engine: Loaded %d active rules.\n", len(newCache))
	return nil
}

func (e *Engine) Check(ctx context.Context, batch []models.LogEntry) {
	e.cacheMu.RLock()
	defer e.cacheMu.RUnlock()

	now := time.Now()

	for _, cr := range e.cache {
		for _, log := range batch {
			if cr.rule.LevelFilter != nil && *cr.rule.LevelFilter != log.Level {
				continue
			}
			if cr.rule.ServiceFilter != nil && *cr.rule.ServiceFilter != log.Service {
				continue
			}
			if !cr.regex.MatchString(log.Message) {
				continue
			}

			// Lock the cooldown map for reading and writing
			e.cooldownMu.Lock()
			lastTime := e.lastFired[cr.rule.ID]
			cooldownDuration := time.Duration(cr.rule.CooldownMinutes) * time.Minute

			if now.Sub(lastTime) < cooldownDuration {
				e.cooldownMu.Unlock()
				continue
			}

			// Fire Alert and update cooldown safely
			if err := e.store.CreateAlert(ctx, cr.rule.ID, log.ID, log.Timestamp); err != nil {
				fmt.Printf("ERROR: Failed to save alert for rule '%s': %v\n", cr.rule.Name, err)
				e.cooldownMu.Unlock()
				continue
			}

			e.lastFired[cr.rule.ID] = now
			e.cooldownMu.Unlock()

			fmt.Printf("🔔 ALERT [%s]: '%s' matched log #%d from %s\n", cr.rule.Name, cr.rule.Pattern, log.ID, log.Service)
			break
		}
	}
}
