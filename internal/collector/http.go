// internal/collector/http.go
package collector

import (
	"fmt"
	"io"
	"net/http"

	"github.com/logstream/internal/parser"
)

type HTTPHandler struct {
	batcher       *Batcher
	parser        parser.LogParser
	ingestEnabled bool
}

func NewHTTPHandler(b *Batcher, p parser.LogParser, ingestEnabled bool) *HTTPHandler {
	return &HTTPHandler{
		batcher:       b,
		parser:        p,
		ingestEnabled: ingestEnabled,
	}
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// INGEST_ENABLED=false → read-only deployment mode
	if !h.ingestEnabled {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"ingestion disabled — this is a read-only deployment"}`))
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// ParseBatch handles both single JSON objects and arrays
	entries, err := h.parser.ParseBatch(body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse log payload: %v", err), http.StatusBadRequest)
		return
	}

	// Push each entry to the batcher channel
	for _, entry := range entries {
		h.batcher.Send(entry)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, `{"status":"accepted","ingested":%d}`, len(entries))
}
