// production_overhead_test.go: Testing Argus Production Overhead
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// Benchmark to measure Argus overhead in production-like scenarios
func BenchmarkArgus_ProductionOverhead(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "production_overhead")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "logger.json")
	initialConfig := `{"level": "info", "output": "stdout", "format": "json"}`

	if err := os.WriteFile(configFile, []byte(initialConfig), 0644); err != nil {
		b.Fatal(err)
	}

	// Test 1: Baseline without file watching
	b.Run("Baseline_NoWatching", func(b *testing.B) {
		benchmarkBaselineNoWatching(b)
	})

	// Test 2: With Argus active but no changes (normal scenario)
	b.Run("WithArgus_NoChanges", func(b *testing.B) {
		benchmarkWithArgusNoChanges(b, configFile)
	})

	// Test 3: With Argus and changes every 5 seconds (worst case)
	b.Run("WithArgus_Changes5s", func(b *testing.B) {
		benchmarkWithArgusChanges(b, configFile, 5*time.Second)
	})

	// Test 4: CPU overhead of Argus in background
	b.Run("CPU_Overhead", func(b *testing.B) {
		benchmarkCPUOverhead(b, configFile)
	})
}

// Baseline: simulates logging without Argus
func benchmarkBaselineNoWatching(b *testing.B) {
	var logCount atomic.Int64

	// Simulate logging operation (like Iris would)
	logEntry := func(level string, message string) {
		logCount.Add(1)
		// Simulate the cost of a log entry
		_ = level + ": " + message
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logEntry("INFO", "Request processed")
	}

	b.StopTimer()
	b.Logf("Logged %d entries", logCount.Load())
}

// With Argus active but no config changes (normal production scenario)
func benchmarkWithArgusNoChanges(b *testing.B, configFile string) {
	var logCount atomic.Int64
	var configReloads atomic.Int64

	// Setup Argus
	config := Config{
		PollInterval:         100 * time.Millisecond, // Poll every 100ms (typical)
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Logf("Failed to stop watcher: %v", err)
		}
	}()
	// Config callback (viene chiamato solo su cambiamenti)
	if err := watcher.Watch(configFile, func(event ChangeEvent) {
		configReloads.Add(1)
		// Simulate config reload
		time.Sleep(1 * time.Millisecond) // Cost of parsing config
	}); err != nil {
		b.Fatalf("Failed to watch config file: %v", err)
	}

	if err := watcher.Start(); err != nil {
		b.Fatalf("Failed to start watcher: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // Stabilization
	logEntry := func(level string, message string) {
		logCount.Add(1)
		// Simulate the cost of a log entry (identical to baseline)
		_ = level + ": " + message
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logEntry("INFO", "Request processed")
	}

	b.StopTimer()
	b.Logf("Logged %d entries, Config reloads: %d", logCount.Load(), configReloads.Load())
}

// With Argus and config changes every 5 seconds (worst case scenario)
func benchmarkWithArgusChanges(b *testing.B, configFile string, changeInterval time.Duration) {
	var logCount atomic.Int64
	var configReloads atomic.Int64

	// Setup Argus
	config := Config{
		PollInterval:         100 * time.Millisecond,
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Logf("Failed to stop watcher: %v", err)
		}
	}()

	// Config callback
	if err := watcher.Watch(configFile, func(event ChangeEvent) {
		configReloads.Add(1)
		// Simulate config reload (parsing JSON, validation, etc.)
		time.Sleep(2 * time.Millisecond) // Cost of parsing config
	}); err != nil {
		b.Fatalf("Failed to watch config file: %v", err)
	}

	if err := watcher.Start(); err != nil {
		b.Fatalf("Failed to start watcher: %v", err)
	}

	// Goroutine that simulates config changes every 5 seconds
	stopChanges := make(chan bool)
	go func() {
		ticker := time.NewTicker(changeInterval)
		defer ticker.Stop()

		counter := 0
		for {
			select {
			case <-ticker.C:
				counter++
				newConfig := `{"level": "debug", "output": "file", "counter": ` +
					string(rune('0'+counter%10)) + `}`
				if err := os.WriteFile(configFile, []byte(newConfig), 0644); err != nil {
					b.Logf("Failed to write config file: %v", err)
				}
			case <-stopChanges:
				return
			}
		}
	}()

	defer func() {
		stopChanges <- true
		time.Sleep(10 * time.Millisecond) // Cleanup
	}()

	// Simulate intensive logging
	logEntry := func(level string, message string) {
		logCount.Add(1)
		_ = level + ": " + message
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logEntry("INFO", "Request processed")
	}

	b.StopTimer()
	b.Logf("Logged %d entries, Config reloads: %d", logCount.Load(), configReloads.Load())
}

// Calculate the CPU overhead of Argus in the background
func benchmarkCPUOverhead(b *testing.B, configFile string) {
	// Setup Argus
	config := Config{
		PollInterval:         100 * time.Millisecond, // 10 poll/second
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Logf("Failed to stop watcher: %v", err)
		}
	}()

	var configChecks atomic.Int64
	if err := watcher.Watch(configFile, func(event ChangeEvent) {
		configChecks.Add(1)
	}); err != nil {
		b.Fatalf("Failed to watch config file: %v", err)
	}

	if err := watcher.Start(); err != nil {
		b.Fatalf("Failed to start watcher: %v", err)
	}

	// Misura CPU time prima
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	start := time.Now()

	b.ResetTimer()

	// Simulates 1 second of activity (10 poll cycles)
	time.Sleep(1 * time.Second)

	b.StopTimer()

	// Measures CPU time after
	elapsed := time.Since(start)
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)

	b.Logf("Background overhead for 1 second:")
	b.Logf("  Time elapsed: %v", elapsed)
	b.Logf("  Memory allocated: %d bytes", m2.TotalAlloc-m1.TotalAlloc)
	b.Logf("  Config checks: %d", configChecks.Load())

	// Calculate overhead per operation
	if b.N > 0 {
		overheadPerOp := elapsed / time.Duration(b.N)
		b.Logf("  Overhead per operation: %v", overheadPerOp)
	}
}

// Benchmark to measure Argus overhead in production-like scenarios
func BenchmarkArgus_Every5Seconds(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "every5s")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "config.json")
	if err := os.WriteFile(configFile, []byte(`{"level": "info"}`), 0644); err != nil {
		b.Fatal(err)
	}

	b.Run("PollCost_Per5Seconds", func(b *testing.B) {
		benchmarkPollCostPer5Seconds(b, configFile)
	})

	b.Run("ConfigChange_Cost", func(b *testing.B) {
		benchmarkConfigChangeCost(b, configFile)
	})
}

func benchmarkPollCostPer5Seconds(b *testing.B, configFile string) {
	config := Config{
		PollInterval:         100 * time.Millisecond, // 10 poll/second
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Logf("Failed to stop watcher: %v", err)
		}
	}()

	var pollCount atomic.Int64
	if err := watcher.Watch(configFile, func(event ChangeEvent) {
		pollCount.Add(1)
	}); err != nil {
		b.Fatalf("Failed to watch config file: %v", err)
	}

	if err := watcher.Start(); err != nil {
		b.Fatalf("Failed to start watcher: %v", err)
	}

	b.ResetTimer()

	// Simulates exactly 5 seconds of polling (50 poll operations)
	for i := 0; i < b.N; i++ {
		start := time.Now()
		time.Sleep(5 * time.Second)
		elapsed := time.Since(start)

		if i == 0 { // Log only the first to avoid cluttering output
			b.Logf("5 seconds of polling: %v elapsed", elapsed)
		}
	}

	b.StopTimer()
	b.Logf("Poll operations in test: %d", pollCount.Load())
}

func benchmarkConfigChangeCost(b *testing.B, configFile string) {
	config := Config{
		PollInterval:         50 * time.Millisecond,
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Logf("Failed to stop watcher: %v", err)
		}
	}()

	var changeCount atomic.Int64
	if err := watcher.Watch(configFile, func(event ChangeEvent) {
		changeCount.Add(1)
		// Simulate realistic config parsing cost
		time.Sleep(1 * time.Millisecond)
	}); err != nil {
		b.Fatalf("Failed to watch config file: %v", err)
	}

	if err := watcher.Start(); err != nil {
		b.Fatalf("Failed to start watcher: %v", err)
	}
	time.Sleep(10 * time.Millisecond) // Stabilization

	b.ResetTimer()

	// Measures the cost of a single config change
	for i := 0; i < b.N; i++ {
		// Modifica config
		newConfig := `{"level": "debug", "iteration": ` + string(rune('0'+(i%10))) + `}`
		if err := os.WriteFile(configFile, []byte(newConfig), 0644); err != nil {
			b.Logf("Failed to write config file: %v", err)
		}

		// Wait for it to be processed
		time.Sleep(200 * time.Millisecond)
	}

	b.StopTimer()
	b.Logf("Config changes processed: %d", changeCount.Load())
}
