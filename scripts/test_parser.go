// scripts/test_parser.go — standalone parser diagnostic
// Run: go run scripts/test_parser.go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/logstream/internal/parser"
)

func main() {
	// Every real log format from the VOID containers
	tests := []struct {
		name   string
		format string // "docker" or "text"
		input  string
	}{
		// === Go services: structured JSON with msg/time ===
		{
			name:   "Go slog JSON (webhook-handler startup)",
			format: "docker",
			input:  `{"time":"2026-06-06T16:32:18.030897422Z","level":"WARN","msg":"No .env file found. Falling back to system environment variables."}`,
		},
		{
			name:   "Go slog JSON (api-server startup)",
			format: "docker",
			input:  `{"time":"2026-06-06T16:32:18.111833964Z","level":"INFO","msg":"SECURE PLG API Server starting","port":":8083"}`,
		},

		// === Go net/http default access log ===
		{
			name:   "Go HTTP access log (OPTIONS)",
			format: "docker",
			input:  `2026/06/07 07:36:47 "OPTIONS http://localhost:8083/api/v1/repos/649505e1-accd-450a-a9b7-a1312286a983/indexed-files HTTP/1.1" from 192.168.65.1:61582 - 200 0B in 19.959µs`,
		},
		{
			name:   "Go HTTP access log (GET)",
			format: "docker",
			input:  `2026/06/07 07:36:47 "GET http://localhost:8083/api/v1/repos HTTP/1.1" from 192.168.65.1:61582 - 200 890B in 1.196667ms`,
		},

		// === Python uvicorn ===
		{
			name:   "Uvicorn startup",
			format: "docker",
			input:  `INFO:     Started server process [1]`,
		},
		{
			name:   "Uvicorn access log",
			format: "docker",
			input:  `INFO:     172.19.0.8:56338 - "POST /api/ast HTTP/1.1" 200 OK`,
		},
		{
			name:   "Python plain text",
			format: "docker",
			input:  `Loading CodeBERT model into memory... (This takes a moment)`,
		},

		// === PostgreSQL ===
		{
			name:   "Postgres checkpoint",
			format: "text",
			input:  `2026-06-07 07:35:25.689 UTC [27] LOG:  checkpoint starting: time`,
		},
		{
			name:   "Postgres startup",
			format: "text",
			input:  `PostgreSQL Database directory appears to contain a database; Skipping initialization`,
		},

		// === Nginx (frontend) ===
		{
			name:   "Nginx access log",
			format: "docker",
			input:  `192.168.65.1 - - [07/Jun/2026:07:36:12 +0000] "GET / HTTP/1.1" 304 0 "-" "Mozilla/5.0" "-"`,
		},
		{
			name:   "Nginx notice",
			format: "docker",
			input:  `2026/06/07 07:30:31 [notice] 1#1: start worker process 36`,
		},
	}

	passed := 0
	failed := 0
	for i, tt := range tests {
		var p parser.LogParser
		if tt.format == "docker" {
			p = parser.NewDockerParser("test-service")
		} else {
			p = &parser.TextParser{DefaultService: "test-service"}
		}

		entry, err := p.Parse([]byte(tt.input))

		status := "✅ PASS"
		if err != nil {
			status = "❌ FAIL"
			failed++
		} else if entry.Message == "" {
			status = "⚠️  EMPTY MSG"
			failed++
		} else {
			passed++
		}

		fmt.Printf("\n--- Test %d: %s ---\n", i+1, tt.name)
		fmt.Printf("Status:  %s\n", status)
		fmt.Printf("Format:  %s\n", tt.format)
		fmt.Printf("Input:   %.100s\n", tt.input)
		if err != nil {
			fmt.Printf("Error:   %v\n", err)
		} else {
			j, _ := json.MarshalIndent(map[string]any{
				"level":   entry.Level,
				"service": entry.Service,
				"message": entry.Message,
				"ts":      entry.Timestamp.String(),
			}, "  ", "  ")
			fmt.Printf("Parsed:  %s\n", string(j))
		}
	}

	fmt.Printf("\n=== RESULTS: %d passed, %d failed out of %d ===\n", passed, failed, passed+failed)
	if failed > 0 {
		os.Exit(1)
	}
}
