// cmd/logstream/main.go
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/logstream/internal/alerts"
	"github.com/logstream/internal/api"
	"github.com/logstream/internal/collector"
	"github.com/logstream/internal/parser"
	"github.com/logstream/internal/storage"
	"github.com/spf13/cobra"
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "logstream",
		Short: "LogStream - Real-time log aggregation pipeline",
	}

	rootCmd.AddCommand(buildServeCmd())
	rootCmd.AddCommand(buildCollectCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// --- SERVE COMMAND (Your exact Day 3 logic + WebSocket Hub) ---

func buildServeCmd() *cobra.Command {
	var port int
	var dbURLFlag, apiKeyFlag, walPathFlag string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the LogStream API server",
		Run: func(cmd *cobra.Command, args []string) {
			// 1. Load Configuration (Flags override Env Vars)
			dbURL := dbURLFlag
			if dbURL == "" {
				dbURL = os.Getenv("DATABASE_URL")
				if dbURL == "" {
					dbURL = "postgres://logstream:password@localhost:5433/logstream?sslmode=disable"
				}
			}

			walPath := walPathFlag
			if walPath == "" {
				walPath = os.Getenv("WAL_PATH")
				if walPath == "" {
					walPath = "wal.log"
				}
			}

			apiKey := apiKeyFlag
			if apiKey == "" {
				apiKey = os.Getenv("API_KEY")
				if apiKey == "" {
					apiKey = "dev-key"
				}
			}

			// INGEST_ENABLED controls whether POST /ingest accepts logs.
			// Set to "false" for read-only public deployments.
			// Defaults to true (dev mode, dogfooding, load tests).
			ingestEnabled := os.Getenv("INGEST_ENABLED") != "false"
			if ingestEnabled {
				fmt.Println("Ingestion: ENABLED (set INGEST_ENABLED=false for read-only mode)")
			} else {
				fmt.Println("Ingestion: DISABLED — read-only deployment mode")
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

			// 3. Initialize WAL and Replay
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
			alertEngine := alerts.NewEngine(store)
			if err := alertEngine.LoadRules(ctx); err != nil {
				fmt.Printf("WARNING: Failed to load alert rules on startup: %v\n", err)
			}

			// NEW: Initialize WebSocket Hub and start its broadcast loop
			hub := api.NewHub()
			go hub.Run()

			jsonParser := &parser.JSONParser{}
			// UPDATED: Pass hub to Batcher
			batcher := collector.NewBatcher(store, wal, alertEngine, hub)
			httpHandler := collector.NewHTTPHandler(batcher, jsonParser, ingestEnabled)

			// 5. Start the Batcher Goroutine
			go batcher.Run(ctx)

			// 6. Setup API Router
			// UPDATED: Pass hub to Router
			router := api.NewRouter(store, httpHandler, apiKey, alertEngine, hub)

			serverAddr := fmt.Sprintf(":%d", port)
			server := &http.Server{
				Addr:    serverAddr,
				Handler: router,
			}

			// 7. Start Server in background
			go func() {
				fmt.Printf("LogStream API & Ingestion Server running on %s\n", serverAddr)
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					fmt.Printf("Server failed: %v\n", err)
				}
			}()

			// 8. Graceful Shutdown
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			<-sigChan
			fmt.Println("\nReceived shutdown signal. Draining...")

			cancel()
			<-batcher.Done()

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				fmt.Printf("HTTP server shutdown error: %v\n", err)
			}

			fmt.Println("LogStream shut down gracefully.")
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 8090, "Port to listen on")
	cmd.Flags().StringVar(&dbURLFlag, "db-url", "", "PostgreSQL connection string")
	cmd.Flags().StringVar(&apiKeyFlag, "api-key", "", "API Key for protection")
	cmd.Flags().StringVar(&walPathFlag, "wal-path", "", "Path to WAL file")

	return cmd
}

// --- COLLECT COMMAND (The Stdin Pipe) ---

func buildCollectCmd() *cobra.Command {
	var service, url, apiKey, format string

	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Pipe stdin logs to a LogStream server",
		Run: func(cmd *cobra.Command, args []string) {
			if apiKey == "" {
				apiKey = os.Getenv("API_KEY")
				if apiKey == "" {
					apiKey = "dev-key"
				}
			}

			var logParser parser.LogParser
			switch strings.ToLower(format) {
			case "docker":
				logParser = parser.NewDockerParser(service)
			case "json":
				logParser = &parser.JSONParser{}
			case "text":
				logParser = &parser.TextParser{DefaultService: service}
			default: // auto
				logParser = parser.NewDockerParser(service)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Graceful shutdown for the collector
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-sigChan
				cancel()
			}()

			agent := collector.NewStdinCollector(url, service, apiKey, logParser)
			agent.Run(ctx) // Blocks until EOF (Ctrl+D) or SIGINT
		},
	}

	// Required flag
	cmd.Flags().StringVar(&service, "service", "", "Service name to inject into logs (required)")
	cmd.MarkFlagRequired("service")

	// Optional flags
	cmd.Flags().StringVar(&url, "url", "http://localhost:8090", "LogStream server URL")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "LogStream API Key")
	cmd.Flags().StringVar(&format, "format", "auto", "Log format: auto, docker, json, text")

	return cmd
}
