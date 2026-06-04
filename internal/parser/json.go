// internal/parser/json.go
package parser

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/logstream/internal/models"
)

type JSONParser struct{}

// Parse handles a single JSON log entry.
func (p *JSONParser) Parse(data []byte) (models.LogEntry, error) {
	var entry models.LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return entry, err
	}

	p.applyDefaults(&entry)
	return entry, nil
}

// ParseBatch handles both a single JSON object and a JSON array of objects.
// Always returns a slice for uniform handling by the HTTP handler.
func (p *JSONParser) ParseBatch(data []byte) ([]models.LogEntry, error) {
	data = bytes.TrimSpace(data)

	if len(data) > 0 && data[0] == '[' {
		// Array of entries
		var entries []models.LogEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			return nil, err
		}
		for i := range entries {
			p.applyDefaults(&entries[i])
		}
		return entries, nil
	}

	// Single object — delegate to Parse
	entry, err := p.Parse(data)
	if err != nil {
		return nil, err
	}
	return []models.LogEntry{entry}, nil
}

// applyDefaults sets sensible defaults for missing fields.
func (p *JSONParser) applyDefaults(entry *models.LogEntry) {
	if entry.Level == "" {
		entry.Level = "INFO"
	}
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.Metadata == nil {
		entry.Metadata = make(map[string]any) // Prevents NULL in JSONB column
	}
}
