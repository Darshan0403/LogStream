// internal/collector/wal.go
package collector

import (
	"encoding/json"
	"os"

	"github.com/logstream/internal/models"
)

// Append writes a batch of logs to the WAL file before database insertion.
func AppendToWAL(batch []models.LogEntry) error {

	f, err := os.OpenFile("wal.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

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

	// Force write to disk
	return f.Sync()
}

// TruncateWAL clears the file after a successful database insert.
func TruncateWAL() error {
	return os.Truncate("wal.log", 0)
}
