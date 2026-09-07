// internal/collector/wal.go
package collector

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/logstream/internal/models"
)

// WAL (Write-Ahead Log) persists each batch to its own segment file before the
// database insert. On a successful insert the segment is deleted; on failure or
// crash it stays on disk and is replayed at startup.
//
// Per-segment files (rather than one shared append-only file) mean a batch that
// failed to insert can never be destroyed by an unrelated batch's cleanup (H4).
type WAL struct {
	dir string
	mu  sync.Mutex
	seq uint64
}

// NewWAL derives a segment directory from the legacy WAL file path so existing
// WAL_PATH settings keep working: "/data/wal.log" -> "/data/wal.wal.d".
func NewWAL(path string) *WAL {
	if path == "" {
		path = "wal.log"
	}
	dir := strings.TrimSuffix(path, filepath.Ext(path)) + ".wal.d"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("could not create WAL dir", slog.String("dir", dir), slog.Any("err", err))
	}
	return &WAL{dir: dir}
}

// AppendSegment durably writes one batch to a new segment file and returns its
// path. An empty batch is a no-op and returns "".
func (w *WAL) AppendSegment(batch []models.LogEntry) (string, error) {
	if len(batch) == 0 {
		return "", nil
	}

	b, err := json.Marshal(batch)
	if err != nil {
		return "", err
	}

	w.mu.Lock()
	w.seq++
	name := fmt.Sprintf("seg-%020d-%06d.json", time.Now().UnixNano(), w.seq)
	w.mu.Unlock()

	final := filepath.Join(w.dir, name)
	tmp := final + ".tmp"

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, final); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return final, nil
}

// RemoveSegment deletes a single committed segment. A missing file is not an error.
func (w *WAL) RemoveSegment(path string) error {
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RemoveSegments deletes several committed segments, best effort.
func (w *WAL) RemoveSegments(paths []string) {
	for _, p := range paths {
		_ = w.RemoveSegment(p)
	}
}

// QuarantineSegments renames segments aside so a permanently-failing batch (bad
// data that will never insert) cannot wedge every future startup or grow the
// WAL without bound (M7). The files are kept for inspection, not deleted.
func (w *WAL) QuarantineSegments(paths []string) {
	for _, p := range paths {
		if p == "" {
			continue
		}
		dst := p + ".failed"
		if err := os.Rename(p, dst); err != nil {
			slog.Warn("could not quarantine WAL segment", slog.String("segment", p), slog.Any("err", err))
			continue
		}
		slog.Warn("quarantined un-insertable WAL segment", slog.String("from", p), slog.String("to", dst))
	}
}

// Segment is one recovered WAL batch and the file it came from.
type Segment struct {
	Path    string
	Entries []models.LogEntry
}

// Replay loads every pending segment in write order, one entry per file kept
// separate so the caller can insert (and remove / quarantine) each batch
// independently — a single poison batch can't block the rest (M7). A corrupt
// file (torn write) is renamed "<name>.corrupt" and skipped.
func (w *WAL) Replay() ([]Segment, error) {
	// Remove partial segments left by a crash mid-write (M7).
	if tmps, _ := filepath.Glob(filepath.Join(w.dir, "seg-*.json.tmp")); len(tmps) > 0 {
		for _, t := range tmps {
			_ = os.Remove(t)
		}
	}

	matches, err := filepath.Glob(filepath.Join(w.dir, "seg-*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to list WAL segments: %w", err)
	}
	sort.Strings(matches)

	var segs []Segment
	for _, p := range matches {
		data, err := os.ReadFile(p)
		if err != nil {
			return segs, fmt.Errorf("failed to read WAL segment %s: %w", p, err)
		}
		var batch []models.LogEntry
		if err := json.Unmarshal(data, &batch); err != nil {
			corrupt := p + ".corrupt"
			_ = os.Rename(p, corrupt)
			slog.Warn("quarantined corrupt WAL segment", slog.String("from", p), slog.String("to", corrupt), slog.Any("err", err))
			continue
		}
		segs = append(segs, Segment{Path: p, Entries: batch})
	}
	return segs, nil
}
