// internal/storage/postgres.go
package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/logstream/internal/models"
)

type Store struct {
	pool *pgxpool.Pool
}

// New initializes the PostgreSQL connection pool
func New(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse db config: %w", err)
	}

	// Optimize for batch ingestion
	config.MaxConns = 20
	config.MinConns = 5
	config.MaxConnIdleTime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to db: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("database unreachable: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Close shuts down the connection pool
func (s *Store) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

// InsertBatch writes multiple logs in a single transaction and populates each
// entry's ID from RETURNING. All-or-nothing: a failure part-way through rolls
// the whole batch back, so a retained WAL segment replays cleanly with no
// partially-committed duplicates (M8).
func (s *Store) InsertBatch(ctx context.Context, logs []models.LogEntry) error {
	if len(logs) == 0 {
		return nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin insert tx: %w", err)
	}
	defer tx.Rollback(ctx) // no-op once committed

	const query = `
		INSERT INTO logs (timestamp, level, service, message, metadata)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`

	batch := &pgx.Batch{}
	now := time.Now()
	for _, log := range logs {
		ts := log.Timestamp
		if ts.IsZero() {
			ts = now
		}
		batch.Queue(query, ts, log.Level, log.Service, log.Message, log.Metadata)
	}

	br := tx.SendBatch(ctx, batch)
	for i := range logs {
		if err := br.QueryRow().Scan(&logs[i].ID); err != nil {
			br.Close()
			return fmt.Errorf("failed inserting log at index %d: %w", i, err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("closing insert batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit insert tx: %w", err)
	}
	return nil
}

// Ping verifies the database connection is alive. Used for health checks.
func (s *Store) Ping(ctx context.Context) error {
	if s.pool == nil {
		return fmt.Errorf("database pool is not initialized")
	}
	return s.pool.Ping(ctx)
}
