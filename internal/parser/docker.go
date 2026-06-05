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

func (p *DockerParser) Parse(data []byte) (models.LogEntry, error) {
	// 1. Try to parse it as raw JSON first (bypassing the Docker envelope)
	if entry, err := p.jsonParser.Parse(data); err == nil && entry.Message != "" {
		if entry.Service == "" {
			entry.Service = p.DefaultService
		}
		return entry, nil
	}

	// 2. See if it is actually a Docker envelope
	var envelope dockerLogEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Log == "" {
		// Not JSON, and not Docker. Fall back to pure text.
		return p.textParser.Parse(data)
	}

	// --- Handle actual Docker envelope ---
	cleanLog := strings.TrimRight(envelope.Log, "\n\r")
	cleanLog = ansiEscapePattern.ReplaceAllString(cleanLog, "")

	// 3. Try to parse the INNER log content as JSON
	if entry, err := p.jsonParser.Parse([]byte(cleanLog)); err == nil && entry.Message != "" {
		if entry.Timestamp.IsZero() {
			parsedTime, tErr := time.Parse(time.RFC3339Nano, envelope.Time)
			if tErr == nil {
				entry.Timestamp = parsedTime
			}
		}
		if entry.Service == "" {
			entry.Service = p.DefaultService
		}
		return entry, nil
	}

	// 4. Try structured text (e.g., "[TIME] INFO service: message")
	if entry, err := p.textParser.Parse([]byte(cleanLog)); err == nil && entry.Service != p.textParser.DefaultService {
		return entry, nil
	}

	// 5. Ultimate Fallback: pure unstructured string inside a Docker log
	parsedTime, err := time.Parse(time.RFC3339Nano, envelope.Time)
	if err != nil {
		parsedTime = time.Now()
	}

	return models.LogEntry{
		Timestamp: parsedTime,
		Level:     "INFO",
		Service:   p.DefaultService,
		Message:   cleanLog,
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
