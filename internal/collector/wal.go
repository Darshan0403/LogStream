// internal/collector/wal.go
package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/logstream/internal/models"
)

// WAL (Write-Ahead Log) persists batches to disk before DB insertion.
// On crash recovery, uncommitted batches are replayed from the WAL file.
type WAL struct {
	path string
}

// NewWAL creates a WAL with a configurable file path.
func NewWAL(path string) *WAL {
	if path == "" {
		path = "wal.log"
	}
	return &WAL{path: path}
}

// Append writes a batch of logs to the WAL file before database insertion.
func (w *WAL) Append(batch []models.LogEntry) error {
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(batch)
	if err != nil {
		return err
	}

	if _, err := f.Write(b); err != nil {
		return err
	}
	if _, err := f.WriteString("\n"); err != nil {
		return err
	}

	return f.Sync()
}

// Truncate clears the WAL file after a successful database insert.
func (w *WAL) Truncate() error {
	return os.Truncate(w.path, 0)
}

// Replay reads uncommitted batches from the WAL file.
// Called once at startup before the batcher begins, to recover from crashes.
func (w *WAL) Replay() ([]models.LogEntry, error) {
	data, err := os.ReadFile(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No WAL file = nothing to replay
		}
		return nil, fmt.Errorf("failed to read WAL: %w", err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}

	var all []models.LogEntry
	// WAL format: one JSON array per line (each line is a serialized batch)
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var batch []models.LogEntry
		if err := json.Unmarshal(line, &batch); err != nil {
			return nil, fmt.Errorf("corrupt WAL entry: %w", err)
		}
		all = append(all, batch...)
	}

	return all, nil
}
