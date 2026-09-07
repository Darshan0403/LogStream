// internal/collector/batcher.go
package collector

import (
	"context"
	"errors"
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/logstream/internal/alerts"
	"github.com/logstream/internal/api"
	"github.com/logstream/internal/models"
	"github.com/logstream/internal/storage"
)

var (
	ErrBackpressure = errors.New("channel is experiencing backpressure")
	ErrChannelFull  = errors.New("channel is completely full")
)

type Batcher struct {
	ch     chan models.LogEntry
	store  *storage.Store
	wal    *WAL
	engine *alerts.Engine
	hub    *api.Hub
	done   chan struct{}
}

func NewBatcher(store *storage.Store, wal *WAL, engine *alerts.Engine, hub *api.Hub) *Batcher {
	return &Batcher{
		ch:     make(chan models.LogEntry, 10000),
		store:  store,
		wal:    wal,
		engine: engine,
		hub:    hub,
		done:   make(chan struct{}),
	}
}

func (b *Batcher) Send(entry models.LogEntry) error {
	capacity := float64(cap(b.ch))
	current := float64(len(b.ch))

	if current > capacity*0.8 {
		return ErrBackpressure
	}

	select {
	case b.ch <- entry:
		return nil
	default:
		return ErrChannelFull
	}
}

func (b *Batcher) Done() <-chan struct{} {
	return b.done
}

// FEATURE M: The Worker Pool
func (b *Batcher) Run(ctx context.Context) {
	defer close(b.done)

	// Dynamically scale workers to the number of available CPU cores
	numWorkers := runtime.NumCPU()
	slog.Info("batcher starting", slog.Int("workers", numWorkers))

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		// Spawn independent concurrent workers
		go b.resilientWorker(ctx, &wg, i)
	}

	// Block until all workers finish draining during a graceful shutdown
	wg.Wait()
	slog.Info("batcher workers shut down")
}

// resilientWorker ensures if one thread panics, the others keep running,
// and the dead thread is instantly revived.
func (b *Batcher) resilientWorker(ctx context.Context, wg *sync.WaitGroup, workerID int) {
	defer wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("batcher worker panicked, restarting", slog.Int("worker", workerID), slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
				}
			}()
			b.workerLoop(ctx, workerID)
		}()

		if ctx.Err() != nil {
			return
		}
		time.Sleep(100 * time.Millisecond) // Prevent rapid crash loops
	}
}

func (b *Batcher) workerLoop(ctx context.Context, workerID int) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// THE BUCKET: Each worker has its own isolated memory array
	batch := make([]models.LogEntry, 0, 100)

	for {
		select {
		case entry, ok := <-b.ch:
			if !ok {
				return // Channel closed
			}
			batch = append(batch, entry)
			if len(batch) >= 100 {
				b.flush(ctx, batch, workerID)
				batch = nil // fresh backing array; old one is owned by flush/hub now
			}
		case <-ticker.C:
			if len(batch) > 0 {
				b.flush(ctx, batch, workerID)
				batch = nil // fresh backing array; old one is owned by flush/hub now
			}
		case <-ctx.Done():
			// Drain logic for this specific worker
			for {
				select {
				case entry := <-b.ch:
					batch = append(batch, entry)
				default:
					goto drain_done
				}
			}
		drain_done:
			if len(batch) > 0 {
				b.flush(context.Background(), batch, workerID)
			}
			return
		}
	}
}

// flush persists one batch. Each batch gets its own WAL segment, so workers no
// longer need a shared lock and a failed insert leaves exactly that batch on
// disk for recovery without any other flush being able to delete it (H4).
//
// The caller must not reuse the backing array of batch after calling flush —
// the slice is handed to the WebSocket hub asynchronously.
func (b *Batcher) flush(ctx context.Context, batch []models.LogEntry, workerID int) {
	seg, err := b.wal.AppendSegment(batch)
	if err != nil {
		slog.Error("WAL append failed", slog.Int("worker", workerID), slog.Any("err", err))
	}

	if err := b.store.InsertBatch(ctx, batch); err != nil {
		// Segment is intentionally retained; it will be replayed on next start.
		slog.Error("DB insert failed; WAL segment retained", slog.Int("worker", workerID), slog.String("segment", seg), slog.Any("err", err))
		return
	}

	if err := b.wal.RemoveSegment(seg); err != nil {
		slog.Error("failed to remove committed WAL segment", slog.String("segment", seg), slog.Any("err", err))
	}

	// SIEM evaluation + live tail. Broadcast is non-blocking (H3).
	if b.engine != nil {
		b.engine.Check(ctx, batch)
	}
	if b.hub != nil {
		b.hub.Broadcast(batch)
	}
}
