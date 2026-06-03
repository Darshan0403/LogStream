// internal/collector/http.go
package collector

import (
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

	entry, err := h.parser.Parse(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Push to the channel. This is non-blocking as long as the 10K buffer isn't full.
	h.batcher.Send(entry)

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}
