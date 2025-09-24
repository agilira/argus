// overhead_analysis_test.go: Testing Argus Overhead
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Benchmark specific to measure overhead per log entry
func BenchmarkArgus_PerLogEntryOverhead(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "per_entry_overhead")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Failed to remove tempDir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configFile, []byte(`{"level": "info"}`), 0644); err != nil {
		b.Fatal(err)
	}

	b.Run("Single_LogEntry_Baseline", func(b *testing.B) {
		benchmarkSingleLogEntryBaseline(b)
	})

	b.Run("Single_LogEntry_WithArgus", func(b *testing.B) {
		benchmarkSingleLogEntryWithArgus(b, configFile)
	})
}

func benchmarkSingleLogEntryBaseline(b *testing.B) {
	// Simulates a single logging operation (as in Iris)
	message := "User request processed successfully"
	level := "INFO"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simulates the minimal cost of a log entry
		_ = level + ": " + message
	}
}

func benchmarkSingleLogEntryWithArgus(b *testing.B, configFile string) {
	// Setup Argus in background
	config := Config{
		PollInterval:         100 * time.Millisecond, // Typical production poll
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Logf("Failed to stop watcher: %v", err)
		}
	}()

	var configReloads atomic.Int64
	if err := watcher.Watch(configFile, func(event ChangeEvent) {
		configReloads.Add(1)
	}); err != nil {
		b.Fatalf("Failed to watch configFile: %v", err)
	}

	if err := watcher.Start(); err != nil {
		b.Fatalf("Failed to start watcher: %v", err)
	}
	time.Sleep(50 * time.Millisecond) // Stabilization

	// Simulates the same logging operation but with Argus active
	message := "User request processed successfully"
	level := "INFO"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Same operation as baseline
		_ = level + ": " + message
	}

	b.StopTimer()
	b.Logf("Config reloads during test: %d", configReloads.Load())
}

// Benchmark to calculate HTTP request overhead
func BenchmarkArgus_HTTPRequestOverhead(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "http_overhead")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Failed to remove tempDir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configFile, []byte(`{"level": "info"}`), 0644); err != nil {
		b.Fatal(err)
	}

	b.Run("HTTP_Request_Baseline", func(b *testing.B) {
		benchmarkHTTPRequestBaseline(b)
	})

	b.Run("HTTP_Request_WithArgus", func(b *testing.B) {
		benchmarkHTTPRequestWithArgus(b, configFile)
	})
}

func benchmarkHTTPRequestBaseline(b *testing.B) {
	// Simulation of a typical HTTP request with logging (as in Iris)
	var requestCount atomic.Int64

	processHTTPRequest := func() {
		requestCount.Add(1)
		// Simula:
		// 1. Request parsing
		// 2. Business logic
		// 3. Response generation
		// 4. Logging (2-3 log entries per request)
		_ = "GET /api/users"
		_ = "Request processed"
		_ = "Response sent"
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		processHTTPRequest()
	}

	b.StopTimer()
	b.Logf("Processed %d HTTP requests", requestCount.Load())
}

func benchmarkHTTPRequestWithArgus(b *testing.B, configFile string) {
	// Setup Argus (as it would be done in Iris)
	config := Config{
		PollInterval:         200 * time.Millisecond, // Less aggressive polling for production
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Logf("Failed to stop watcher: %v", err)
		}
	}()

	var configReloads atomic.Int64
	if err := watcher.Watch(configFile, func(event ChangeEvent) {
		configReloads.Add(1)
		// Simulates the cost of reconfiguring the logger in Iris
		time.Sleep(100 * time.Microsecond) // 0.1ms typical for reload config
	}); err != nil {
		b.Fatalf("Failed to watch config file: %v", err)
	}

	if err := watcher.Start(); err != nil {
		b.Fatalf("Failed to start watcher: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // Stabilization

	var requestCount atomic.Int64

	processHTTPRequest := func() {
		requestCount.Add(1)
		// Same logic as baseline - Argus works in the background
		_ = "GET /api/users"
		_ = "Request processed"
		_ = "Response sent"
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		processHTTPRequest()
	}

	b.StopTimer()
	b.Logf("Processed %d HTTP requests", requestCount.Load())
	b.Logf("Config reloads: %d", configReloads.Load())
}

// Benchmark for calculating overhead
func BenchmarkArgus_ThroughputImpact(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "throughput_impact")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Failed to remove tempDir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configFile, []byte(`{"level": "info"}`), 0644); err != nil {
		b.Fatal(err)
	}

	b.Run("Throughput_1000RPS_Baseline", func(b *testing.B) {
		benchmarkThroughputBaseline(b, 1000) // 1000 requests/second
	})

	b.Run("Throughput_1000RPS_WithArgus", func(b *testing.B) {
		benchmarkThroughputWithArgus(b, configFile, 1000)
	})

	b.Run("Throughput_10000RPS_Baseline", func(b *testing.B) {
		benchmarkThroughputBaseline(b, 10000) // 10K requests/second
	})

	b.Run("Throughput_10000RPS_WithArgus", func(b *testing.B) {
		benchmarkThroughputWithArgus(b, configFile, 10000)
	})
}

func benchmarkThroughputBaseline(b *testing.B, targetRPS int) {
	var processed atomic.Int64

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()

		// Simulation of 1 second of requests at targetRPS
		for j := 0; j < targetRPS; j++ {
			processed.Add(1)
			// Simula minimal request processing
			_ = "request processed"
		}

		elapsed := time.Since(start)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
	}

	b.StopTimer()
	b.Logf("Target: %d RPS, Processed: %d requests", targetRPS, processed.Load())
}

func benchmarkThroughputWithArgus(b *testing.B, configFile string, targetRPS int) {
	// Setup Argus
	config := Config{
		PollInterval:         100 * time.Millisecond, // 10 times per second
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Logf("Failed to stop watcher: %v", err)
		}
	}()

	var configReloads atomic.Int64
	if err := watcher.Watch(configFile, func(event ChangeEvent) {
		configReloads.Add(1)
	}); err != nil {
		b.Fatalf("Failed to watch config file: %v", err)
	}

	if err := watcher.Start(); err != nil {
		b.Fatalf("Failed to start watcher: %v", err)
	}
	time.Sleep(50 * time.Millisecond) // Stabilization

	var processed atomic.Int64

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()

		// Same logic as baseline
		for j := 0; j < targetRPS; j++ {
			processed.Add(1)
			_ = "request processed"
		}

		elapsed := time.Since(start)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
	}

	b.StopTimer()
	b.Logf("Target: %d RPS, Processed: %d requests", targetRPS, processed.Load())
	b.Logf("Config reloads: %d", configReloads.Load())
}
