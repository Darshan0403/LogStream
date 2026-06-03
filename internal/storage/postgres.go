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

// InsertBatch writes multiple logs in a single network round-trip.
// This is critical for LogStream's performance.
// InsertBatch writes multiple logs in a single network round-trip.
// InsertBatch writes multiple logs in a single network round-trip.
func (s *Store) InsertBatch(ctx context.Context, logs []models.LogEntry) error {
	if len(logs) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	query := `
		INSERT INTO logs (timestamp, level, service, message, metadata) 
		VALUES ($1, $2, $3, $4, $5)`

	for _, log := range logs {
		if log.Timestamp.IsZero() {
			log.Timestamp = time.Now()
		}
		batch.Queue(query, log.Timestamp, log.Level, log.Service, log.Message, log.Metadata)
	}

	br := s.pool.SendBatch(ctx, batch)

	defer br.Close()

	for i := 0; i < len(logs); i++ {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("failed inserting log at index %d: %w", i, err)
		}
	}

	return nil
}
