package collector

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/logstream/internal/models"
	"github.com/logstream/internal/parser"
)

// a Batcher that accepts sends without running any workers or touching a DB.
func drainBatcher() *Batcher {
	return &Batcher{ch: make(chan models.LogEntry, 4096)}
}

func do(h *HTTPHandler, method, body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/ingest", strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestIngest_ContentType(t *testing.T) {
	h := NewHTTPHandler(drainBatcher(), &parser.JSONParser{}, true)

	if rr := do(h, "POST", `[{"message":"x"}]`, map[string]string{"Content-Type": "text/plain"}); rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("text/plain: got %d", rr.Code)
	}
	if rr := do(h, "POST", `[{"message":"x"}]`, map[string]string{"Content-Type": "application/json; charset=utf-8"}); rr.Code != http.StatusAccepted {
		t.Fatalf("json+charset: got %d", rr.Code)
	}
	if rr := do(h, "POST", `[{"message":"x"}]`, nil); rr.Code != http.StatusAccepted {
		t.Fatalf("no content-type: got %d", rr.Code)
	}
}

func TestIngest_GenericParseError(t *testing.T) {
	h := NewHTTPHandler(drainBatcher(), &parser.JSONParser{}, true)
	rr := do(h, "POST", `{not json`, map[string]string{"Content-Type": "application/json"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("got %d", rr.Code)
	}
	if strings.Contains(strings.ToLower(rr.Body.String()), "unmarshal") || strings.Contains(rr.Body.String(), "not json") {
		t.Fatalf("raw parser error leaked: %s", rr.Body.String())
	}
}

func TestIngest_MetadataCap(t *testing.T) {
	h := NewHTTPHandler(drainBatcher(), &parser.JSONParser{}, true)
	big := strings.Repeat("A", maxMetadataBytes+100)
	body := `[{"message":"x","metadata":{"blob":"` + big + `"}}]`
	rr := do(h, "POST", body, map[string]string{"Content-Type": "application/json"})
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestIngest_Idempotency(t *testing.T) {
	h := NewHTTPHandler(drainBatcher(), &parser.JSONParser{}, true)
	hdr := map[string]string{"Content-Type": "application/json", "X-Idempotency-Key": "k1"}

	if rr := do(h, "POST", `[{"message":"x"}]`, hdr); rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), `"ingested":1`) {
		t.Fatalf("first: %d %s", rr.Code, rr.Body.String())
	}
	if rr := do(h, "POST", `[{"message":"x"}]`, hdr); !strings.Contains(rr.Body.String(), "duplicate_ignored") {
		t.Fatalf("second should be deduped: %s", rr.Body.String())
	}
}

func TestIngest_Disabled(t *testing.T) {
	h := NewHTTPHandler(drainBatcher(), &parser.JSONParser{}, false)
	if rr := do(h, "POST", `[{"message":"x"}]`, map[string]string{"Content-Type": "application/json"}); rr.Code != http.StatusForbidden {
		t.Fatalf("got %d", rr.Code)
	}
}
