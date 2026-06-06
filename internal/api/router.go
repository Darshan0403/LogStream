// internal/api/router.go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/logstream/internal/alerts"
	"github.com/logstream/internal/models"
	"github.com/logstream/internal/storage"
)

// API holds the dependencies for the HTTP handlers
type API struct {
	store  *storage.Store
	engine *alerts.Engine
	hub    *Hub   // NEW
	apiKey string // NEW: Storing apiKey here so the websocket handler can validate it
}

// NewRouter constructs the chi router, mounts middleware, and registers all endpoints
func NewRouter(store *storage.Store, ingestHandler http.Handler, apiKey string, engine *alerts.Engine, hub *Hub) http.Handler {
	r := chi.NewRouter()
	api := &API{store: store, engine: engine, hub: hub, apiKey: apiKey}

	// Global Middleware
	r.Use(Recoverer)
	r.Use(Logger)
	r.Use(CORS)

	// Public Routes
	r.Get("/health", api.healthHandler)
	r.With(RateLimit).Post("/ingest", ingestHandler.ServeHTTP)

	// NEW: WebSocket Route (Public, validates via query param)
	r.Get("/ws/tail", func(w http.ResponseWriter, r *http.Request) {
		api.hub.ServeWS(w, r, api.apiKey)
	})

	// Protected API Routes
	r.Route("/api", func(r chi.Router) {
		r.Use(APIKeyAuth(apiKey))

		r.Get("/logs", api.searchHandler)
		r.Get("/logs/{id}", api.getLogHandler)
		r.Get("/logs/stats", api.statsHandler)
		r.Get("/services", api.servicesHandler)
		r.Post("/rules", api.createRuleHandler)
		r.Get("/rules", api.listRulesHandler)
		r.Put("/rules/{id}", api.updateRuleHandler)
		r.Delete("/rules/{id}", api.deleteRuleHandler)
		r.Get("/alerts", api.listAlertsHandler)
	})

	return r
}

// Helper function to send standard JSON responses
func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		fmt.Printf("Failed to encode JSON response: %v\n", err)
	}
}

func (a *API) healthHandler(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "logstream"})
}

func (a *API) searchHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	service := r.URL.Query().Get("service")
	level := r.URL.Query().Get("level")

	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
		if limit > 200 {
			limit = 200 // Max cap
		}
	}

	offset := 0
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	var from, to time.Time
	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		var err error
		from, err = time.Parse(time.RFC3339, fromStr)
		if err != nil {
			http.Error(w, "Invalid 'from' timestamp. Must be RFC3339 format", http.StatusBadRequest)
			return
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		var err error
		to, err = time.Parse(time.RFC3339, toStr)
		if err != nil {
			http.Error(w, "Invalid 'to' timestamp. Must be RFC3339 format", http.StatusBadRequest)
			return
		}
	}

	logs, total, err := a.store.Search(r.Context(), q, service, level, from, to, limit, offset)
	if err != nil {
		http.Error(w, "Failed to search logs", http.StatusInternalServerError)
		return
	}

	// Null slice protection for empty JSON arrays
	if logs == nil {
		logs = []models.LogEntry{} // Prevent returning `null`
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"logs":   logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

func (a *API) getLogHandler(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idParam, 10, 64)
	if err != nil {
		http.Error(w, "Invalid log ID", http.StatusBadRequest)
		return
	}

	entry, err := a.store.GetLog(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	respondJSON(w, http.StatusOK, entry)
}

func (a *API) statsHandler(w http.ResponseWriter, r *http.Request) {
	to := time.Now()
	from := to.Add(-24 * time.Hour) // Default to last 24 hours

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if p, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = p
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if p, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = p
		}
	}

	stats, err := a.store.Stats(r.Context(), from, to)
	if err != nil {
		http.Error(w, "Failed to aggregate stats", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, stats)
}

func (a *API) servicesHandler(w http.ResponseWriter, r *http.Request) {
	services, err := a.store.ListServices(r.Context())
	if err != nil {
		http.Error(w, "Failed to list services", http.StatusInternalServerError)
		return
	}

	if services == nil {
		services = []string{} // Prevent returning `null`
	}

	respondJSON(w, http.StatusOK, services)
}

// --- Alert Handlers ---

func (a *API) createRuleHandler(w http.ResponseWriter, r *http.Request) {
	var rule models.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if rule.Name == "" || rule.Pattern == "" {
		http.Error(w, "name and pattern are required", http.StatusBadRequest)
		return
	}

	if _, err := regexp.Compile(rule.Pattern); err != nil {
		http.Error(w, fmt.Sprintf("invalid regex pattern: %v", err), http.StatusBadRequest)
		return
	}

	created, err := a.store.CreateRule(r.Context(), rule)
	if err != nil {
		http.Error(w, "Failed to create rule", http.StatusInternalServerError)
		return
	}

	// Hot-reload the engine cache
	if err := a.engine.LoadRules(r.Context()); err != nil {
		fmt.Printf("WARNING: Failed to reload alert rules: %v\n", err)
	}
	respondJSON(w, http.StatusCreated, created)
}

func (a *API) listRulesHandler(w http.ResponseWriter, r *http.Request) {
	rules, err := a.store.ListRules(r.Context())
	if err != nil {
		http.Error(w, "Failed to list rules", http.StatusInternalServerError)
		return
	}
	if rules == nil {
		rules = []models.AlertRule{}
	}
	respondJSON(w, http.StatusOK, rules)
}

func (a *API) updateRuleHandler(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		http.Error(w, "Invalid rule ID", http.StatusBadRequest)
		return
	}

	var rule models.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if _, err := regexp.Compile(rule.Pattern); err != nil {
		http.Error(w, fmt.Sprintf("invalid regex pattern: %v", err), http.StatusBadRequest)
		return
	}

	updated, err := a.store.UpdateRule(r.Context(), id, rule)
	if err != nil {
		http.Error(w, "Failed to update rule", http.StatusInternalServerError)
		return
	}

	if err := a.engine.LoadRules(r.Context()); err != nil {
		fmt.Printf("WARNING: Failed to reload alert rules: %v\n", err)
	}
	respondJSON(w, http.StatusOK, updated)
}

func (a *API) deleteRuleHandler(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")
	id, err := uuid.Parse(idParam)
	if err != nil {
		http.Error(w, "Invalid rule ID", http.StatusBadRequest)
		return
	}

	if err := a.store.DeleteRule(r.Context(), id); err != nil {
		http.Error(w, "Failed to delete rule", http.StatusInternalServerError)
		return
	}

	if err := a.engine.LoadRules(r.Context()); err != nil {
		fmt.Printf("WARNING: Failed to reload alert rules: %v\n", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listAlertsHandler(w http.ResponseWriter, r *http.Request) {
	to := time.Now()
	from := to.Add(-24 * time.Hour)

	if fromStr := r.URL.Query().Get("from"); fromStr != "" {
		if p, err := time.Parse(time.RFC3339, fromStr); err == nil {
			from = p
		}
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		if p, err := time.Parse(time.RFC3339, toStr); err == nil {
			to = p
		}
	}

	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}

	var ruleIDFilter *uuid.UUID
	if ruleIDStr := r.URL.Query().Get("rule_id"); ruleIDStr != "" {
		if id, err := uuid.Parse(ruleIDStr); err == nil {
			ruleIDFilter = &id
		}
	}

	alertsList, err := a.store.ListAlerts(r.Context(), ruleIDFilter, from, to, limit)
	if err != nil {
		http.Error(w, "Failed to list alerts", http.StatusInternalServerError)
		return
	}
	if alertsList == nil {
		alertsList = []models.AlertWithContext{}
	}

	respondJSON(w, http.StatusOK, alertsList)
}
