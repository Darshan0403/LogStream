// internal/parser/parser.go
package parser

import "github.com/logstream/internal/models"

// LogParser defines the contract for parsing incoming log data
type LogParser interface {
	Parse(data []byte) (models.LogEntry, error)
}
