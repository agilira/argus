// low_file_test.go: Testing Argus Low File Count Handling
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Test ottimizzato per scenari 1-2 files (caso più comune)
func TestBoreasLite_LowFileCount(b *testing.T) {
	tempDir, err := os.MkdirTemp("", "low_file_test")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	config := Config{
		PollInterval:    20 * time.Millisecond, // Polling veloce per test
		CacheTTL:        10 * time.Millisecond,
		MaxWatchedFiles: 10,
	}

	b.Logf("=== TESTING OPTIMIZED 1-2 FILE SCENARIOS ===")

	// Test 1: Singolo file (scenario più comune)
	b.Run("Single_File", func(b *testing.T) {
		testLowFileCount(b, tempDir, config, 1)
	})

	// Test 2: Due file (secondo caso più comune)
	b.Run("Two_Files", func(b *testing.T) {
		testLowFileCount(b, tempDir, config, 2)
	})
}

func testLowFileCount(t *testing.T, tempDir string, config Config, fileCount int) {
	watcher := New(config)
	defer watcher.Stop()

	var eventCount atomic.Int64
	filePaths := make([]string, fileCount)

	callback := func(event ChangeEvent) {
		eventCount.Add(1)
		t.Logf("Event: %s", filepath.Base(event.Path))
	}

	// Crea e registra files
	for i := 0; i < fileCount; i++ {
		filePath := filepath.Join(tempDir, fmt.Sprintf("config_%d.json", i))
		filePaths[i] = filePath

		// Crea file
		content := fmt.Sprintf(`{"id": %d, "value": "initial"}`, i)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// Registra nel watcher
		if err := watcher.Watch(filePath, callback); err != nil {
			t.Fatal(err)
		}
	}

	watcher.Start()
	time.Sleep(50 * time.Millisecond) // Inizializzazione

	// Misura latenza per modifiche singole
	iterations := 10
	totalDuration := time.Duration(0)

	for iter := 0; iter < iterations; iter++ {
		initialCount := eventCount.Load()

		startTime := time.Now()

		// Modifica tutti i file rapidamente (simula scenario reale)
		for i, filePath := range filePaths {
			content := fmt.Sprintf(`{"id": %d, "value": "modified_%d_%d"}`, i, iter, time.Now().UnixNano())
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}

		// Aspetta che tutti gli eventi arrivino
		timeout := 200 * time.Millisecond
		waitStart := time.Now()

		for time.Since(waitStart) < timeout {
			if eventCount.Load() >= initialCount+int64(fileCount) {
				break
			}
			time.Sleep(1 * time.Millisecond)
		}

		iterDuration := time.Since(startTime)
		totalDuration += iterDuration

		if eventCount.Load() < initialCount+int64(fileCount) {
			t.Errorf("Iteration %d: Missing events. Expected %d, got %d",
				iter, fileCount, eventCount.Load()-initialCount)
		}
	}

	avgLatency := totalDuration / time.Duration(iterations)
	totalEvents := eventCount.Load()

	// BoreasLite stats
	var stats map[string]int64
	if watcher.eventRing != nil {
		stats = watcher.eventRing.Stats()
	}

	t.Logf("=== %d FILE(S) PERFORMANCE RESULTS ===", fileCount)
	t.Logf("Total events: %d", totalEvents)
	t.Logf("Average latency per iteration: %v", avgLatency)
	t.Logf("Latency per event: %v", avgLatency/time.Duration(fileCount))

	if stats != nil {
		t.Logf("BoreasLite: processed=%d, dropped=%d",
			stats["items_processed"], stats["items_dropped"])

		if stats["items_dropped"] > 0 {
			t.Errorf("Events dropped: %d", stats["items_dropped"])
		}
	}

	// Per 1-2 files, dovremmo avere latenza molto bassa
	maxExpectedLatency := 50 * time.Millisecond
	if avgLatency > maxExpectedLatency {
		t.Logf("WARNING: High latency %v for %d files (expected <%v)",
			avgLatency, fileCount, maxExpectedLatency)
	} else {
		t.Logf("✅ Excellent latency %v for %d files", avgLatency, fileCount)
	}
}
