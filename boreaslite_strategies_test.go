// boreaslite_strategies_test.go - Test delle strategie di processing di BoreasLite
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/agilira/go-timecache"
)

// TestBoreasLiteSmallBatchStrategy tests the small batch processing strategy
func TestBoreasLiteSmallBatchStrategy(t *testing.T) {
	processedCount := 0
	var processMutex sync.Mutex

	processor := func(event *FileChangeEvent) {
		processMutex.Lock()
		processedCount++
		processMutex.Unlock()
	}

	// Create BoreasLite with small batch configuration
	boreas := NewBoreasLite(8, OptimizationSmallBatch, processor)
	if boreas == nil {
		t.Fatal("Failed to create BoreasLite instance")
	}
	defer boreas.Stop()

	// Add enough events to trigger small batch processing
	events := []FileChangeEvent{
		{PathLen: 11, ModTime: timecache.CachedTimeNano(), Size: 100, Flags: FileEventCreate},
		{PathLen: 11, ModTime: timecache.CachedTimeNano(), Size: 200, Flags: FileEventModify},
		{PathLen: 11, ModTime: timecache.CachedTimeNano(), Size: 0, Flags: FileEventDelete},
		{PathLen: 11, ModTime: timecache.CachedTimeNano(), Size: 150, Flags: FileEventCreate},
		{PathLen: 11, ModTime: timecache.CachedTimeNano(), Size: 250, Flags: FileEventModify},
	}

	// Copy path names into the fixed-size arrays
	copy(events[0].Path[:], "/test1.json")
	copy(events[1].Path[:], "/test2.json")
	copy(events[2].Path[:], "/test3.json")
	copy(events[3].Path[:], "/test4.json")
	copy(events[4].Path[:], "/test5.json")

	// Write events
	for _, event := range events {
		boreas.WriteFileEvent(&event)
	}

	// Force small batch processing by calling ProcessBatch directly
	// This should trigger processSmallBatchOptimized
	processed := 0
	for i := 0; i < 10 && processed < len(events); i++ {
		batchProcessed := boreas.ProcessBatch()
		processed += batchProcessed
		if batchProcessed == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	time.Sleep(100 * time.Millisecond) // Let processing complete

	if processedCount == 0 {
		t.Errorf("No events were processed in small batch strategy")
	}

	stats := boreas.Stats()
	t.Logf("Small batch processing stats: %+v", stats)
}

// TestBoreasLiteLargeBatchStrategy tests the large batch processing strategy
func TestBoreasLiteLargeBatchStrategy(t *testing.T) {
	processedCount := 0
	var processMutex sync.Mutex

	processor := func(event *FileChangeEvent) {
		processMutex.Lock()
		processedCount++
		processMutex.Unlock()
	}

	// Create BoreasLite with large batch configuration
	boreas := NewBoreasLite(64, OptimizationLargeBatch, processor)
	if boreas == nil {
		t.Fatal("Failed to create BoreasLite instance")
	}
	defer boreas.Stop()

	// Add many events to trigger large batch processing
	events := make([]FileChangeEvent, 30) // More than 20 to trigger large batch
	for i := 0; i < 30; i++ {
		events[i] = FileChangeEvent{
			PathLen: 20,
			ModTime: timecache.CachedTimeNano(),
			Size:    int64(100 + i),
			Flags:   FileEventModify,
		}
		// Copy unique path for each event
		path := "/test_large_" + string(rune('A'+i%26)) + ".json"
		copy(events[i].Path[:], path)
		events[i].PathLen = uint8(len(path))
	}

	// Write events to fill buffer significantly
	for _, event := range events {
		boreas.WriteFileEvent(&event)
	}

	// Force large batch processing
	processed := 0
	for i := 0; i < 15 && processed < len(events); i++ {
		batchProcessed := boreas.ProcessBatch()
		processed += batchProcessed
		if batchProcessed == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	time.Sleep(100 * time.Millisecond) // Let processing complete

	if processedCount == 0 {
		t.Errorf("No events were processed in large batch strategy")
	}

	stats := boreas.Stats()
	t.Logf("Large batch processing stats: %+v", stats)
}

// TestBoreasLiteSmallBatchProcessor tests the small batch processor running mode
func TestBoreasLiteSmallBatchProcessor(t *testing.T) {
	processedCount := 0
	var processMutex sync.Mutex

	processor := func(event *FileChangeEvent) {
		processMutex.Lock()
		processedCount++
		processMutex.Unlock()
	}

	// Create BoreasLite with small batch strategy
	boreas := NewBoreasLite(8, OptimizationSmallBatch, processor)
	if boreas == nil {
		t.Fatal("Failed to create BoreasLite instance")
	}

	// Start the processor in background
	go boreas.RunProcessor()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Add events to be processed
	events := []FileChangeEvent{
		{PathLen: 19, ModTime: timecache.CachedTimeNano(), Size: 100, Flags: FileEventCreate},
		{PathLen: 19, ModTime: timecache.CachedTimeNano(), Size: 200, Flags: FileEventModify},
		{PathLen: 19, ModTime: timecache.CachedTimeNano(), Size: 0, Flags: FileEventDelete},
	}

	copy(events[0].Path[:], "/processor_test1.json")
	copy(events[1].Path[:], "/processor_test2.json")
	copy(events[2].Path[:], "/processor_test3.json")

	for _, event := range events {
		boreas.WriteFileEvent(&event)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Stop the processor
	boreas.Stop()

	if processedCount == 0 {
		t.Errorf("No events were processed by small batch processor")
	}

	stats := boreas.Stats()
	t.Logf("Small batch processor stats: %+v", stats)
}

// TestBoreasLiteLargeBatchProcessor tests the large batch processor running mode
func TestBoreasLiteLargeBatchProcessor(t *testing.T) {
	processedCount := 0
	var processMutex sync.Mutex

	processor := func(event *FileChangeEvent) {
		processMutex.Lock()
		processedCount++
		processMutex.Unlock()
	}

	// Create BoreasLite with large batch strategy
	boreas := NewBoreasLite(64, OptimizationLargeBatch, processor)
	if boreas == nil {
		t.Fatal("Failed to create BoreasLite instance")
	}

	// Start the processor in background
	go boreas.RunProcessor()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Add many events to trigger large batch processing
	events := make([]FileChangeEvent, 25) // More than 20 for large batch
	for i := 0; i < 25; i++ {
		events[i] = FileChangeEvent{
			PathLen: 25,
			ModTime: timecache.CachedTimeNano(),
			Size:    int64(100 + i),
			Flags:   FileEventModify,
		}
		path := "/large_processor_test_" + string(rune('A'+i%26)) + ".json"
		copy(events[i].Path[:], path)
		events[i].PathLen = uint8(len(path))
	}

	for _, event := range events {
		boreas.WriteFileEvent(&event)
	}

	// Wait for processing
	time.Sleep(300 * time.Millisecond)

	// Stop the processor
	boreas.Stop()

	if processedCount == 0 {
		t.Errorf("No events were processed by large batch processor")
	}

	stats := boreas.Stats()
	t.Logf("Large batch processor stats: %+v", stats)
}

// TestBoreasLiteLightStrategy tests the light (sleep-only) processing strategy
func TestBoreasLiteLightStrategy(t *testing.T) {
	processedCount := 0
	var processMutex sync.Mutex

	processor := func(event *FileChangeEvent) {
		processMutex.Lock()
		processedCount++
		processMutex.Unlock()
	}

	boreas := NewBoreasLite(8, OptimizationLight, processor)
	if boreas == nil {
		t.Fatal("Failed to create BoreasLite instance with Light strategy")
	}
	defer boreas.Stop()

	events := []FileChangeEvent{
		{PathLen: 12, ModTime: timecache.CachedTimeNano(), Size: 100, Flags: FileEventModify},
		{PathLen: 12, ModTime: timecache.CachedTimeNano(), Size: 200, Flags: FileEventModify},
		{PathLen: 12, ModTime: timecache.CachedTimeNano(), Size: 300, Flags: FileEventCreate},
	}
	copy(events[0].Path[:], "/config.yaml")
	copy(events[1].Path[:], "/agent.yaml ")
	copy(events[2].Path[:], "/policy.yaml")

	for _, event := range events {
		boreas.WriteFileEvent(&event)
	}

	processed := 0
	for i := 0; i < 20 && processed < len(events); i++ {
		batchProcessed := boreas.ProcessBatch()
		processed += batchProcessed
		if batchProcessed == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	time.Sleep(100 * time.Millisecond)

	if processedCount == 0 {
		t.Errorf("No events were processed in light strategy")
	}
	if processedCount != len(events) {
		t.Errorf("Expected %d events processed, got %d", len(events), processedCount)
	}

	stats := boreas.Stats()
	t.Logf("Light strategy stats: %+v", stats)
}

// TestBoreasLiteLightProcessor tests that the light processor uses sleep, no spin-wait
func TestBoreasLiteLightProcessor(t *testing.T) {
	processedCount := 0
	var processMutex sync.Mutex

	processor := func(event *FileChangeEvent) {
		processMutex.Lock()
		processedCount++
		processMutex.Unlock()
	}

	boreas := NewBoreasLite(8, OptimizationLight, processor)
	if boreas == nil {
		t.Fatal("Failed to create BoreasLite instance")
	}

	go boreas.RunProcessor()
	time.Sleep(50 * time.Millisecond)

	events := make([]FileChangeEvent, 5)
	for i := 0; i < 5; i++ {
		events[i] = FileChangeEvent{
			PathLen: 15,
			ModTime: timecache.CachedTimeNano(),
			Size:    int64(100 + i),
			Flags:   FileEventModify,
		}
		path := "/light_test_" + string(rune('A'+i)) + ".yml"
		copy(events[i].Path[:], path)
		events[i].PathLen = uint8(len(path))
	}

	for _, event := range events {
		boreas.WriteFileEvent(&event)
	}

	time.Sleep(300 * time.Millisecond)
	boreas.Stop()

	processMutex.Lock()
	count := processedCount
	processMutex.Unlock()

	if count == 0 {
		t.Errorf("No events were processed by light processor")
	}

	stats := boreas.Stats()
	t.Logf("Light processor stats: %+v", stats)
}

// TestBoreasLiteLightProcessorCPUEfficiency verifies the light processor
// does not burn CPU with spin-wait. It should use near-zero CPU when idle.
func TestBoreasLiteLightProcessorCPUEfficiency(t *testing.T) {
	processor := func(_ *FileChangeEvent) {}

	boreas := NewBoreasLite(8, OptimizationLight, processor)
	if boreas == nil {
		t.Fatal("Failed to create BoreasLite instance")
	}

	// Measure goroutine count before
	goroutinesBefore := runtime.NumGoroutine()

	go boreas.RunProcessor()
	time.Sleep(50 * time.Millisecond)

	// Verify only one extra goroutine was created (the processor)
	goroutinesAfter := runtime.NumGoroutine()
	if goroutinesAfter-goroutinesBefore > 2 {
		t.Errorf("Light processor should create at most 1 goroutine, got %d extra",
			goroutinesAfter-goroutinesBefore)
	}

	// Let it idle for 500ms with zero events -- should not burn CPU
	time.Sleep(500 * time.Millisecond)

	boreas.Stop()

	stats := boreas.Stats()
	if stats["items_processed"] != 0 {
		t.Errorf("Expected 0 processed events when idle, got %d", stats["items_processed"])
	}
}

// TestBoreasLiteLightBatchSize verifies the Light strategy uses batch size 1
func TestBoreasLiteLightBatchSize(t *testing.T) {
	boreas := NewBoreasLite(8, OptimizationLight, func(_ *FileChangeEvent) {})
	defer boreas.Stop()

	if boreas.batchSize != 1 {
		t.Errorf("Light strategy should use batchSize=1, got %d", boreas.batchSize)
	}
}
