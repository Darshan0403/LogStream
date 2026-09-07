// cmd/logstream/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/logstream/internal/alerts"
	"github.com/logstream/internal/api"
	"github.com/logstream/internal/collector"
	"github.com/logstream/internal/logging"
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
			logging.Init(os.Getenv("LOG_FORMAT"), os.Getenv("LOG_LEVEL"))

			// 1. Load Configuration (Flags override Env Vars)
			// PORT env fills in when --port wasn't passed (L6).
			if !cmd.Flags().Changed("port") {
				if p, err := strconv.Atoi(os.Getenv("PORT")); err == nil && p > 0 && p < 65536 {
					port = p
				}
			}

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

			// INGEST_KEY authenticates POST /ingest. Falls back to API_KEY when unset
			// so single-key deployments keep working, but can be set separately to give
			// log shippers a credential that cannot read/mutate the dashboard API.
			ingestKey := os.Getenv("INGEST_KEY")
			if ingestKey == "" {
				ingestKey = apiKey
			}

			// C3: refuse to boot with well-known default secrets unless explicitly
			// opted in. This prevents accidentally shipping an unauthenticated service.
			allowInsecure := os.Getenv("ALLOW_INSECURE_DEFAULTS") == "true"
			if !allowInsecure {
				var problems []string
				if apiKey == "" || apiKey == "dev-key" {
					problems = append(problems, "API_KEY is unset or the default 'dev-key'")
				}
				if ingestKey == "" || ingestKey == "dev-key" {
					problems = append(problems, "INGEST_KEY/API_KEY resolves to the default 'dev-key'")
				}
				if strings.Contains(dbURL, ":password@") {
					problems = append(problems, "DATABASE_URL uses the default password 'password'")
				}
				if len(problems) > 0 {
					slog.Error("refusing to start with insecure default configuration",
						slog.Any("problems", problems),
						slog.String("hint", "set strong values, or export ALLOW_INSECURE_DEFAULTS=true for local dev"))
					os.Exit(1)
				}
			}

			// INGEST_ENABLED controls whether POST /ingest accepts logs.
			// Set to "false" for read-only public deployments.
			// Defaults to true (dev mode, dogfooding, load tests).
			ingestEnabled := os.Getenv("INGEST_ENABLED") != "false"
			slog.Info("ingestion mode", slog.Bool("enabled", ingestEnabled))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// 2. Initialize Database
			store, err := storage.New(ctx, dbURL)
			if err != nil {
				slog.Error("failed to connect to database", slog.Any("err", err))
				os.Exit(1)
			}
			defer store.Close()

			// Provision the rolling window of weekly partitions, then keep it
			// topped up in the background (M1).
			if err := store.EnsurePartitions(ctx); err != nil {
				slog.Warn("partition provisioning failed", slog.Any("err", err))
			}
			go store.RunPartitionMaintenance(ctx, 6*time.Hour)

			// 3. Initialize WAL and Replay (segmented: one file per batch, H4/M7)
			wal := collector.NewWAL(walPath)
			segments, err := wal.Replay()
			if err != nil {
				slog.Warn("failed to replay WAL", slog.Any("err", err))
			}
			var replayed, quarantined int
			for _, seg := range segments {
				if err := store.InsertBatch(ctx, seg.Entries); err != nil {
					slog.Warn("WAL replay of segment failed", slog.String("segment", seg.Path), slog.Any("err", err))
					wal.QuarantineSegments([]string{seg.Path})
					quarantined++
					continue
				}
				wal.RemoveSegments([]string{seg.Path})
				replayed += len(seg.Entries)
			}
			if replayed > 0 || quarantined > 0 {
				slog.Info("WAL replay complete", slog.Int("logs_recovered", replayed), slog.Int("segments_quarantined", quarantined))
			}

			// 4. Wire Dependencies
			alertEngine := alerts.NewEngine(store)
			if err := alertEngine.LoadRules(ctx); err != nil {
				slog.Warn("failed to load alert rules on startup", slog.Any("err", err))
			}

			// Security configuration for the HTTP/WS layer (H1, H2, H6).
			wsSecret := api.DeriveWSSecret(os.Getenv("WS_JWT_SECRET"), apiKey)
			trustedProxies := api.DefaultTrustedProxies()
			if tp := os.Getenv("TRUSTED_PROXIES"); tp != "" {
				trustedProxies = api.ParseCIDRs(tp)
			}
			allowedOrigins := api.ParseOrigins(os.Getenv("ALLOWED_ORIGINS"))
			if len(allowedOrigins) == 0 {
				slog.Warn("ALLOWED_ORIGINS not set; cross-origin browsers refused (same-origin only)")
			}
			apiCfg := api.Config{
				APIKey:         apiKey,
				IngestKey:      ingestKey,
				WSJWTSecret:    wsSecret,
				AllowedOrigins: allowedOrigins,
				TrustedProxies: trustedProxies,
			}

			// NEW: Initialize WebSocket Hub and start its broadcast loop
			hub := api.NewHub(wsSecret, allowedOrigins)
			go hub.Run()

			jsonParser := &parser.JSONParser{}
			// UPDATED: Pass hub to Batcher
			batcher := collector.NewBatcher(store, wal, alertEngine, hub)
			httpHandler := collector.NewHTTPHandler(batcher, jsonParser, ingestEnabled)

			// 5. Start the Batcher Goroutine
			go batcher.Run(ctx)

			// 6. Setup API Router
			// UPDATED: Pass hub to Router
			router := api.NewRouter(store, httpHandler, apiCfg, alertEngine, hub)

			serverAddr := fmt.Sprintf(":%d", port)
			server := &http.Server{
				Addr:    serverAddr,
				Handler: router,
			}

			// 7. Start Server in background
			go func() {
				slog.Info("server listening", slog.String("addr", serverAddr))
				if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					slog.Error("server failed", slog.Any("err", err))
				}
			}()

			// 8. Graceful Shutdown
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			<-sigChan
			slog.Info("shutdown signal received, draining")

			cancel()
			<-batcher.Done()

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := server.Shutdown(shutdownCtx); err != nil {
				slog.Error("HTTP server shutdown error", slog.Any("err", err))
			}

			slog.Info("shutdown complete")
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
