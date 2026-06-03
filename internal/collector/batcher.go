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
func (b *Batcher) Run(ctx context.Context) {

	ticker := time.NewTicker(2 * time.Second)
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

			fmt.Println("Batcher shutting down...")
			return
		}
	}
}

func (b *Batcher) flush(ctx context.Context, batch []models.LogEntry) {
	if err := AppendToWAL(batch); err != nil {
		fmt.Printf("WAL error: %v\n", err)
	}

	if err := b.store.InsertBatch(ctx, batch); err != nil {
		fmt.Printf("DB Insert error: %v\n", err)
		return
	}

	TruncateWAL()
}
