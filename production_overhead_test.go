// production_overhead_test.go: Testing Argus Production Overhead
//
// Copyright (c) 2025 AGILira
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

// Benchmark per misurare l'overhead di Argus su un logger production-like
func BenchmarkArgus_ProductionOverhead(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "production_overhead")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "logger.json")
	initialConfig := `{"level": "info", "output": "stdout", "format": "json"}`

	if err := os.WriteFile(configFile, []byte(initialConfig), 0644); err != nil {
		b.Fatal(err)
	}

	// Test 1: Baseline senza file watching
	b.Run("Baseline_NoWatching", func(b *testing.B) {
		benchmarkBaselineNoWatching(b)
	})

	// Test 2: Con Argus attivo ma senza cambiamenti (scenario normale)
	b.Run("WithArgus_NoChanges", func(b *testing.B) {
		benchmarkWithArgusNoChanges(b, configFile)
	})

	// Test 3: Con Argus e cambiamenti ogni 5 secondi (worst case)
	b.Run("WithArgus_Changes5s", func(b *testing.B) {
		benchmarkWithArgusChanges(b, configFile, 5*time.Second)
	})

	// Test 4: CPU overhead di Argus in background
	b.Run("CPU_Overhead", func(b *testing.B) {
		benchmarkCPUOverhead(b, configFile)
	})
}

// Baseline: simula logger senza file watching
func benchmarkBaselineNoWatching(b *testing.B) {
	var logCount atomic.Int64

	// Simula logging operation (come farebbe Iris)
	logEntry := func(level string, message string) {
		logCount.Add(1)
		// Simula il costo di un log entry
		_ = level + ": " + message
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logEntry("INFO", "Request processed")
	}

	b.StopTimer()
	b.Logf("Logged %d entries", logCount.Load())
}

// Con Argus attivo ma senza config changes (scenario normale production)
func benchmarkWithArgusNoChanges(b *testing.B, configFile string) {
	var logCount atomic.Int64
	var configReloads atomic.Int64

	// Setup Argus
	config := Config{
		PollInterval:         100 * time.Millisecond, // Poll ogni 100ms (tipico)
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer watcher.Stop()

	// Config callback (viene chiamato solo su cambiamenti)
	watcher.Watch(configFile, func(event ChangeEvent) {
		configReloads.Add(1)
		// Simula reload config
		time.Sleep(1 * time.Millisecond) // Costo parsing config
	})

	watcher.Start()
	time.Sleep(10 * time.Millisecond) // Stabilizzazione

	// Simula logging con Argus attivo in background
	logEntry := func(level string, message string) {
		logCount.Add(1)
		// Simula il costo di un log entry (identico al baseline)
		_ = level + ": " + message
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logEntry("INFO", "Request processed")
	}

	b.StopTimer()
	b.Logf("Logged %d entries, Config reloads: %d", logCount.Load(), configReloads.Load())
}

// Con Argus e config changes ogni 5 secondi (worst case scenario)
func benchmarkWithArgusChanges(b *testing.B, configFile string, changeInterval time.Duration) {
	var logCount atomic.Int64
	var configReloads atomic.Int64

	// Setup Argus
	config := Config{
		PollInterval:         100 * time.Millisecond,
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer watcher.Stop()

	// Config callback
	watcher.Watch(configFile, func(event ChangeEvent) {
		configReloads.Add(1)
		// Simula reload config (parsing JSON, validation, etc.)
		time.Sleep(2 * time.Millisecond) // Costo realistico parsing config
	})

	watcher.Start()

	// Goroutine che simula config changes ogni 5 secondi
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
				os.WriteFile(configFile, []byte(newConfig), 0644)
			case <-stopChanges:
				return
			}
		}
	}()

	defer func() {
		stopChanges <- true
		time.Sleep(10 * time.Millisecond) // Cleanup
	}()

	// Simula logging intensivo
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

// Misura l'overhead CPU di Argus in background
func benchmarkCPUOverhead(b *testing.B, configFile string) {
	// Setup Argus
	config := Config{
		PollInterval:         100 * time.Millisecond, // 10 poll/secondo
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer watcher.Stop()

	var configChecks atomic.Int64
	watcher.Watch(configFile, func(event ChangeEvent) {
		configChecks.Add(1)
	})

	watcher.Start()

	// Misura CPU time prima
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	start := time.Now()

	b.ResetTimer()

	// Simula 1 secondo di attività (10 poll cycles)
	time.Sleep(1 * time.Second)

	b.StopTimer()

	// Misura CPU time dopo
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

// Benchmark per misurare il costo specifico di ogni 5 secondi
func BenchmarkArgus_Every5Seconds(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "every5s")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

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
		PollInterval:         100 * time.Millisecond, // 10 poll/secondo
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer watcher.Stop()

	var pollCount atomic.Int64
	watcher.Watch(configFile, func(event ChangeEvent) {
		pollCount.Add(1)
	})

	watcher.Start()

	b.ResetTimer()

	// Simula esattamente 5 secondi di polling (50 poll operations)
	for i := 0; i < b.N; i++ {
		start := time.Now()
		time.Sleep(5 * time.Second)
		elapsed := time.Since(start)

		if i == 0 { // Log solo il primo per non sporcare l'output
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
	defer watcher.Stop()

	var changeCount atomic.Int64
	watcher.Watch(configFile, func(event ChangeEvent) {
		changeCount.Add(1)
		// Simula costo parsing config realistico
		time.Sleep(1 * time.Millisecond)
	})

	watcher.Start()
	time.Sleep(10 * time.Millisecond) // Stabilizzazione

	b.ResetTimer()

	// Misura il costo di una singola config change
	for i := 0; i < b.N; i++ {
		// Modifica config
		newConfig := `{"level": "debug", "iteration": ` + string(rune('0'+(i%10))) + `}`
		os.WriteFile(configFile, []byte(newConfig), 0644)

		// Aspetta che venga processata
		time.Sleep(200 * time.Millisecond)
	}

	b.StopTimer()
	b.Logf("Config changes processed: %d", changeCount.Load())
}
