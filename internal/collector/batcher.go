// internal/collector/batcher.go
package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/logstream/internal/models"
	"github.com/logstream/internal/storage"
)

type Batcher struct {
	ch    chan models.LogEntry
	store *storage.Store
}

func NewBatcher(store *storage.Store) *Batcher {
	return &Batcher{
		ch:    make(chan models.LogEntry, 10000), // Buffer handles sudden spikes
		store: store,
	}
}

// Send is called by HTTP handlers to quickly drop a log into the channel
func (b *Batcher) Send(entry models.LogEntry) {
	b.ch <- entry
}

// Run executes in a single background goroutine
// internal/collector/batcher.go

// Run executes in a single background goroutine
func (b *Batcher) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	// FIX 2: Prevent Ticker Leak
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
			// FIX 3: Prevent Data Loss on Shutdown
			// We MUST flush whatever is left in memory before we exit.
			// We use context.Background() here because the parent ctx is already canceled.
			if len(buf) > 0 {
				fmt.Printf("Shutting down: Flushing final %d logs...\n", len(buf))
				b.flush(context.Background(), buf)
			}
			fmt.Println("Batcher shut down gracefully.")
			return
		}
	}
}

func (b *Batcher) flush(ctx context.Context, batch []models.LogEntry) {
	if err := AppendToWAL(batch); err != nil {
		// In a real app, we might use a robust logger, but we shouldn't crash the batcher
		fmt.Printf("CRITICAL - WAL error: %v\n", err)
	}

	if err := b.store.InsertBatch(ctx, batch); err != nil {
		fmt.Printf("CRITICAL - DB Insert error: %v\n", err)
		return // If DB fails, we leave the WAL file intact so it can be replayed on reboot
	}

	// Only clear the WAL if the DB insert succeeded
	if err := TruncateWAL(); err != nil {
		fmt.Printf("ERROR - Failed to truncate WAL: %v\n", err)
	}
}
