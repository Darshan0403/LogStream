// internal/api/router.go
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/logstream/internal/models"
	"github.com/logstream/internal/storage"
)

// API holds the dependencies for the HTTP handlers
type API struct {
	store *storage.Store
}

// NewRouter constructs the chi router, mounts middleware, and registers all endpoints
func NewRouter(store *storage.Store, ingestHandler http.Handler, apiKey string) http.Handler {
	r := chi.NewRouter()
	api := &API{store: store}

	// Global Middleware
	r.Use(Recoverer)
	r.Use(Logger)
	r.Use(CORS)

	// Public Routes
	r.Get("/health", api.healthHandler)
	r.Post("/ingest", ingestHandler.ServeHTTP)

	// Protected API Routes
	r.Route("/api", func(r chi.Router) {
		r.Use(APIKeyAuth(apiKey))

		r.Get("/logs", api.searchHandler)
		r.Get("/logs/{id}", api.getLogHandler)
		r.Get("/logs/stats", api.statsHandler)
		r.Get("/services", api.servicesHandler)
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
		from, _ = time.Parse(time.RFC3339, fromStr)
	}
	if toStr := r.URL.Query().Get("to"); toStr != "" {
		to, _ = time.Parse(time.RFC3339, toStr)
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
