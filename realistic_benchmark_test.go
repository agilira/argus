// realistic_benchmark_test.go: Testing Argus Realistic Benchmarking
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

	"github.com/agilira/go-timecache"
)

// Benchmark: confronts complete realistic scenarios
func BenchmarkRealWorldArchitectures(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "realistic_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := filepath.Join(tempDir, "config.json")
	initialContent := `{"test": "value", "counter": 0}`
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		b.Fatal(err)
	}

	b.Run("Traditional_Polling", func(b *testing.B) {
		benchmarkTraditionalPolling(b, testFile)
	})

	b.Run("BoreasLite_SingleEvent", func(b *testing.B) {
		benchmarkBoreasLiteSingle(b, testFile)
	})

	b.Run("BoreasLite_SmallBatch", func(b *testing.B) {
		benchmarkBoreasLiteSmall(b, testFile)
	})

	b.Run("DirectCallback_Theoretical", func(b *testing.B) {
		benchmarkDirectCallbackTheoretical(b)
	})
}

// Traditional polling approach
func benchmarkTraditionalPolling(b *testing.B, testFile string) {
	var eventCount atomic.Int64
	callback := func(_ string) {
		eventCount.Add(1)
	}

	// Simulate traditional polling by checking file mod time
	lastStat, _ := os.Stat(testFile)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate the complete cycle of traditional polling
		stat, err := os.Stat(testFile)
		if err == nil && !stat.ModTime().Equal(lastStat.ModTime()) {
			callback(testFile)
			lastStat = stat
		}
	}
}

// BoreasLite with single event strategy (realistic scenario)
func benchmarkBoreasLiteSingle(b *testing.B, testFile string) {
	var eventCount atomic.Int64
	processor := func(event *FileChangeEvent) {
		eventCount.Add(1)
	}

	boreas := NewBoreasLite(64, OptimizationSingleEvent, processor)
	defer boreas.Stop()

	// Realistic event
	event := FileChangeEvent{
		ModTime: timecache.CachedTimeNano(),
		Size:    1024,
		Flags:   FileEventModify,
		PathLen: uint8(len(testFile)),
	}
	copy(event.Path[:], []byte(testFile))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate the complete cycle: write + process
		boreas.WriteFileEvent(&event)
		boreas.ProcessBatch()
	}
}

// BoreasLite with small batch strategy (realistic scenario)
func benchmarkBoreasLiteSmall(b *testing.B, testFile string) {
	var eventCount atomic.Int64
	processor := func(event *FileChangeEvent) {
		eventCount.Add(1)
	}

	boreas := NewBoreasLite(128, OptimizationSmallBatch, processor)
	defer boreas.Stop()

	// Realistic event
	event := FileChangeEvent{
		ModTime: timecache.CachedTimeNano(),
		Size:    1024,
		Flags:   FileEventModify,
		PathLen: uint8(len(testFile)),
	}
	copy(event.Path[:], []byte(testFile))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate the complete cycle: write + process
		boreas.WriteFileEvent(&event)
		boreas.ProcessBatch()
	}
}

// DirectCallback - Benchmark with realistic context for theoretical comparison
func benchmarkDirectCallbackTheoretical(b *testing.B) {
	var eventCount atomic.Int64
	callback := func(event ChangeEvent) {
		// Simulate realistic processing of the callback
		eventCount.Add(1)
		_ = event.Path
		_ = event.ModTime
		_ = event.Size
	}

	event := ChangeEvent{
		Path:     "config.json",
		ModTime:  timecache.CachedTime(),
		Size:     1024,
		IsModify: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// NOTE: This benchmark ONLY simulates the callback invocation
		// DOES NOT include file system polling, detection, conversion
		callback(event)
	}
}

// Benchmark end-to-end realistic with real file system
func BenchmarkEndToEnd_RealFileSystem(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "e2e_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "config.json")
	initialContent := `{"benchmark": true}`
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		b.Fatal(err)
	}

	b.Run("Traditional_FileWatcher", func(b *testing.B) {
		benchmarkTraditionalFileWatcher(b, testFile)
	})

	b.Run("Argus_WithBoreasLite", func(b *testing.B) {
		benchmarkArgusComplete(b, testFile)
	})
}

func benchmarkTraditionalFileWatcher(b *testing.B, testFile string) {
	var eventCount atomic.Int64
	lastStat, _ := os.Stat(testFile)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Modify file
		content := []byte(`{"benchmark": true, "iteration": ` + string(rune(i)) + `}`)
		os.WriteFile(testFile, content, 0644)

		// Traditional polling
		stat, err := os.Stat(testFile)
		if err == nil && !stat.ModTime().Equal(lastStat.ModTime()) {
			eventCount.Add(1)
			lastStat = stat
		}
	}
}

func benchmarkArgusComplete(b *testing.B, testFile string) {
	config := Config{
		PollInterval:         1 * time.Millisecond,
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer watcher.Stop()

	var eventCount atomic.Int64
	callback := func(event ChangeEvent) {
		eventCount.Add(1)
	}

	watcher.Watch(testFile, callback)
	watcher.Start()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Modify real file
		content := []byte(`{"benchmark": true, "iteration": ` + string(rune(i)) + `}`)
		os.WriteFile(testFile, content, 0644)

		// Small pause to allow detection
		time.Sleep(2 * time.Millisecond)
	}

	// Wait for all events to be processed
	time.Sleep(10 * time.Millisecond)
}
