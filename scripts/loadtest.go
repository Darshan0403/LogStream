// scripts/loadtest.go
package main

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// We send batches of 5 to simulate the CLI behavior, but very rapidly
const payloadTemplate = `[
	{"level":"INFO","service":"load-tester","message":"Stress test log A-%d. Testing GIN index full-text search performance and WAL disk flushing."},
	{"level":"WARN","service":"load-tester","message":"Stress test log B-%d. Testing GIN index full-text search performance and WAL disk flushing."},
	{"level":"ERROR","service":"load-tester","message":"Stress test log C-%d. Testing GIN index full-text search performance and WAL disk flushing."},
	{"level":"DEBUG","service":"load-tester","message":"Stress test log D-%d. Testing GIN index full-text search performance and WAL disk flushing."},
	{"level":"INFO","service":"load-tester","message":"Stress test log E-%d. Testing GIN index full-text search performance and WAL disk flushing."}
]`

func main() {
	totalRequests := 2000 // 2,000 requests * 5 logs = 10,000 logs
	concurrency := 100    // 100 concurrent workers

	fmt.Printf("🔥 STARTING FIREHOSE: %d requests (%d total logs) via %d concurrent workers...\n", totalRequests, totalRequests*5, concurrency)

	var wg sync.WaitGroup
	requests := make(chan int, totalRequests)

	// Load up the job queue
	for i := 0; i < totalRequests; i++ {
		requests <- i
	}
	close(requests)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
		},
	}

	start := time.Now()

	var successCount, failCount int
	var mu sync.Mutex

	// Spawn the workers
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range requests {
				payload := fmt.Sprintf(payloadTemplate, i, i, i, i, i)
				req, _ := http.NewRequest("POST", "http://localhost:8090/ingest", bytes.NewBuffer([]byte(payload)))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-API-Key", "dev-key")

				resp, err := client.Do(req)

				mu.Lock()
				if err != nil || resp.StatusCode >= 400 {
					failCount++
					if err == nil {
						fmt.Printf("\n[Error] Server returned status: %d", resp.StatusCode)
					}
				} else {
					successCount++
				}
				mu.Unlock()

				if resp != nil {
					resp.Body.Close()
				}
			}
		}()
	}

	// Wait for all workers to finish
	wg.Wait()
	duration := time.Since(start)

	fmt.Printf("\n\n--- 📊 STRESS TEST COMPLETE ---\n")
	fmt.Printf("Time taken:          %v\n", duration)
	fmt.Printf("Successful Requests: %d\n", successCount)
	fmt.Printf("Failed Requests:     %d\n", failCount)

	logsPerSec := float64(successCount*5) / duration.Seconds()
	fmt.Printf("Throughput:          %.2f logs / second\n", logsPerSec)
}
