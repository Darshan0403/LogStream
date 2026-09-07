// internal/storage/partitions.go
package storage

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// partitionWindow is how many weekly partitions to keep provisioned ahead of and
// behind "now". 1 week back + this-week + 2 weeks forward.
const (
	partitionsBehind = 1
	partitionsAhead  = 2
)

// EnsurePartitions creates the weekly partitions of the `logs` table that cover
// the current rolling window (M1). Without this every row falls into
// logs_default and RANGE partitioning buys nothing.
//
// Best effort: a CREATE that fails because logs_default already holds rows for
// that range (a pre-existing non-empty database) is logged and skipped rather
// than aborting startup. Migrating existing data out of the default partition is
// a manual, one-time operation.
func (s *Store) EnsurePartitions(ctx context.Context) error {
	start := isoWeekStart(time.Now().UTC()).AddDate(0, 0, -7*partitionsBehind)
	for i := 0; i < partitionsBehind+1+partitionsAhead; i++ {
		wkStart := start.AddDate(0, 0, 7*i)
		wkEnd := wkStart.AddDate(0, 0, 7)
		name := "logs_" + wkStart.Format("2006_01_02")

		stmt := fmt.Sprintf(
			`CREATE TABLE IF NOT EXISTS %s PARTITION OF logs FOR VALUES FROM ('%s') TO ('%s')`,
			name, wkStart.Format("2006-01-02"), wkEnd.Format("2006-01-02"),
		)
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			slog.Warn("could not provision partition",
				slog.String("partition", name),
				slog.String("from", wkStart.Format("2006-01-02")),
				slog.String("to", wkEnd.Format("2006-01-02")),
				slog.Any("err", err))
		}
	}
	return nil
}

// RunPartitionMaintenance re-checks partitions on an interval until ctx is
// cancelled. Call EnsurePartitions once synchronously first, then run this in a
// goroutine.
func (s *Store) RunPartitionMaintenance(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.EnsurePartitions(ctx)
		}
	}
}

// isoWeekStart returns midnight UTC on the Monday of t's ISO week.
func isoWeekStart(t time.Time) time.Time {
	t = t.UTC()
	weekday := int(t.Weekday()) // Sunday = 0
	if weekday == 0 {
		weekday = 7
	}
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return d.AddDate(0, 0, -(weekday - 1))
}
