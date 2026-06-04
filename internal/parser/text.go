// internal/parser/text.go
package parser

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/logstream/internal/models"
)

// Pre-compile the regex globally so it only runs once at startup.
var logPattern = regexp.MustCompile(`\[(.*?)\]\s+(DEBUG|INFO|WARN|ERROR|FATAL)\s+([^:]+):\s+(.*)`)

type TextParser struct {
	DefaultService string
}

// Parse handles a single plaintext log line.
func (p *TextParser) Parse(data []byte) (models.LogEntry, error) {
	line := string(data)

	matches := logPattern.FindStringSubmatch(line)

	if len(matches) != 5 {
		return models.LogEntry{}, fmt.Errorf("log line does not match expected format: %s", line)
	}

	parsedTime, err := time.Parse(time.RFC3339, matches[1])
	if err != nil {
		return models.LogEntry{}, fmt.Errorf("invalid timestamp format '%s': %w", matches[1], err)
	}

	return models.LogEntry{
		Timestamp: parsedTime,
		Level:     matches[2],
		Service:   matches[3],
		Message:   strings.TrimSpace(matches[4]),
		Metadata:  make(map[string]any), // Never nil
	}, nil
}

// ParseBatch wraps Parse for a single text line.
// Text parsing is inherently line-by-line, so batch = single parse.
func (p *TextParser) ParseBatch(data []byte) ([]models.LogEntry, error) {
	entry, err := p.Parse(data)
	if err != nil {
		return nil, err
	}
	return []models.LogEntry{entry}, nil
}
