// internal/parser/text.go
package parser

import (
	"regexp"
	"strings"
	"time"

	"github.com/logstream/internal/models"
)

type TextParser struct {
	DefaultService string
}

func (p *TextParser) Parse(data []byte) (models.LogEntry, error) {
	line := string(data)

	re := regexp.MustCompile(`\[(.*?)\]\s+(DEBUG|INFO|WARN|ERROR|FATAL)\s+([^:]+):\s+(.*)`)

	matches := re.FindStringSubmatch(line)

	// If it doesn't match our strict format, we just dump it as an INFO log.
	if len(matches) != 5 {
		return models.LogEntry{
			Timestamp: time.Now(),
			Level:     "INFO",
			Service:   p.DefaultService,
			Message:   strings.TrimSpace(line),
		}, nil
	}

	// Parse the matched timestamp (ignoring errors for simplicity in the fallback)
	parsedTime, err := time.Parse(time.RFC3339, matches[1])
	if err != nil {
		parsedTime = time.Now()
	}

	return models.LogEntry{
		Timestamp: parsedTime,
		Level:     matches[2],
		Service:   matches[3],
		Message:   strings.TrimSpace(matches[4]),
	}, nil
}
