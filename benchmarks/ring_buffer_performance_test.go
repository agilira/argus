// ring_buffer_performance_test.go: Isolated Ring Buffer Performance Benchmarks
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package benchmarks

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agilira/argus"
	"github.com/agilira/go-timecache"
)

// TestDummy - Dummy test so Go recognizes this as a test package
func TestDummy(t *testing.T) {
	// This test does nothing but allows Go to recognize benchmarks
}

// BenchmarkBoreasLite_SingleEvent - Ultra-low latency single event processing
// This is the fastest path optimized for 1-2 files scenarios
func BenchmarkBoreasLite_SingleEvent(b *testing.B) {
	var processed int64
	processor := func(*argus.FileChangeEvent) {
		processed++
	}

	boreas := argus.NewBoreasLite(256, argus.OptimizationSingleEvent, processor)
	defer boreas.Stop()

	event := argus.FileChangeEvent{
		ModTime: timecache.CachedTimeNano(),
		Size:    1024,
		Flags:   argus.FileEventModify,
		PathLen: 9,
	}
	copy(event.Path[:], []byte("test.json"))

	b.ResetTimer()
	b.ReportAllocs()

	// Simulate real scenario: write 1 event, process immediately
	for i := 0; i < b.N; i++ {
		if !boreas.WriteFileEvent(&event) {
			b.Fatal("Failed to write event")
		}
		boreas.ProcessBatch()
	}

	// Report operations per second
	ops := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(ops/1_000_000, "Mops/sec")
}

// BenchmarkBoreasLite_WriteFileEvent - Pure write performance
func BenchmarkBoreasLite_WriteFileEvent(b *testing.B) {
	processor := func(*argus.FileChangeEvent) {
		// Minimal processing for pure write benchmark
	}

	boreas := argus.NewBoreasLite(256, argus.OptimizationSmallBatch, processor)
	defer boreas.Stop()

	// Start processor in background
	go boreas.RunProcessor()

	event := argus.FileChangeEvent{
		ModTime: timecache.CachedTimeNano(),
		Size:    1024,
		Flags:   argus.FileEventModify,
		PathLen: 11,
	}
	copy(event.Path[:], []byte("config.json"))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		boreas.WriteFileEvent(&event)
	}

	ops := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(ops/1_000_000, "Mops/sec")
}

// BenchmarkBoreasLite_MPSC - Multiple Producer Single Consumer performance
func BenchmarkBoreasLite_MPSC(b *testing.B) {
	processor := func(*argus.FileChangeEvent) {
		// Minimal processing
	}

	boreas := argus.NewBoreasLite(1024, argus.OptimizationSmallBatch, processor)
	defer boreas.Stop()

	go boreas.RunProcessor()

	event := argus.FileChangeEvent{
		ModTime: time.Now().UnixNano(),
		Size:    1024,
		Flags:   argus.FileEventModify,
		PathLen: 11,
	}
	copy(event.Path[:], []byte("config.json"))

	// Test with multiple concurrent writers
	numWriters := runtime.GOMAXPROCS(0)

	b.ResetTimer()
	b.ReportAllocs()

	var wg sync.WaitGroup
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/numWriters; i++ {
				boreas.WriteFileEvent(&event)
			}
		}()
	}
	wg.Wait()

	ops := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(ops/1_000_000, "Mops/sec")
}

// BenchmarkBoreasLite_vsChannels - Direct comparison with Go channels
func BenchmarkBoreasLite_vsChannels(b *testing.B) {
	b.Run("BoreasLite", func(b *testing.B) {
		var processed int64
		processor := func(*argus.FileChangeEvent) {
			processed++
		}

		boreas := argus.NewBoreasLite(256, argus.OptimizationSmallBatch, processor)
		defer boreas.Stop()

		go boreas.RunProcessor()

		event := argus.FileChangeEvent{
			ModTime: time.Now().UnixNano(),
			Size:    1024,
			Flags:   argus.FileEventModify,
			PathLen: 11,
		}
		copy(event.Path[:], []byte("config.json"))

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			boreas.WriteFileEvent(&event)
		}

		ops := float64(b.N) / b.Elapsed().Seconds()
		b.ReportMetric(ops/1_000_000, "Mops/sec")
	})

	b.Run("GoChannels", func(b *testing.B) {
		var processed int64
		eventCh := make(chan argus.ChangeEvent, 256)

		// Start processor goroutine
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for event := range eventCh {
				_ = event // Simulate processing
				processed++
			}
		}()

		event := argus.ChangeEvent{
			Path:     "config.json",
			ModTime:  time.Now(),
			Size:     1024,
			IsModify: true,
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			eventCh <- event
		}

		close(eventCh)
		wg.Wait()

		ops := float64(b.N) / b.Elapsed().Seconds()
		b.ReportMetric(ops/1_000_000, "Mops/sec")
	})
}

// BenchmarkBoreasLite_HighThroughput - Maximum sustained throughput test
func BenchmarkBoreasLite_HighThroughput(b *testing.B) {
	var processed atomic.Int64
	processor := func(*argus.FileChangeEvent) {
		processed.Add(1) // Use atomic to fix lint error
	}

	// Large buffer for maximum throughput, but don't wait for processing
	boreas := argus.NewBoreasLite(8192, argus.OptimizationLargeBatch, processor)
	defer boreas.Stop()

	go boreas.RunProcessor()

	event := argus.FileChangeEvent{
		ModTime: time.Now().UnixNano(),
		Size:    1024,
		Flags:   argus.FileEventModify,
		PathLen: 4,
	}
	copy(event.Path[:], []byte("test"))

	// Let processor warm up
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	// Pure write throughput test - don't wait for processing
	for i := 0; i < b.N; i++ {
		boreas.WriteFileEvent(&event)
	}

	ops := float64(b.N) / b.Elapsed().Seconds()
	b.ReportMetric(ops/1_000_000, "Mops/sec")
}
