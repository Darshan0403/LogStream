// internal/parser/text.go
package parser

import (
	"regexp"
	"strings"
	"time"

	"github.com/logstream/internal/models"
)

// Pre-compiled patterns for known log formats, ordered from most specific to
// least specific. The last resort is always "wrap the raw line as INFO."
var (
	// [TIMESTAMP] LEVEL service: message  (original LogStream format)
	logstreamPattern = regexp.MustCompile(`\[(.*?)\]\s+(DEBUG|INFO|WARN|ERROR|FATAL)\s+([^:]+):\s+(.*)`)

	// Python uvicorn:  INFO:     172.19.0.8:56338 - "POST /api/ast HTTP/1.1" 200 OK
	// Also matches:    INFO:     Started server process [1]
	uvicornPattern = regexp.MustCompile(`^(DEBUG|INFO|WARNING|ERROR|CRITICAL):\s+(.+)`)

	// Go net/http default logger:  2026/06/07 07:36:47 "GET http://... HTTP/1.1" from ... - 200 890B in 1.19ms
	goHTTPPattern = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2}\s+\d{2}:\d{2}:\d{2})\s+(.+)`)

	// PostgreSQL:  2026-06-07 07:35:25.689 UTC [27] LOG:  checkpoint starting: time
	postgresPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\.\d+\s+\w+)\s+\[\d+\]\s+(\w+):\s+(.+)`)

	// Nginx access:  192.168.65.1 - - [07/Jun/2026:07:36:12 +0000] "GET / HTTP/1.1" 304 0 ...
	nginxPattern = regexp.MustCompile(`^[\d.]+ - - \[(.+?)\]\s+"(\w+)\s+(.+?)\s+HTTP/[\d.]+"\s+(\d+)`)

	// Generic level detector for anything with a recognizable level keyword
	levelKeywords = regexp.MustCompile(`(?i)\b(DEBUG|INFO|WARN(?:ING)?|ERROR|FATAL|CRITICAL|NOTICE|LOG|PANIC)\b`)
)

type TextParser struct {
	DefaultService string
}

// Parse handles a single plaintext log line.
// It tries several known formats and ALWAYS succeeds — the last resort wraps
// the raw line as an INFO entry. This ensures no log line is ever silently dropped.
func (p *TextParser) Parse(data []byte) (models.LogEntry, error) {
	line := strings.TrimSpace(string(data))
	if line == "" {
		return models.LogEntry{}, nil
	}

	// 1. Original LogStream structured text: [TIMESTAMP] LEVEL service: message
	if m := logstreamPattern.FindStringSubmatch(line); len(m) == 5 {
		if ts, err := time.Parse(time.RFC3339, m[1]); err == nil {
			return models.LogEntry{
				Timestamp: ts,
				Level:     normaliseLevel(m[2]),
				Service:   m[3],
				Message:   strings.TrimSpace(m[4]),
				Metadata:  make(map[string]any),
			}, nil
		}
	}

	// 2. Uvicorn / Python logging:  INFO:     message text
	if m := uvicornPattern.FindStringSubmatch(line); len(m) == 3 {
		return models.LogEntry{
			Timestamp: time.Now(),
			Level:     normaliseLevel(m[1]),
			Service:   p.DefaultService,
			Message:   strings.TrimSpace(m[2]),
			Metadata:  make(map[string]any),
		}, nil
	}

	// 3. PostgreSQL:  2026-06-07 07:35:25.689 UTC [27] LOG:  checkpoint starting
	if m := postgresPattern.FindStringSubmatch(line); len(m) == 4 {
		ts := parseLooseTimestamp(m[1])
		return models.LogEntry{
			Timestamp: ts,
			Level:     normaliseLevel(m[2]),
			Service:   p.DefaultService,
			Message:   strings.TrimSpace(m[3]),
			Metadata:  make(map[string]any),
		}, nil
	}

	// 4. Go net/http access log:  2026/06/07 07:36:47 "GET http://... HTTP/1.1" from ...
	if m := goHTTPPattern.FindStringSubmatch(line); len(m) == 3 {
		ts := parseLooseTimestamp(m[1])
		return models.LogEntry{
			Timestamp: ts,
			Level:     "INFO",
			Service:   p.DefaultService,
			Message:   strings.TrimSpace(m[2]),
			Metadata:  make(map[string]any),
		}, nil
	}

	// 5. Nginx access log:  192.168.65.1 - - [07/Jun/2026:07:36:12 +0000] "GET / HTTP/1.1" 304 ...
	if m := nginxPattern.FindStringSubmatch(line); len(m) >= 5 {
		ts := parseNginxTimestamp(m[1])
		statusCode := m[4]
		level := "INFO"
		if statusCode >= "400" && statusCode < "500" {
			level = "WARN"
		} else if statusCode >= "500" {
			level = "ERROR"
		}
		return models.LogEntry{
			Timestamp: ts,
			Level:     level,
			Service:   p.DefaultService,
			Message:   line, // Keep the full access log line
			Metadata:  make(map[string]any),
		}, nil
	}

	// 6. ULTIMATE FALLBACK — never fails.
	// Try to extract a level from anywhere in the line; default to INFO.
	level := "INFO"
	if m := levelKeywords.FindStringSubmatch(line); len(m) == 2 {
		level = normaliseLevel(m[1])
	}

	return models.LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Service:   p.DefaultService,
		Message:   line,
		Metadata:  make(map[string]any),
	}, nil
}

// ParseBatch wraps Parse for a single text line.
func (p *TextParser) ParseBatch(data []byte) ([]models.LogEntry, error) {
	entry, err := p.Parse(data)
	if err != nil {
		return nil, err
	}
	// Don't return empty entries from blank lines
	if entry.Message == "" {
		return []models.LogEntry{}, nil
	}
	return []models.LogEntry{entry}, nil
}

// normaliseLevel maps common level strings to LogStream's canonical set.
func normaliseLevel(raw string) string {
	switch strings.ToUpper(raw) {
	case "DEBUG", "TRACE":
		return "DEBUG"
	case "INFO", "NOTICE", "LOG":
		return "INFO"
	case "WARN", "WARNING":
		return "WARN"
	case "ERROR", "CRITICAL":
		return "ERROR"
	case "FATAL", "PANIC":
		return "FATAL"
	default:
		return "INFO"
	}
}

// parseLooseTimestamp tries several common timestamp layouts.
func parseLooseTimestamp(s string) time.Time {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006/01/02 15:04:05",
		"2006-01-02 15:04:05.000 MST",
		"2006-01-02 15:04:05.000000 MST",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}

// parseNginxTimestamp handles Nginx's combined log format timestamp.
func parseNginxTimestamp(s string) time.Time {
	// 07/Jun/2026:07:36:12 +0000
	if t, err := time.Parse("02/Jan/2006:15:04:05 -0700", s); err == nil {
		return t
	}
	return time.Now()
}
