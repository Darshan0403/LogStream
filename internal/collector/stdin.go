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
	"strings"
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
	fmt.Printf("[collect:%s] Connected. Reading stdin → %s\n", s.service, s.serverURL)

	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer size to 1MB for exceptionally long log lines
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, len(buf))

	var batch []models.LogEntry
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	linesRead := 0
	linesParsed := 0

	// Channel to signal a new line was read
	lines := make(chan string)

	go func() {
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			fmt.Printf("[collect:%s] Scanner error: %v\n", s.service, err)
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
				fmt.Printf("[collect:%s] EOF. Total: %d lines read, %d parsed.\n", s.service, linesRead, linesParsed)
				return
			}

			linesRead++

			// Skip empty lines
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			// Parse the single line directly — stdin is always one line at a time.
			// (ParseBatch was a no-op here because DockerParser.ParseBatch swallows
			// errors and returns empty slices, making the fallback to Parse dead code.)
			entry, err := s.parser.Parse([]byte(trimmed))
			if err != nil {
				fmt.Printf("[collect:%s] PARSE FAIL: %v | line: %.80s\n", s.service, err, trimmed)
				continue
			}

			// Skip entries with empty messages (blank lines that parsed as empty)
			if entry.Message == "" {
				continue
			}

			// Inject service name if the parser didn't set one
			if entry.Service == "" {
				entry.Service = s.service
			}

			linesParsed++
			batch = append(batch, entry)

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
			fmt.Printf("[collect:%s] Stopped. Total: %d lines read, %d parsed.\n", s.service, linesRead, linesParsed)
			return
		}
	}
}

func (s *StdinCollector) flush(entries []models.LogEntry) {
	body, err := json.Marshal(entries)
	if err != nil {
		fmt.Printf("[collect:%s] ERROR marshal: %v\n", s.service, err)
		return
	}

	req, err := http.NewRequest("POST", s.serverURL+"/ingest", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("[collect:%s] ERROR creating request: %v\n", s.service, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("X-API-Key", s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		fmt.Printf("[collect:%s] ERROR sending %d logs to %s: %v\n", s.service, len(entries), s.serverURL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		fmt.Printf("[collect:%s] WARNING server rejected %d logs (Status %d)\n", s.service, len(entries), resp.StatusCode)
	} else {
		fmt.Printf("[collect:%s] Flushed %d logs → %s (HTTP %d)\n", s.service, len(entries), s.serverURL, resp.StatusCode)
	}
}

