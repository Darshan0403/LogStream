// internal/collector/http.go
package collector

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/logstream/internal/parser"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	maxBatchBytes   = 10 << 20 // 10 MiB request body
	maxBatchEntries = 1000
	// maxMetadataBytes bounds the JSONB blob a client can attach to a single log
	// entry, so 1000 entries can't smuggle a huge payload past the body cap (L12).
	maxMetadataBytes = 16 << 10         // 16 KiB
	idempTTL         = 10 * time.Minute // idempotency keys expire (L9)
	idempCapacity    = 10000
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

	idempMu    sync.Mutex
	idempCache *expirable.LRU[string, struct{}]
}

func NewHTTPHandler(b *Batcher, p parser.LogParser, ingestEnabled bool) *HTTPHandler {
	return &HTTPHandler{
		batcher:       b,
		parser:        p,
		ingestEnabled: ingestEnabled,
		idempCache:    expirable.NewLRU[string, struct{}](idempCapacity, nil, idempTTL),
	}
}

// seenIdempKey atomically reports whether key was already seen, recording it if not.
func (h *HTTPHandler) seenIdempKey(key string) bool {
	if key == "" {
		return false
	}
	h.idempMu.Lock()
	defer h.idempMu.Unlock()
	if h.idempCache.Contains(key) {
		return true
	}
	h.idempCache.Add(key, struct{}{})
	return false
}

func (h *HTTPHandler) releaseIdempKey(key string) {
	if key == "" {
		return
	}
	h.idempMu.Lock()
	h.idempCache.Remove(key)
	h.idempMu.Unlock()
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !h.ingestEnabled {
		writeJSONError(w, http.StatusForbidden, "ingestion disabled")
		return
	}

	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Require a JSON content type when one is provided (L7). An empty header is
	// tolerated for minimal clients / curl.
	if ct := r.Header.Get("Content-Type"); ct != "" {
		if mt, _, _ := strings.Cut(ct, ";"); strings.TrimSpace(mt) != "application/json" {
			writeJSONError(w, http.StatusUnsupportedMediaType, "expected application/json")
			return
		}
	}

	idempKey := r.Header.Get("X-Idempotency-Key")
	if h.seenIdempKey(idempKey) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"status":"accepted","ingested":0,"note":"duplicate_ignored"}`)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBatchBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.releaseIdempKey(idempKey)
		writeJSONError(w, http.StatusRequestEntityTooLarge, "payload too large")
		return
	}
	defer r.Body.Close()

	entries, err := h.parser.ParseBatch(body)
	if err != nil {
		h.releaseIdempKey(idempKey)
		// Don't echo the raw parser error back to the client (L7).
		slog.WarnContext(r.Context(), "ingest parse failure", slog.String("remote", r.RemoteAddr), slog.Any("err", err))
		writeJSONError(w, http.StatusBadRequest, "invalid log payload")
		return
	}

	if len(entries) > maxBatchEntries {
		h.releaseIdempKey(idempKey)
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("max %d logs per batch", maxBatchEntries))
		return
	}

	for i := range entries {
		if n := metadataSize(entries[i].Metadata); n > maxMetadataBytes {
			h.releaseIdempKey(idempKey)
			writeJSONError(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("entry %d metadata too large (%d bytes, max %d)", i, n, maxMetadataBytes))
			return
		}
	}

	for _, entry := range entries {
		if err := h.batcher.Send(entry); err != nil {
			h.releaseIdempKey(idempKey)
			w.Header().Set("Retry-After", "1")
			writeJSONError(w, http.StatusServiceUnavailable, "server busy")
			return
		}
	}

	LogsIngestedTotal.Add(float64(len(entries)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"status":"accepted","ingested":%d}`, len(entries))
}

func metadataSize(m map[string]any) int {
	if len(m) == 0 {
		return 0
	}
	b, err := json.Marshal(m)
	if err != nil {
		return maxMetadataBytes + 1 // unserialisable → reject
	}
	return len(b)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	b, _ := json.Marshal(map[string]string{"error": msg})
	w.Write(b)
}
