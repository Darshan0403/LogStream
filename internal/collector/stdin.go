// internal/collector/stdin.go
package collector

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/logstream/internal/models"
	"github.com/logstream/internal/parser"
)

type StdinCollector struct {
	serverURL string
	service   string
	apiKey    string
	parser    parser.LogParser
	client    *http.Client
}

func NewStdinCollector(serverURL, service, apiKey string, logParser parser.LogParser) *StdinCollector {
	return &StdinCollector{
		serverURL: serverURL,
		service:   service,
		apiKey:    apiKey,
		parser:    logParser,
		client:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (s *StdinCollector) Run(ctx context.Context) {
	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer size to 1MB for exceptionally long log lines
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))

	var batch []models.LogEntry
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Channel to signal a new line was read
	lines := make(chan string)

	go func() {
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	for {
		select {
		case line, ok := <-lines:
			if !ok {
				// EOF reached, flush remaining and exit
				if len(batch) > 0 {
					s.flush(batch)
				}
				return
			}

			// Try to parse as a batch first (if it's a JSON array string)
			entries, err := s.parser.ParseBatch([]byte(line))
			if err != nil {
				// Fallback to single log parsing
				entry, err := s.parser.Parse([]byte(line))
				if err == nil {
					entries = []models.LogEntry{entry}
				}
			}

			for _, entry := range entries {
				if entry.Service == "" {
					entry.Service = s.service
				}
				batch = append(batch, entry)
			}

			if len(batch) >= 50 {
				s.flush(batch)
				batch = nil
			}

		case <-ticker.C:
			if len(batch) > 0 {
				s.flush(batch)
				batch = nil
			}

		case <-ctx.Done():
			if len(batch) > 0 {
				s.flush(batch)
			}
			return
		}
	}
}

func (s *StdinCollector) flush(entries []models.LogEntry) {
	body, err := json.Marshal(entries)
	if err != nil {
		fmt.Printf("ERROR - Failed to marshal log batch: %v\n", err)
		return
	}

	req, err := http.NewRequest("POST", s.serverURL+"/ingest", bytes.NewReader(body))
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("X-API-Key", s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		fmt.Printf("ERROR - Failed to send logs to %s: %v\n", s.serverURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		fmt.Printf("WARNING - Server rejected logs (Status %d)\n", resp.StatusCode)
	}
}
