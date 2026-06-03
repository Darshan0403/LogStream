// internal/parser/text.go
package parser

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/logstream/internal/models"
)

// FIX 1: Pre-compile the regex globally so it only runs once at startup.
// This turns a O(N) CPU spike into O(1).
var logPattern = regexp.MustCompile(`\[(.*?)\]\s+(DEBUG|INFO|WARN|ERROR|FATAL)\s+([^:]+):\s+(.*)`)

type TextParser struct {
	DefaultService string
}

func (p *TextParser) Parse(data []byte) (models.LogEntry, error) {
	line := string(data)

	matches := logPattern.FindStringSubmatch(line)

	if len(matches) != 5 {
		// FIX 2: Instead of returning a successful INFO log, we return an actual error
		// if it doesn't match our strict format, letting the caller decide how to handle it.
		return models.LogEntry{}, fmt.Errorf("log line does not match expected format: %s", line)
	}

	// FIX 3: Properly handle the timestamp parsing error instead of swallowing it.
	parsedTime, err := time.Parse(time.RFC3339, matches[1])
	if err != nil {
		return models.LogEntry{}, fmt.Errorf("invalid timestamp format '%s': %w", matches[1], err)
	}

	return models.LogEntry{
		Timestamp: parsedTime,
		Level:     matches[2],
		Service:   matches[3],
		Message:   strings.TrimSpace(matches[4]),
	}, nil
}
