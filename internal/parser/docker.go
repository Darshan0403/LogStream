// internal/parser/docker.go
package parser

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/logstream/internal/models"
)

type dockerLogEnvelope struct {
	Log    string `json:"log"`
	Stream string `json:"stream"`
	Time   string `json:"time"`
}

// flexibleJSON captures both canonical LogStream field names AND common
// Go/Python service aliases so we don't silently drop real logs.
//
//	Canonical:  "message", "timestamp"
//	VOID Go:    "msg",     "time"
//	Python:     "message", "created" (uvicorn/structlog)
type flexibleJSON struct {
	// message aliases
	Message string `json:"message"`
	Msg     string `json:"msg"`

	// level — same key in both worlds
	Level string `json:"level"`

	// timestamp aliases
	Timestamp time.Time `json:"timestamp"`
	Time      string    `json:"time"` // raw string — parse manually

	// service (optional)
	Service string `json:"service"`
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

type DockerParser struct {
	DefaultService string
	jsonParser     *JSONParser
	textParser     *TextParser
}

func NewDockerParser(defaultService string) *DockerParser {
	return &DockerParser{
		DefaultService: defaultService,
		jsonParser:     &JSONParser{},
		textParser:     &TextParser{DefaultService: defaultService},
	}
}

// parseFlexibleJSON tries to decode a JSON blob that may use "msg"/"time"
// instead of "message"/"timestamp". Returns (entry, true) on success.
// Any extra fields (repo, sha, port, db_id, etc.) are preserved in Metadata.
func (p *DockerParser) parseFlexibleJSON(data []byte) (models.LogEntry, bool) {
	// Step 1: Unmarshal into generic map to capture ALL fields
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return models.LogEntry{}, false
	}

	// Step 2: Also unmarshal into the typed struct for known fields
	var flex flexibleJSON
	if err := json.Unmarshal(data, &flex); err != nil {
		return models.LogEntry{}, false
	}

	// Resolve message: prefer "message", fall back to "msg"
	msg := flex.Message
	if msg == "" {
		msg = flex.Msg
	}
	if msg == "" {
		return models.LogEntry{}, false // nothing useful here
	}

	// Resolve timestamp: prefer typed "timestamp", fall back to string "time"
	ts := flex.Timestamp
	if ts.IsZero() && flex.Time != "" {
		if t, err := time.Parse(time.RFC3339Nano, flex.Time); err == nil {
			ts = t
		} else if t, err := time.Parse(time.RFC3339, flex.Time); err == nil {
			ts = t
		}
	}
	if ts.IsZero() {
		ts = time.Now()
	}

	// Normalise level
	level := strings.ToUpper(flex.Level)
	if level == "" {
		level = "INFO"
	}
	// Some services emit "WARNING" instead of "WARN"
	if level == "WARNING" {
		level = "WARN"
	}

	svc := flex.Service
	if svc == "" {
		svc = p.DefaultService
	}

	// Step 3: Build Metadata from all remaining fields not consumed above
	knownKeys := map[string]bool{
		"message": true, "msg": true,
		"level": true,
		"timestamp": true, "time": true,
		"service": true,
	}
	metadata := make(map[string]any)
	for k, v := range raw {
		if !knownKeys[k] {
			metadata[k] = v
		}
	}

	return models.LogEntry{
		Timestamp: ts,
		Level:     level,
		Service:   svc,
		Message:   msg,
		Metadata:  metadata,
	}, true
}


func (p *DockerParser) Parse(data []byte) (models.LogEntry, error) {
	// 1. Try flexible JSON first — handles VOID Go services (msg/time)
	//    and standard LogStream format (message/timestamp)
	if entry, ok := p.parseFlexibleJSON(data); ok {
		return entry, nil
	}

	// 2. See if it is a Docker JSON log envelope {"log":"...","stream":"...","time":"..."}
	var envelope dockerLogEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Log == "" {
		// Not JSON at all — try plain text (postgres, uvicorn access logs, etc.)
		return p.textParser.Parse(data)
	}

	// --- Handle actual Docker envelope ---
	cleanLog := strings.TrimRight(envelope.Log, "\n\r")
	cleanLog = ansiEscapePattern.ReplaceAllString(cleanLog, "")

	// 3. Inner content may itself be JSON (e.g. a Go service wrapped by Docker)
	if entry, ok := p.parseFlexibleJSON([]byte(cleanLog)); ok {
		// Prefer the outer envelope timestamp if the inner JSON didn't have one
		if entry.Timestamp.IsZero() {
			if t, err := time.Parse(time.RFC3339Nano, envelope.Time); err == nil {
				entry.Timestamp = t
			}
		}
		return entry, nil
	}

	// 4. Try structured text pattern [TIME] LEVEL service: message
	if entry, err := p.textParser.Parse([]byte(cleanLog)); err == nil {
		return entry, nil
	}

	// 5. Ultimate fallback: treat entire inner string as the message
	ts := time.Now()
	if t, err := time.Parse(time.RFC3339Nano, envelope.Time); err == nil {
		ts = t
	}

	return models.LogEntry{
		Timestamp: ts,
		Level:     "INFO",
		Service:   p.DefaultService,
		Message:   cleanLog,
		Metadata:  make(map[string]any),
	}, nil
}

func (p *DockerParser) ParseBatch(data []byte) ([]models.LogEntry, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var entries []models.LogEntry

	for _, line := range lines {
		if line == "" {
			continue
		}
		entry, err := p.Parse([]byte(line))
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}
