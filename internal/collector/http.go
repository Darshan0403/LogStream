// internal/collector/http.go
package collector

import (
	"fmt"
	"io"
	"net/http"

	"github.com/logstream/internal/parser"
)

type HTTPHandler struct {
	batcher *Batcher
	parser  parser.LogParser
}

func NewHTTPHandler(b *Batcher, p parser.LogParser) *HTTPHandler {
	return &HTTPHandler{
		batcher: b,
		parser:  p,
	}
}

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, err.Error(), http.StatusBadRequest)
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
