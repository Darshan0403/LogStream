// internal/collector/http.go
package collector

import (
	"fmt"
	"io"
	"net/http"

	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/logstream/internal/parser"
	"github.com/prometheus/client_golang/prometheus"
)

var LogsIngestedTotal = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "logstream_logs_ingested_total",
	Help: "Total number of successfully ingested logs",
})

func init() {
	prometheus.MustRegister(LogsIngestedTotal)
}

type HTTPHandler struct {
	batcher       *Batcher
	parser        parser.LogParser
	ingestEnabled bool
	idempCache    *lru.Cache[string, struct{}]
}

func NewHTTPHandler(b *Batcher, p parser.LogParser, ingestEnabled bool) *HTTPHandler {
	// 10,000 capacity HashiCorp LRU cache
	cache, _ := lru.New[string, struct{}](10000)

	return &HTTPHandler{
		batcher:       b,
		parser:        p,
		ingestEnabled: ingestEnabled,
		idempCache:    cache,
	}
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.ingestEnabled {
		http.Error(w, `{"error":"ingestion disabled"}`, http.StatusForbidden)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. CHECK & LOCK THE IDEMPOTENCY KEY INSTANTLY
	idempKey := r.Header.Get("X-Idempotency-Key")
	if idempKey != "" {
		// ContainsOrAdd checks if it exists, AND adds it atomically in one step!
		// This prevents the race condition when double-curling rapidly.
		if exists, _ := h.idempCache.ContainsOrAdd(idempKey, struct{}{}); exists {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprintf(w, `{"status":"accepted","ingested":0,"note":"duplicate_ignored"}`)
			return
		}
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		// If payload fails, remove the key so it can be retried properly
		if idempKey != "" {
			h.idempCache.Remove(idempKey)
		}
		http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	defer r.Body.Close()

	entries, err := h.parser.ParseBatch(body)
	if err != nil {
		if idempKey != "" {
			h.idempCache.Remove(idempKey)
		}
		http.Error(w, fmt.Sprintf("Failed to parse: %v", err), http.StatusBadRequest)
		return
	}

	if len(entries) > 1000 {
		if idempKey != "" {
			h.idempCache.Remove(idempKey)
		}
		http.Error(w, "max 1000 logs per batch", http.StatusBadRequest)
		return
	}

	for _, entry := range entries {
		if err := h.batcher.Send(entry); err != nil {
			if idempKey != "" {
				h.idempCache.Remove(idempKey)
			}
			w.Header().Set("Retry-After", "1")
			http.Error(w, "server busy", http.StatusServiceUnavailable)
			return
		}
	}

	LogsIngestedTotal.Add(float64(len(entries)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"status":"accepted","ingested":%d}`, len(entries))
}
