// cmd/logstream/main.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/logstream/internal/api"
	"github.com/logstream/internal/collector"
	"github.com/logstream/internal/parser"
	"github.com/logstream/internal/storage"
)

func main() {
	// 1. Load Configuration
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://logstream:password@localhost:5433/logstream?sslmode=disable"
	}

	walPath := os.Getenv("WAL_PATH")
	if walPath == "" {
		walPath = "wal.log"
	}

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		apiKey = "dev-key"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Initialize Database
	store, err := storage.New(ctx, dbURL)
	if err != nil {
		fmt.Printf("Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// 3. Initialize WAL and Replay any uncommitted batches from a previous crash
	wal := collector.NewWAL(walPath)

	recovered, err := wal.Replay()
	if err != nil {
		fmt.Printf("WARNING: Failed to replay WAL: %v\n", err)
	} else if len(recovered) > 0 {
		fmt.Printf("Replaying %d logs from WAL...\n", len(recovered))
		if err := store.InsertBatch(ctx, recovered); err != nil {
			fmt.Printf("WARNING: WAL replay insert failed: %v\n", err)
		} else {
			if err := wal.Truncate(); err != nil {
				fmt.Printf("WARNING: WAL truncate after replay failed: %v\n", err)
			}
			fmt.Printf("WAL replay complete. %d logs recovered.\n", len(recovered))
		}
	}

	// 4. Wire Dependencies
	jsonParser := &parser.JSONParser{}
	batcher := collector.NewBatcher(store, wal)
	httpHandler := collector.NewHTTPHandler(batcher, jsonParser)

	// 5. Start the Batcher Goroutine
	go batcher.Run(ctx)

	// 6. Setup API Router
	router := api.NewRouter(store, httpHandler, apiKey)

	server := &http.Server{
		Addr:    ":8090",
		Handler: router,
	}

	// 7. Start Server in background
	go func() {
		fmt.Println("LogStream API & Ingestion Server running on :8090")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server failed: %v\n", err)
		}
	}()

	// 8. Graceful Shutdown — cancel context, wait for batcher, then stop HTTP
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nReceived shutdown signal. Draining...")

	// Cancel context — triggers batcher's ctx.Done() → final flush
	cancel()

	// Wait for batcher to finish its final flush (WAL append → DB insert → WAL truncate)
	<-batcher.Done()

	// Gracefully stop HTTP server (finishes in-flight requests)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Printf("HTTP server shutdown error: %v\n", err)
	}

	fmt.Println("LogStream shut down gracefully.")
}
