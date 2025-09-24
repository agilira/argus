// low_file_test.go: Testing Argus Low File Count Handling
//
// Copyright (c) 2025 AGILira - A. Giordano
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

// Test optimized handling for low file counts (1-2 files)
func TestBoreasLite_LowFileCount(b *testing.T) {
	tempDir, err := os.MkdirTemp("", "low_file_test")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Failed to remove tempDir: %v", err)
		}
	}()

	config := Config{
		PollInterval:    20 * time.Millisecond, // Polling more frequently for test
		CacheTTL:        10 * time.Millisecond,
		MaxWatchedFiles: 10,
	}

	b.Logf("=== TESTING OPTIMIZED 1-2 FILE SCENARIOS ===")

	// Test 1: Single file (most common scenario)
	b.Run("Single_File", func(b *testing.T) {
		testLowFileCount(b, tempDir, config, 1)
	})

	// Test 2: Two files (second most common case)
	b.Run("Two_Files", func(b *testing.T) {
		testLowFileCount(b, tempDir, config, 2)
	})
}

func testLowFileCount(t *testing.T, tempDir string, config Config, fileCount int) {
	watcher := New(config)
	defer func() {
		if err := watcher.Stop(); err != nil {
			t.Logf("Failed to stop watcher: %v", err)
		}
	}()

	var eventCount atomic.Int64
	filePaths := make([]string, fileCount)

	callback := func(event ChangeEvent) {
		eventCount.Add(1)
		t.Logf("Event: %s", filepath.Base(event.Path))
	}

	// Create and register files
	for i := 0; i < fileCount; i++ {
		filePath := filepath.Join(tempDir, fmt.Sprintf("config_%d.json", i))
		filePaths[i] = filePath

		// Create file
		content := fmt.Sprintf(`{"id": %d, "value": "initial"}`, i)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", filePath, err)
		}

		// Register with watcher
		if err := watcher.Watch(filePath, callback); err != nil {
			t.Fatalf("Failed to watch file %s: %v", filePath, err)
		}
	}

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	time.Sleep(50 * time.Millisecond) // Stabilization

	// Measure latency for single changes
	iterations := 10
	totalDuration := time.Duration(0)

	for iter := 0; iter < iterations; iter++ {
		initialCount := eventCount.Load()

		startTime := time.Now()

		// Modify all files rapidly (simulate real-world scenario)
		for i, filePath := range filePaths {
			content := fmt.Sprintf(`{"id": %d, "value": "modified_%d_%d"}`, i, iter, time.Now().UnixNano())
			if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}

		// Wait for all events to arrive
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

	// For 1-2 files, we should have very low latency
	maxExpectedLatency := 50 * time.Millisecond
	if avgLatency > maxExpectedLatency {
		t.Logf("WARNING: High latency %v for %d files (expected <%v)",
			avgLatency, fileCount, maxExpectedLatency)
	} else {
		t.Logf("✅ Excellent latency %v for %d files", avgLatency, fileCount)
	}
}
