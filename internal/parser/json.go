// internal/parser/json.go
package parser

import (
	"encoding/json"
	"time"

	"github.com/logstream/internal/models"
)

type JSONParser struct{}

func (p *JSONParser) Parse(data []byte) (models.LogEntry, error) {
	var entry models.LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return entry, err
	}

	// Default to INFO if not provided
	if entry.Level == "" {
		entry.Level = "INFO"
	}

	// Ensure timestamp exists
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}

	return entry, nil
}
