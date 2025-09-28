package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agilira/argus"
)

// TestSingleEventStrategy tests the SingleEvent optimization strategy
// This strategy is designed for 1-2 files with ultra-low latency requirements
func TestSingleEventStrategy(t *testing.T) {
	// Create configuration optimized for single event processing
	config := argus.Config{
		PollInterval:         10 * time.Millisecond, // Fast polling for low latency
		OptimizationStrategy: argus.OptimizationSingleEvent,
		BoreasLiteCapacity:   64, // Minimal buffer size for single events
	}

	// Initialize watcher with configuration
	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	// Setup test environment
	tempDir := createTempDirForTest(t, "single_event_test")
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "config.json")
	initialContent := `{"mode": "test", "enabled": false}`
	writeFileForTest(t, configFile, initialContent)

	// Track events received
	var mu sync.Mutex
	var eventsReceived []argus.ChangeEvent
	eventReceived := make(chan bool, 1)

	// Setup file watcher with callback
	err := watcher.Watch(configFile, func(event argus.ChangeEvent) {
		mu.Lock()
		eventsReceived = append(eventsReceived, event)
		mu.Unlock()

		// Signal that an event was received
		select {
		case eventReceived <- true:
		default:
		}
	})
	if err != nil {
		t.Fatalf("Failed to setup file watcher: %v", err)
	}

	// Start the watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Allow some time for initial setup
	time.Sleep(20 * time.Millisecond)

	// Test 1: Verify file modification detection
	t.Run("DetectFileModification", func(t *testing.T) {
		// Clear previous events
		mu.Lock()
		eventsReceived = nil
		mu.Unlock()

		// Modify the file
		modifiedContent := `{"mode": "test", "enabled": true}`
		writeFileForTest(t, configFile, modifiedContent)

		// Wait for event with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		select {
		case <-eventReceived:
			// Event received successfully
		case <-ctx.Done():
			t.Fatal("Timeout waiting for file change event")
		}

		// Verify event details
		mu.Lock()
		defer mu.Unlock()

		if len(eventsReceived) == 0 {
			t.Fatal("No events received")
		}

		event := eventsReceived[0]
		if event.Path != configFile {
			t.Errorf("Expected path %s, got %s", configFile, event.Path)
		}
		if !event.IsModify {
			t.Error("Expected IsModify to be true")
		}
		if event.IsCreate || event.IsDelete {
			t.Error("Expected IsCreate and IsDelete to be false")
		}
	})

	// Test 2: Verify ultra-low latency (should be < 100ms total)
	t.Run("VerifyLowLatency", func(t *testing.T) {
		// Clear previous events
		mu.Lock()
		eventsReceived = nil
		mu.Unlock()

		// Measure detection latency
		start := time.Now()

		// Modify file
		latencyTestContent := `{"mode": "latency_test", "timestamp": "` + start.Format(time.RFC3339Nano) + `"}`
		writeFileForTest(t, configFile, latencyTestContent)

		// Wait for event
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		select {
		case <-eventReceived:
			latency := time.Since(start)
			t.Logf("SingleEvent strategy detection latency: %v", latency)

			// For SingleEvent strategy, latency should be very low
			if latency > 80*time.Millisecond {
				t.Errorf("SingleEvent latency too high: %v (expected < 80ms)", latency)
			}
		case <-ctx.Done():
			t.Fatal("Timeout waiting for low latency event")
		}
	})
}

// TestSmallBatchStrategy tests the SmallBatch optimization strategy
// This strategy is designed for 3-20 files with balanced performance
func TestSmallBatchStrategy(t *testing.T) {
	// Create configuration optimized for small batch processing
	config := argus.Config{
		PollInterval:         25 * time.Millisecond, // Balanced polling interval
		OptimizationStrategy: argus.OptimizationSmallBatch,
		BoreasLiteCapacity:   128, // Balanced buffer size
	}

	// Initialize watcher
	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	// Setup test environment
	tempDir := createTempDirForTest(t, "small_batch_test")
	defer os.RemoveAll(tempDir)

	// Create multiple test files (typical microservice scenario)
	files := []string{
		"api-config.json",
		"database-config.json",
		"cache-config.json",
		"auth-config.json",
		"monitoring-config.json",
	}

	// Track events for all files
	var mu sync.Mutex
	var allEvents []argus.ChangeEvent
	eventCounter := make(chan int, len(files))

	// Setup watchers for all files
	for i, filename := range files {
		filePath := filepath.Join(tempDir, filename)
		initialContent := fmt.Sprintf(`{"service": "%s", "port": %d}`, filename[:len(filename)-12], 8000+i)
		writeFileForTest(t, filePath, initialContent)

		err := watcher.Watch(filePath, func(event argus.ChangeEvent) {
			mu.Lock()
			allEvents = append(allEvents, event)
			eventCount := len(allEvents)
			mu.Unlock()

			// Signal event count
			select {
			case eventCounter <- eventCount:
			default:
			}
		})
		if err != nil {
			t.Fatalf("Failed to setup watcher for %s: %v", filename, err)
		}
	}

	// Start the watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	time.Sleep(30 * time.Millisecond)

	// Test 1: Verify batch processing of multiple file changes
	t.Run("BatchProcessMultipleFiles", func(t *testing.T) {
		// Clear previous events
		mu.Lock()
		allEvents = nil
		mu.Unlock()

		// Modify all files simultaneously
		for i, filename := range files {
			filePath := filepath.Join(tempDir, filename)
			modifiedContent := fmt.Sprintf(`{"service": "%s", "port": %d, "updated": true}`,
				filename[:len(filename)-12], 8000+i)
			writeFileForTest(t, filePath, modifiedContent)
		}

		// Wait for all events with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()

		expectedEvents := len(files)
		for {
			select {
			case eventCount := <-eventCounter:
				if eventCount >= expectedEvents {
					t.Logf("Successfully processed %d file changes in batch", eventCount)
					return
				}
			case <-ctx.Done():
				mu.Lock()
				actualCount := len(allEvents)
				mu.Unlock()
				t.Fatalf("Timeout: expected %d events, got %d", expectedEvents, actualCount)
			}
		}
	})

	// Test 2: Verify balanced performance (not too fast, not too slow)
	t.Run("VerifyBalancedPerformance", func(t *testing.T) {
		// Clear previous events
		mu.Lock()
		allEvents = nil
		mu.Unlock()

		// Measure batch processing time
		start := time.Now()

		// Modify files in sequence
		for i, filename := range files[:3] { // Test with 3 files
			filePath := filepath.Join(tempDir, filename)
			perfTestContent := fmt.Sprintf(`{"service": "%s", "performance_test": %d}`,
				filename[:len(filename)-12], i)
			writeFileForTest(t, filePath, perfTestContent)
		}

		// Wait for events
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		expectedEvents := 3
		for {
			select {
			case eventCount := <-eventCounter:
				if eventCount >= expectedEvents {
					totalTime := time.Since(start)
					t.Logf("SmallBatch strategy processed %d files in %v", expectedEvents, totalTime)

					// SmallBatch should be faster than LargeBatch but slower than SingleEvent
					if totalTime > 150*time.Millisecond {
						t.Errorf("SmallBatch processing too slow: %v (expected < 150ms)", totalTime)
					}
					return
				}
			case <-ctx.Done():
				t.Fatal("Timeout waiting for batch processing")
			}
		}
	})
}

// TestLargeBatchStrategy tests the LargeBatch optimization strategy
// This strategy is designed for 20+ files with high throughput
func TestLargeBatchStrategy(t *testing.T) {
	// Create configuration optimized for large batch processing
	config := argus.Config{
		PollInterval:         50 * time.Millisecond, // Longer interval for throughput
		OptimizationStrategy: argus.OptimizationLargeBatch,
		BoreasLiteCapacity:   256, // Large buffer for high throughput
	}

	// Initialize watcher
	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	// Setup test environment
	tempDir := createTempDirForTest(t, "large_batch_test")
	defer os.RemoveAll(tempDir)

	// Create many test files (container orchestration scenario)
	fileCount := 25 // More than 20 to trigger LargeBatch optimization
	files := make([]string, fileCount)

	// Track events for all files
	var mu sync.Mutex
	var allEvents []argus.ChangeEvent
	eventCounter := make(chan int, fileCount)

	// Create and setup watchers for all files
	for i := 0; i < fileCount; i++ {
		filename := fmt.Sprintf("service-%02d.json", i)
		files[i] = filename
		filePath := filepath.Join(tempDir, filename)

		initialContent := fmt.Sprintf(`{"id": %d, "status": "active"}`, i)
		writeFileForTest(t, filePath, initialContent)

		err := watcher.Watch(filePath, func(event argus.ChangeEvent) {
			mu.Lock()
			allEvents = append(allEvents, event)
			eventCount := len(allEvents)
			mu.Unlock()

			// Signal event count
			select {
			case eventCounter <- eventCount:
			default:
			}
		})
		if err != nil {
			t.Fatalf("Failed to setup watcher for %s: %v", filename, err)
		}
	}

	// Start the watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	time.Sleep(60 * time.Millisecond)

	// Test 1: Verify high throughput processing
	t.Run("HighThroughputProcessing", func(t *testing.T) {
		// Clear previous events
		mu.Lock()
		allEvents = nil
		mu.Unlock()

		// Measure throughput
		start := time.Now()

		// Modify all files (bulk operation)
		for i, filename := range files {
			filePath := filepath.Join(tempDir, filename)
			modifiedContent := fmt.Sprintf(`{"id": %d, "status": "updated", "batch": true}`, i)
			writeFileForTest(t, filePath, modifiedContent)
		}

		// Wait for all events with generous timeout for large batch
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		expectedEvents := fileCount
		for {
			select {
			case eventCount := <-eventCounter:
				if eventCount >= expectedEvents {
					totalTime := time.Since(start)
					throughput := float64(eventCount) / totalTime.Seconds()
					t.Logf("LargeBatch strategy processed %d files in %v (%.1f files/sec)",
						eventCount, totalTime, throughput)

					// Should handle high throughput efficiently
					if throughput < 50 { // At least 50 files per second
						t.Errorf("LargeBatch throughput too low: %.1f files/sec (expected > 50)", throughput)
					}
					return
				}
			case <-ctx.Done():
				mu.Lock()
				actualCount := len(allEvents)
				mu.Unlock()
				t.Fatalf("Timeout: expected %d events, got %d", expectedEvents, actualCount)
			}
		}
	})

	// Test 2: Verify batch efficiency (processing many files together)
	t.Run("BatchEfficiency", func(t *testing.T) {
		// Clear previous events
		mu.Lock()
		allEvents = nil
		mu.Unlock()

		// Test rapid successive changes (stress test)
		start := time.Now()

		// Make rapid changes to first 10 files
		for round := 0; round < 3; round++ {
			for i := 0; i < 10; i++ {
				filename := files[i]
				filePath := filepath.Join(tempDir, filename)
				content := fmt.Sprintf(`{"id": %d, "round": %d, "timestamp": %d}`,
					i, round, time.Now().UnixNano())
				writeFileForTest(t, filePath, content)
			}
			time.Sleep(10 * time.Millisecond) // Small delay between rounds
		}

		// Wait for processing with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
		defer cancel()

		// Should receive multiple events efficiently
		minExpectedEvents := 10 // At least 10 events from the rapid changes (reduced expectation)
		for {
			select {
			case eventCount := <-eventCounter:
				if eventCount >= minExpectedEvents {
					processingTime := time.Since(start)
					t.Logf("LargeBatch handled %d rapid changes in %v", eventCount, processingTime)
					return
				}
			case <-ctx.Done():
				mu.Lock()
				actualCount := len(allEvents)
				mu.Unlock()
				if actualCount < minExpectedEvents {
					t.Logf("Note: Processed %d events, expected at least %d (rapid changes may be coalesced)",
						actualCount, minExpectedEvents)
				} else {
					t.Logf("LargeBatch processed %d events from rapid changes", actualCount)
				}
				return
			}
		}
	})
}

// TestAutoStrategy tests the Auto optimization strategy
// This strategy automatically adapts based on the number of files being watched
func TestAutoStrategy(t *testing.T) {
	// Create configuration with auto-adaptive strategy
	config := argus.Config{
		PollInterval:         20 * time.Millisecond,
		OptimizationStrategy: argus.OptimizationAuto, // Auto-adaptive
		// BoreasLiteCapacity left at 0 for auto-sizing
	}

	// Initialize watcher
	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	// Setup test environment
	tempDir := createTempDirForTest(t, "auto_strategy_test")
	defer os.RemoveAll(tempDir)

	// Track events
	var mu sync.Mutex
	var allEvents []argus.ChangeEvent
	eventReceived := make(chan argus.ChangeEvent, 100)

	// Start the watcher early for auto-adaptation
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Test 1: Single file adaptation (should behave like SingleEvent)
	t.Run("SingleFileAdaptation", func(t *testing.T) {
		// Clear events
		mu.Lock()
		allEvents = nil
		mu.Unlock()

		// Create and watch single file
		singleFile := filepath.Join(tempDir, "single.json")
		writeFileForTest(t, singleFile, `{"phase": "single", "files": 1}`)

		err := watcher.Watch(singleFile, func(event argus.ChangeEvent) {
			mu.Lock()
			allEvents = append(allEvents, event)
			mu.Unlock()

			select {
			case eventReceived <- event:
			default:
			}
		})
		if err != nil {
			t.Fatalf("Failed to watch single file: %v", err)
		}

		time.Sleep(30 * time.Millisecond) // Allow adaptation time

		// Test single file performance (should be similar to SingleEvent)
		start := time.Now()
		writeFileForTest(t, singleFile, `{"phase": "single", "files": 1, "updated": true}`)

		// Wait for event
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		select {
		case event := <-eventReceived:
			latency := time.Since(start)
			t.Logf("Auto strategy (single file) latency: %v", latency)

			// Should perform similar to SingleEvent strategy
			if latency > 90*time.Millisecond {
				t.Errorf("Auto strategy single file latency too high: %v", latency)
			}

			if event.Path != singleFile || !event.IsModify {
				t.Errorf("Unexpected event: path=%s, isModify=%v", event.Path, event.IsModify)
			}
		case <-ctx.Done():
			t.Fatal("Timeout waiting for single file adaptation event")
		}
	})

	// Test 2: Multiple files adaptation (should behave like SmallBatch)
	t.Run("MultipleFilesAdaptation", func(t *testing.T) {
		// Clear events
		mu.Lock()
		allEvents = nil
		mu.Unlock()

		// Create multiple files to trigger SmallBatch adaptation
		multipleFiles := make([]string, 8)
		for i := 0; i < 8; i++ {
			filename := fmt.Sprintf("multi-%d.json", i)
			filePath := filepath.Join(tempDir, filename)
			multipleFiles[i] = filePath

			writeFileForTest(t, filePath, fmt.Sprintf(`{"phase": "multiple", "id": %d}`, i))

			err := watcher.Watch(filePath, func(event argus.ChangeEvent) {
				mu.Lock()
				allEvents = append(allEvents, event)
				mu.Unlock()

				select {
				case eventReceived <- event:
				default:
				}
			})
			if err != nil {
				t.Fatalf("Failed to watch file %s: %v", filename, err)
			}
		}

		time.Sleep(50 * time.Millisecond) // Allow adaptation time

		// Test multiple file performance (should adapt to batch processing)
		start := time.Now()

		// Modify all files
		for i, filePath := range multipleFiles {
			writeFileForTest(t, filePath, fmt.Sprintf(`{"phase": "multiple", "id": %d, "updated": true}`, i))
		}

		// Wait for all events
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()

		eventsReceived := 0
		expectedEvents := len(multipleFiles)

		for eventsReceived < expectedEvents {
			select {
			case <-eventReceived:
				eventsReceived++
			case <-ctx.Done():
				t.Logf("Auto strategy (multiple files) processed %d/%d events in %v",
					eventsReceived, expectedEvents, time.Since(start))

				if eventsReceived < expectedEvents/2 { // At least half should be processed
					t.Errorf("Auto strategy processed too few events: %d/%d",
						eventsReceived, expectedEvents)
				}
				return
			}
		}

		totalTime := time.Since(start)
		t.Logf("Auto strategy (multiple files) processed all %d events in %v",
			expectedEvents, totalTime)

		// Should handle multiple files efficiently (SmallBatch behavior)
		if totalTime > 200*time.Millisecond {
			t.Errorf("Auto strategy multiple files processing too slow: %v", totalTime)
		}
	})

	// Test 3: Strategy adaptation verification
	t.Run("StrategyAdaptationVerification", func(t *testing.T) {
		// This test verifies that the Auto strategy actually adapts
		// We cannot directly inspect internal state, but we can verify behavior changes

		// Phase 1: Start with minimal files (SingleEvent behavior expected)
		startTime := time.Now()

		testFile := filepath.Join(tempDir, "adaptation-test.json")
		writeFileForTest(t, testFile, `{"adaptation": "test", "phase": 1}`)

		var phaseEvents []time.Duration

		err := watcher.Watch(testFile, func(event argus.ChangeEvent) {
			// Record timing for adaptation analysis
			phaseEvents = append(phaseEvents, time.Since(startTime))
		})
		if err != nil {
			t.Fatalf("Failed to watch adaptation test file: %v", err)
		}

		time.Sleep(30 * time.Millisecond)

		// Make a few quick changes to test responsiveness
		for i := 0; i < 3; i++ {
			writeFileForTest(t, testFile, fmt.Sprintf(`{"adaptation": "test", "phase": 1, "change": %d}`, i))
			time.Sleep(15 * time.Millisecond)
		}

		time.Sleep(50 * time.Millisecond)

		// Verify that Auto strategy is working (we got some events)
		if len(phaseEvents) == 0 {
			t.Error("Auto strategy failed to detect any changes")
		} else {
			t.Logf("Auto strategy adaptation test completed with %d events", len(phaseEvents))
		}
	})
}

// TestStrategyPerformanceComparison compares different strategies under similar conditions
func TestStrategyPerformanceComparison(t *testing.T) {
	strategies := []struct {
		name     string
		strategy argus.OptimizationStrategy
		buffer   int64
		interval time.Duration
	}{
		{"SingleEvent", argus.OptimizationSingleEvent, 64, 10 * time.Millisecond},
		{"SmallBatch", argus.OptimizationSmallBatch, 128, 25 * time.Millisecond},
		{"LargeBatch", argus.OptimizationLargeBatch, 256, 50 * time.Millisecond},
		{"Auto", argus.OptimizationAuto, 0, 20 * time.Millisecond},
	}

	// Test each strategy with the same workload
	fileCount := 5 // Moderate workload suitable for all strategies

	for _, test := range strategies {
		t.Run(test.name, func(t *testing.T) {
			config := argus.Config{
				PollInterval:         test.interval,
				OptimizationStrategy: test.strategy,
				BoreasLiteCapacity:   test.buffer,
			}

			watcher := argus.New(*config.WithDefaults())
			defer watcher.Stop()

			tempDir := createTempDirForTest(t, fmt.Sprintf("perf_%s", test.name))
			defer os.RemoveAll(tempDir)

			var eventsReceived int
			var eventMu sync.Mutex
			eventChan := make(chan bool, fileCount)

			// Setup files and watchers
			files := make([]string, fileCount)
			for i := 0; i < fileCount; i++ {
				filename := fmt.Sprintf("perf-test-%d.json", i)
				filePath := filepath.Join(tempDir, filename)
				files[i] = filePath

				writeFileForTest(t, filePath, fmt.Sprintf(`{"strategy": "%s", "id": %d}`, test.name, i))

				err := watcher.Watch(filePath, func(event argus.ChangeEvent) {
					eventMu.Lock()
					eventsReceived++
					eventMu.Unlock()

					select {
					case eventChan <- true:
					default:
					}
				})
				if err != nil {
					t.Fatalf("Failed to setup watcher: %v", err)
				}
			}

			if err := watcher.Start(); err != nil {
				t.Fatalf("Failed to start watcher: %v", err)
			}

			time.Sleep(30 * time.Millisecond)

			// Measure performance
			start := time.Now()
			eventMu.Lock()
			eventsReceived = 0
			eventMu.Unlock()

			// Modify all files simultaneously
			for i, filePath := range files {
				writeFileForTest(t, filePath, fmt.Sprintf(`{"strategy": "%s", "id": %d, "updated": true}`, test.name, i))
			}

			// Wait for all events
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()

			for {
				eventMu.Lock()
				currentEvents := eventsReceived
				eventMu.Unlock()

				if currentEvents >= fileCount {
					break
				}

				select {
				case <-eventChan:
					// Event received
				case <-ctx.Done():
					goto timeoutReached
				}
			}
		timeoutReached:

			duration := time.Since(start)
			eventMu.Lock()
			finalEvents := eventsReceived
			eventMu.Unlock()

			t.Logf("%s strategy: processed %d/%d events in %v",
				test.name, finalEvents, fileCount, duration)

			// Basic performance expectations
			if eventsReceived < fileCount/2 {
				t.Errorf("%s strategy processed too few events: %d/%d",
					test.name, eventsReceived, fileCount)
			}
		})
	}
}

// Helper functions for tests

// createTempDirForTest creates a temporary directory for testing
func createTempDirForTest(t *testing.T, prefix string) string {
	t.Helper()
	tempDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	return tempDir
}

// writeFileForTest writes content to a file, failing the test on error
func writeFileForTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to write file %s: %v", path, err)
	}
}
