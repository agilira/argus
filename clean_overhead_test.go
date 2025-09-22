// clean_overhead_test.go: Benchmark to measure Argus minimal overhead
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

// BenchmarkClean_MinimalOverhead tracks real overhead
func BenchmarkClean_MinimalOverhead(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "clean_overhead")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configFile, []byte(`{"level": "info"}`), 0644); err != nil {
		b.Fatal(err)
	}

	b.Run("Baseline_Pure", func(b *testing.B) {
		benchmarkPureBaseline(b)
	})

	b.Run("WithArgus_Clean", func(b *testing.B) {
		benchmarkWithArgusClean(b, configFile)
	})
}

// Baseline without atomics or contention
func benchmarkPureBaseline(b *testing.B) {
	// Logging without contention or shared atomics
	logMessage := func() {
		// Equivalent operation without shared atomics
		_ = "INFO: Request processed"
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logMessage()
	}
}

// With Argus but without contention
func benchmarkWithArgusClean(b *testing.B, configFile string) {
	// Setup Argus with optimal configuration
	config := Config{
		PollInterval:         100 * time.Millisecond,
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer watcher.Stop()

	// Simple callback without shared atomics
	watcher.Watch(configFile, func(event ChangeEvent) {
		// Minimal callback
	})

	watcher.Start()
	time.Sleep(10 * time.Millisecond) // Stabilize

	// Same operation as baseline
	logMessage := func() {
		_ = "INFO: Request processed"
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logMessage()
	}
}

// BenchmarkIsolatedComponents measures overhead for isolated components
func BenchmarkIsolatedComponents(b *testing.B) {
	b.Run("AtomicLoad_Only", func(b *testing.B) {
		var flag atomic.Bool
		flag.Store(true)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = flag.Load()
		}
	})

	b.Run("StringConcat_Only", func(b *testing.B) {
		level := "INFO"
		message := "Request processed"

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = level + ": " + message
		}
	})

	b.Run("Combined_NoArgus", func(b *testing.B) {
		var flag atomic.Bool
		flag.Store(true)
		level := "INFO"
		message := "Request processed"

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if flag.Load() {
				_ = level + ": " + message
			}
		}
	})
}
