// internal/api/router.go
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/logstream/internal/alerts"
	"github.com/logstream/internal/models"
	"github.com/logstream/internal/storage"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// API holds the dependencies for the HTTP handlers
type API struct {
	store       *storage.Store
	engine      *alerts.Engine
	hub         *Hub
	wsJWTSecret []byte // signs the short-lived WebSocket tokens (H1)
}

// NewRouter constructs the chi router, mounts middleware, and registers all endpoints
func NewRouter(store *storage.Store, ingestHandler http.Handler, cfg Config, engine *alerts.Engine, hub *Hub) http.Handler {
	r := chi.NewRouter()
	api := &API{store: store, engine: engine, hub: hub, wsJWTSecret: cfg.WSJWTSecret}

	rateLimit := RateLimit(cfg.TrustedProxies)

	// Global Middleware
	r.Use(RequestID)
	r.Use(Recoverer)
	r.Use(Logger)
	r.Use(CORS(cfg.AllowedOrigins))

	// Public Routes
	r.Get("/health", api.healthHandler)
	r.Handle("/metrics", promhttp.Handler()) // FEATURE L: Exposed Prometheus endpoint

	// Ingestion endpoint — authenticated with the ingest key and rate limited.
	// (C1) Previously this was fully public and unthrottled.
	r.Group(func(r chi.Router) {
		r.Use(rateLimit)
		r.Use(APIKeyAuth(cfg.IngestKey))
		r.Post("/ingest", ingestHandler.ServeHTTP)
	})

	// WebSocket Route (Public, but requires valid JWT token via query param)
	r.Get("/ws/tail", func(w http.ResponseWriter, r *http.Request) {
		api.hub.ServeWS(w, r)
	})

	// Protected API Routes
	r.Route("/api", func(r chi.Router) {
		r.Use(APIKeyAuth(cfg.APIKey))
		r.Use(rateLimit) // per-client token bucket

		// FEATURE K: JWT Dispenser for frontend WebSocket connections
		r.Get("/ws-token", api.wsTokenHandler)

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
		slog.Error("failed to encode JSON response", slog.Any("err", err))
	}
}

// defaultSearchWindow bounds an otherwise-unfiltered log search so it cannot
// scan the entire table / every partition (M2).
const defaultSearchWindow = 30 * 24 * time.Hour

// alert-rule validation limits (M5)
const (
	maxRulePatternLen = 500
	maxRuleNameLen    = 200
	maxCooldownMin    = 7 * 24 * 60 // 1 week
	maxActiveRules    = 500
)

func (a *API) healthHandler(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Ping(r.Context()); err != nil {
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degraded",
			"error":  "database unreachable",
		})
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "logstream"})
}

// validateRule enforces the alert-rule limits shared by create and update (M5).
func validateRule(rule *models.AlertRule) string {
	rule.Name = strings.TrimSpace(rule.Name)
	if rule.Name == "" || rule.Pattern == "" {
		return "name and pattern are required"
	}
	if len(rule.Name) > maxRuleNameLen {
		return fmt.Sprintf("name too long (max %d)", maxRuleNameLen)
	}
	if len(rule.Pattern) > maxRulePatternLen {
		return fmt.Sprintf("pattern too long (max %d)", maxRulePatternLen)
	}
	if _, err := regexp.Compile(rule.Pattern); err != nil {
		return fmt.Sprintf("invalid regex pattern: %v", err)
	}
	if rule.CooldownMinutes < 0 || rule.CooldownMinutes > maxCooldownMin {
		return fmt.Sprintf("cooldown_minutes must be between 0 and %d", maxCooldownMin)
	}
	return ""
}

// wsTokenHandler generates a short-lived JWT for the React UI to use when opening the WebSocket
func (a *API) wsTokenHandler(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(now.Add(60 * time.Second)), // Strict 60-second validity
		IssuedAt:  jwt.NewNumericDate(now),
	})

	// Dedicated WS signing secret, decoupled from the API key (H1).
	signed, err := token.SignedString(a.wsJWTSecret)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"token": signed})
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

	// Bound an unfiltered search to a default window so it cannot scan the whole
	// table (M2). Callers wanting older data pass an explicit 'from'.
	if from.IsZero() {
		from = time.Now().Add(-defaultSearchWindow)
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

	// Optional ?ts= hint (RFC3339) lets Postgres prune to one partition (M2).
	var tsHint time.Time
	if tsStr := r.URL.Query().Get("ts"); tsStr != "" {
		if p, perr := time.Parse(time.RFC3339, tsStr); perr == nil {
			tsHint = p
		}
	}

	entry, err := a.store.GetLog(r.Context(), id, tsHint)
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
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	defer r.Body.Close()

	var rule models.AlertRule
	if err := json.Unmarshal(body, &rule); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	// A plain bool can't tell "is_active:false" from omitted; default new rules
	// to active (matching the DB column default) unless explicitly disabled.
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(body, &fields)
	if _, ok := fields["is_active"]; !ok {
		rule.IsActive = true
	}

	if msg := validateRule(&rule); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	if n, err := a.store.CountRules(r.Context()); err == nil && n >= maxActiveRules {
		http.Error(w, fmt.Sprintf("rule limit reached (max %d)", maxActiveRules), http.StatusConflict)
		return
	}

	created, err := a.store.CreateRule(r.Context(), rule)
	if err != nil {
		http.Error(w, "Failed to create rule", http.StatusInternalServerError)
		return
	}

	// Hot-reload the engine cache
	if err := a.engine.LoadRules(r.Context()); err != nil {
		slog.ErrorContext(r.Context(), "failed to reload alert rules", slog.Any("err", err))
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

	if msg := validateRule(&rule); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	updated, err := a.store.UpdateRule(r.Context(), id, rule)
	if err != nil {
		http.Error(w, "Failed to update rule", http.StatusInternalServerError)
		return
	}

	if err := a.engine.LoadRules(r.Context()); err != nil {
		slog.ErrorContext(r.Context(), "failed to reload alert rules", slog.Any("err", err))
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
		slog.ErrorContext(r.Context(), "failed to reload alert rules", slog.Any("err", err))
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

	limit := 50 // Standardized to 50 for pagination
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 {
		limit = l
	}

	offset := 0
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	ruleID := r.URL.Query().Get("rule_id")
	q := r.URL.Query().Get("q")
	service := r.URL.Query().Get("service")
	level := r.URL.Query().Get("level")

	alertsList, total, err := a.store.ListAlerts(r.Context(), ruleID, q, service, level, from, to, limit, offset)
	if err != nil {
		http.Error(w, "Failed to list alerts", http.StatusInternalServerError)
		return
	}
	if alertsList == nil {
		alertsList = []models.AlertWithContext{}
	}

	// Wrap the response so the frontend knows the true total!
	response := map[string]interface{}{
		"alerts": alertsList,
		"total":  total,
	}

	respondJSON(w, http.StatusOK, response)
}
