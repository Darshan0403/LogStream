package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/logstream/internal/models"
)

func entry(msg string) models.LogEntry {
	return models.LogEntry{Timestamp: time.Now(), Level: "INFO", Service: "t", Message: msg}
}

func flatten(segs []Segment) []models.LogEntry {
	var out []models.LogEntry
	for _, s := range segs {
		out = append(out, s.Entries...)
	}
	return out
}

// A failed batch left on disk must survive later successful flushes (H4).
func TestWALFailedSegmentSurvivesLaterFlush(t *testing.T) {
	w := NewWAL(filepath.Join(t.TempDir(), "wal.log"))

	failed, err := w.AppendSegment([]models.LogEntry{entry("failed-1"), entry("failed-2")})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate a subsequent successful flush: append then remove its own segment.
	ok, err := w.AppendSegment([]models.LogEntry{entry("ok-1")})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.RemoveSegment(ok); err != nil {
		t.Fatal(err)
	}

	segs, err := w.Replay()
	if err != nil {
		t.Fatal(err)
	}
	got := flatten(segs)
	if len(got) != 2 || got[0].Message != "failed-1" {
		t.Fatalf("expected the failed batch to be recovered, got %+v", got)
	}
	if len(segs) != 1 || segs[0].Path != failed {
		t.Fatalf("expected only the failed segment, got %v", segs)
	}

	w.RemoveSegments([]string{segs[0].Path})
	segs, _ = w.Replay()
	if len(segs) != 0 {
		t.Fatalf("expected clean WAL after cleanup, got %+v", segs)
	}
}

func TestWALReplayOrderAndEmpty(t *testing.T) {
	w := NewWAL(filepath.Join(t.TempDir(), "x.log"))

	if segs, err := w.Replay(); err != nil || len(segs) != 0 {
		t.Fatalf("empty replay: segs=%v err=%v", segs, err)
	}

	if seg, err := w.AppendSegment(nil); err != nil || seg != "" {
		t.Fatalf("empty batch should be a no-op, got seg=%q err=%v", seg, err)
	}

	for i := 0; i < 5; i++ {
		if _, err := w.AppendSegment([]models.LogEntry{entry(string(rune('a' + i)))}); err != nil {
			t.Fatal(err)
		}
	}
	segs, err := w.Replay()
	if err != nil {
		t.Fatal(err)
	}
	got := flatten(segs)
	if len(got) != 5 || len(segs) != 5 {
		t.Fatalf("want 5 entries / 5 segments, got %d/%d", len(got), len(segs))
	}
	for i := 0; i < 5; i++ {
		if got[i].Message != string(rune('a'+i)) {
			t.Fatalf("segments replayed out of order: %+v", got)
		}
	}
}

func TestWALQuarantineAndTempCleanup(t *testing.T) {
	dir := t.TempDir()
	w := NewWAL(filepath.Join(dir, "w.log"))

	p, err := w.AppendSegment([]models.LogEntry{entry("poison")})
	if err != nil {
		t.Fatal(err)
	}
	// stray temp file from a hypothetical crash mid-write
	tmp := filepath.Join(w.dir, "seg-99999999999999999999-000009.json.tmp")
	if err := os.WriteFile(tmp, []byte("{partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	w.QuarantineSegments([]string{p})
	if _, err := os.Stat(p + ".failed"); err != nil {
		t.Fatalf("expected %s.failed to exist: %v", p, err)
	}

	segs, err := w.Replay()
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) != 0 {
		t.Fatalf("quarantined + temp files must not replay, got %+v", segs)
	}
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("stray .tmp should have been removed on replay")
	}
}
