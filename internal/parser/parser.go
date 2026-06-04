// internal/parser/parser.go
package parser

import "github.com/logstream/internal/models"

// LogParser defines the contract for parsing incoming log data.
type LogParser interface {
	// Parse handles a single log entry.
	Parse(data []byte) (models.LogEntry, error)
	// ParseBatch handles a single entry OR an array of entries.
	// Returns a slice in both cases for uniform handling.
	ParseBatch(data []byte) ([]models.LogEntry, error)
}
