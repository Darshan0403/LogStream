// internal/alerts/engine.go
package alerts

import (
	"context"
	"log/slog"
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
			slog.WarnContext(ctx, "alert rule regex failed to compile", slog.String("rule", r.Name), slog.Any("err", err))
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

	slog.InfoContext(ctx, "alert engine reloaded", slog.Int("active_rules", len(newCache)))
	return nil
}

func (e *Engine) Check(ctx context.Context, batch []models.LogEntry) {
	// Snapshot the compiled rules under a short lock, then evaluate without it so
	// regex matching never blocks a LoadRules() swap and vice versa (M6).
	// compiledRule values are immutable — LoadRules builds a fresh map each time.
	e.cacheMu.RLock()
	rules := make([]*compiledRule, 0, len(e.cache))
	for _, cr := range e.cache {
		rules = append(rules, cr)
	}
	e.cacheMu.RUnlock()

	now := time.Now()

	for _, cr := range rules {
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
				slog.ErrorContext(ctx, "failed to save fired alert", slog.String("rule", cr.rule.Name), slog.Any("err", err))
				e.cooldownMu.Unlock()
				continue
			}

			e.lastFired[cr.rule.ID] = now
			e.cooldownMu.Unlock()

			slog.InfoContext(ctx, "alert fired", slog.String("rule", cr.rule.Name), slog.String("pattern", cr.rule.Pattern), slog.Int64("log_id", log.ID), slog.String("service", log.Service))
			break
		}
	}
}
