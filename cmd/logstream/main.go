// cmd/logstream/main.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/logstream/internal/collector"
	"github.com/logstream/internal/parser"
	"github.com/logstream/internal/storage"
)

func main() {
	// 1. Initialize Database
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://logstream:password@localhost:5433/logstream?sslmode=disable"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := storage.New(ctx, dbURL)
	if err != nil {
		fmt.Printf("Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	// 2. Wire Dependencies
	jsonParser := &parser.JSONParser{}
	batcher := collector.NewBatcher(store)
	httpHandler := collector.NewHTTPHandler(batcher, jsonParser)

	// 3. Start the Batcher Goroutine
	go batcher.Run(ctx)

	// 4. Setup HTTP Server
	mux := http.NewServeMux()
	mux.Handle("/ingest", httpHandler)

	server := &http.Server{
		Addr:    ":8090",
		Handler: mux,
	}

	// 5. Start Server in background
	go func() {
		fmt.Println("LogStream Ingestion Server running on :8090")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("Server failed: %v\n", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	fmt.Println("\nReceived interrupt signal. Hard exiting...")
	os.Exit(0)
}
