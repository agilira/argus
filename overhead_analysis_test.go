// overhead_analysis_test.go: Testing Argus Overhead
//
// Copyright (c) 2025 AGILira
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

// Benchmark specifico per misurare l'overhead su singole operazioni di logging
func BenchmarkArgus_PerLogEntryOverhead(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "per_entry_overhead")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

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
	// Simula una singola operazione di log (come in Iris)
	message := "User request processed successfully"
	level := "INFO"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Simula il costo minimo di un log entry
		_ = level + ": " + message
	}
}

func benchmarkSingleLogEntryWithArgus(b *testing.B, configFile string) {
	// Setup Argus in background
	config := Config{
		PollInterval:         100 * time.Millisecond, // Poll tipico production
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer watcher.Stop()

	var configReloads atomic.Int64
	watcher.Watch(configFile, func(event ChangeEvent) {
		configReloads.Add(1)
	})

	watcher.Start()
	time.Sleep(50 * time.Millisecond) // Stabilizzazione

	// Simula la stessa operazione di log ma con Argus attivo
	message := "User request processed successfully"
	level := "INFO"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Stessa operazione del baseline
		_ = level + ": " + message
	}

	b.StopTimer()
	b.Logf("Config reloads during test: %d", configReloads.Load())
}

// Benchmark per calcolare l'overhead esatto per richiesta HTTP
func BenchmarkArgus_HTTPRequestOverhead(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "http_overhead")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

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
	// Simula una richiesta HTTP tipica con logging (come in Iris)
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
	// Setup Argus (come verrebbe fatto in Iris)
	config := Config{
		PollInterval:         200 * time.Millisecond, // Polling meno aggressivo per production
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer watcher.Stop()

	var configReloads atomic.Int64
	watcher.Watch(configFile, func(event ChangeEvent) {
		configReloads.Add(1)
		// Simula il costo di riconfigurare il logger in Iris
		time.Sleep(100 * time.Microsecond) // 0.1ms tipico per reload config
	})

	watcher.Start()
	time.Sleep(100 * time.Millisecond) // Stabilizzazione

	var requestCount atomic.Int64

	processHTTPRequest := func() {
		requestCount.Add(1)
		// Stessa logica del baseline - Argus lavora in background
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

// Benchmark per calcolare l'overhead in throughput reale
func BenchmarkArgus_ThroughputImpact(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "throughput_impact")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

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

		// Simula 1 secondo di richieste al targetRPS
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
		PollInterval:         100 * time.Millisecond, // 10 volte al secondo
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer watcher.Stop()

	var configReloads atomic.Int64
	watcher.Watch(configFile, func(event ChangeEvent) {
		configReloads.Add(1)
	})

	watcher.Start()
	time.Sleep(50 * time.Millisecond) // Stabilizzazione

	var processed atomic.Int64

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		start := time.Now()

		// Stessa logica del baseline
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
