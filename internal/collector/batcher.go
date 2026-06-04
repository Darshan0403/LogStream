// internal/collector/batcher.go
package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/logstream/internal/alerts"
	"github.com/logstream/internal/models"
	"github.com/logstream/internal/storage"
)

// Batcher accumulates log entries from producers (HTTP handlers, stdin reader)
// and flushes them to the database in batches. A single goroutine drains the
// channel — Go's channel semantics provide synchronization and backpressure.
type Batcher struct {
	ch     chan models.LogEntry
	store  *storage.Store
	wal    *WAL
	engine *alerts.Engine // NEW: Alert engine dependency
	done   chan struct{}  // closed when Run() exits after final flush
}

func NewBatcher(store *storage.Store, wal *WAL, engine *alerts.Engine) *Batcher {
	return &Batcher{
		ch:     make(chan models.LogEntry, 10000), // Buffer handles sudden spikes
		store:  store,
		wal:    wal,
		engine: engine, // NEW
		done:   make(chan struct{}),
	}
}

// Send is called by HTTP handlers to quickly drop a log into the channel.
// Blocks only if the 10K buffer is full (backpressure).
func (b *Batcher) Send(entry models.LogEntry) {
	b.ch <- entry
}

// Done returns a channel that is closed when the batcher has finished its
// final flush and exited. Used by main.go for graceful shutdown sequencing.
func (b *Batcher) Done() <-chan struct{} {
	return b.done
}

// Run executes in a single background goroutine. It drains the channel,
// accumulates entries, and flushes on: 100 entries, 2s ticker, or context cancel.
func (b *Batcher) Run(ctx context.Context) {
	defer close(b.done)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var buf []models.LogEntry

	for {
		select {
		case entry := <-b.ch:
			buf = append(buf, entry)
			if len(buf) >= 100 {
				b.flush(ctx, buf)
				buf = nil
			}
		case <-ticker.C:
			if len(buf) > 0 {
				b.flush(ctx, buf)
				buf = nil
			}
		case <-ctx.Done():
			// Drain any remaining entries from the channel before final flush
			for {
				select {
				case entry := <-b.ch:
					buf = append(buf, entry)
				default:
					goto drain_done
				}
			}
		drain_done:
			if len(buf) > 0 {
				fmt.Printf("Shutting down: flushing final %d logs...\n", len(buf))
				b.flush(context.Background(), buf)
			}
			fmt.Println("Batcher shut down gracefully.")
			return
		}
	}
}

func (b *Batcher) flush(ctx context.Context, batch []models.LogEntry) {
	if err := b.wal.Append(batch); err != nil {
		fmt.Printf("CRITICAL - WAL append error: %v\n", err)
	}

	if err := b.store.InsertBatch(ctx, batch); err != nil {
		fmt.Printf("CRITICAL - DB insert error: %v\n", err)
		return // WAL preserved — will be replayed on next startup
	}

	// Only clear the WAL after successful DB insert
	if err := b.wal.Truncate(); err != nil {
		fmt.Printf("ERROR - Failed to truncate WAL: %v\n", err)
	}

	// NEW: Check alerts after successful insert (using the IDs returned by Postgres)
	if b.engine != nil {
		b.engine.Check(ctx, batch)
	}
}
