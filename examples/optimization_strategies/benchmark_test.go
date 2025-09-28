package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/agilira/argus"
)

// BenchmarkSingleEventStrategy benchmarks the SingleEvent optimization strategy
// This measures the ultra-low latency performance for single file monitoring
func BenchmarkSingleEventStrategy(b *testing.B) {
	config := argus.Config{
		PollInterval:         10 * time.Millisecond,
		OptimizationStrategy: argus.OptimizationSingleEvent,
		BoreasLiteCapacity:   64,
	}

	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	// Setup benchmark environment
	tempDir, err := os.MkdirTemp("", "bench_single_event")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "bench.json")
	if err := os.WriteFile(configFile, []byte(`{"benchmark": true}`), 0600); err != nil {
		b.Fatal(err)
	}

	var eventCount int64
	var mu sync.Mutex

	err = watcher.Watch(configFile, func(event argus.ChangeEvent) {
		mu.Lock()
		eventCount++
		mu.Unlock()
	})
	if err != nil {
		b.Fatal(err)
	}

	if err := watcher.Start(); err != nil {
		b.Fatal(err)
	}

	// Allow initial setup
	time.Sleep(20 * time.Millisecond)

	// Reset event counter after setup to avoid counting setup events
	mu.Lock()
	eventCount = 0
	mu.Unlock()

	b.ResetTimer()
	b.ReportAllocs()

	// Benchmark file modification detection speed
	for i := 0; i < b.N; i++ {
		content := fmt.Sprintf(`{"benchmark": true, "iteration": %d}`, i)
		if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
			b.Fatal(err)
		}

		// Small delay to allow processing (realistic scenario)
		time.Sleep(15 * time.Millisecond)
	}

	b.StopTimer()

	// Report final event count
	mu.Lock()
	finalEventCount := eventCount
	mu.Unlock()

	expectedEvents := float64(b.N)
	detectionRate := float64(finalEventCount) / expectedEvents * 100
	if detectionRate > 100 {
		detectionRate = 100
	}

	b.ReportMetric(float64(finalEventCount), "events_detected")
	b.ReportMetric(detectionRate, "detection_rate_%")
}

// BenchmarkSmallBatchStrategy benchmarks the SmallBatch optimization strategy
// This measures balanced performance for moderate file counts (3-20 files)
func BenchmarkSmallBatchStrategy(b *testing.B) {
	config := argus.Config{
		PollInterval:         25 * time.Millisecond,
		OptimizationStrategy: argus.OptimizationSmallBatch,
		BoreasLiteCapacity:   128,
	}

	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	// Setup benchmark environment with multiple files
	tempDir, err := os.MkdirTemp("", "bench_small_batch")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create 8 files (optimal for SmallBatch)
	fileCount := 8
	files := make([]string, fileCount)

	var totalEvents int64
	var mu sync.Mutex

	for i := 0; i < fileCount; i++ {
		filename := fmt.Sprintf("service-%d.json", i)
		filePath := filepath.Join(tempDir, filename)
		files[i] = filePath

		initialContent := fmt.Sprintf(`{"service": %d, "benchmark": true}`, i)
		if err := os.WriteFile(filePath, []byte(initialContent), 0600); err != nil {
			b.Fatal(err)
		}

		err := watcher.Watch(filePath, func(event argus.ChangeEvent) {
			mu.Lock()
			totalEvents++
			mu.Unlock()
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	if err := watcher.Start(); err != nil {
		b.Fatal(err)
	}

	time.Sleep(30 * time.Millisecond)

	// Reset event counter after setup to avoid counting setup events
	mu.Lock()
	totalEvents = 0
	mu.Unlock()

	b.ResetTimer()
	b.ReportAllocs()

	// Benchmark batch processing of multiple files
	for i := 0; i < b.N; i++ {
		// Update all files in the batch
		for j, filePath := range files {
			content := fmt.Sprintf(`{"service": %d, "benchmark": true, "iteration": %d}`, j, i)
			if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
				b.Fatal(err)
			}
		}

		// Allow batch processing time
		time.Sleep(30 * time.Millisecond)
	}

	b.StopTimer()

	mu.Lock()
	finalEventCount := totalEvents
	mu.Unlock()

	expectedEvents := float64(b.N * fileCount)
	detectionRate := float64(finalEventCount) / expectedEvents * 100
	if detectionRate > 100 {
		detectionRate = 100
	}

	b.ReportMetric(float64(finalEventCount), "total_events")
	b.ReportMetric(detectionRate, "detection_rate_%")
	b.ReportMetric(float64(finalEventCount)/float64(b.N), "events_per_batch")
}

// BenchmarkLargeBatchStrategy benchmarks the LargeBatch optimization strategy
// This measures high-throughput performance for many files (20+ files)
func BenchmarkLargeBatchStrategy(b *testing.B) {
	config := argus.Config{
		PollInterval:         50 * time.Millisecond,
		OptimizationStrategy: argus.OptimizationLargeBatch,
		BoreasLiteCapacity:   256,
	}

	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	// Setup benchmark environment with many files
	tempDir, err := os.MkdirTemp("", "bench_large_batch")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create 30 files (optimal for LargeBatch)
	fileCount := 30
	files := make([]string, fileCount)

	var totalEvents int64
	var mu sync.Mutex

	for i := 0; i < fileCount; i++ {
		filename := fmt.Sprintf("container-%02d.json", i)
		filePath := filepath.Join(tempDir, filename)
		files[i] = filePath

		initialContent := fmt.Sprintf(`{"container": %d, "status": "active"}`, i)
		if err := os.WriteFile(filePath, []byte(initialContent), 0600); err != nil {
			b.Fatal(err)
		}

		err := watcher.Watch(filePath, func(event argus.ChangeEvent) {
			mu.Lock()
			totalEvents++
			mu.Unlock()
		})
		if err != nil {
			b.Fatal(err)
		}
	}

	if err := watcher.Start(); err != nil {
		b.Fatal(err)
	}

	time.Sleep(60 * time.Millisecond)

	// Reset event counter after setup to avoid counting setup events
	mu.Lock()
	totalEvents = 0
	mu.Unlock()

	b.ResetTimer()
	b.ReportAllocs()

	// Benchmark high-throughput bulk processing
	for i := 0; i < b.N; i++ {
		// Update all files in bulk
		for j, filePath := range files {
			content := fmt.Sprintf(`{"container": %d, "status": "updated", "batch": %d}`, j, i)
			if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
				b.Fatal(err)
			}
		}

		// Allow bulk processing time
		time.Sleep(80 * time.Millisecond)
	}

	b.StopTimer()

	mu.Lock()
	finalEventCount := totalEvents
	mu.Unlock()

	expectedEvents := float64(b.N * fileCount)
	detectionRate := float64(finalEventCount) / expectedEvents * 100
	if detectionRate > 100 {
		detectionRate = 100
	}

	b.ReportMetric(float64(finalEventCount), "total_events")
	b.ReportMetric(detectionRate, "detection_rate_%")
	b.ReportMetric(float64(finalEventCount)/float64(b.N), "events_per_batch")

	// Calculate throughput
	elapsed := time.Duration(b.N) * 80 * time.Millisecond
	throughput := float64(finalEventCount) / elapsed.Seconds()
	b.ReportMetric(throughput, "events_per_second")
}

// BenchmarkAutoStrategy benchmarks the Auto optimization strategy
// This measures adaptive performance across different file count scenarios
func BenchmarkAutoStrategy(b *testing.B) {
	config := argus.Config{
		PollInterval:         20 * time.Millisecond,
		OptimizationStrategy: argus.OptimizationAuto,
		// BoreasLiteCapacity: 0 for auto-sizing
	}

	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	tempDir, err := os.MkdirTemp("", "bench_auto_strategy")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	if err := watcher.Start(); err != nil {
		b.Fatal(err)
	}

	var totalEvents int64
	var mu sync.Mutex

	// Test auto-adaptation with varying file counts
	b.Run("SingleFile", func(b *testing.B) {
		// Should adapt to SingleEvent behavior
		singleFile := filepath.Join(tempDir, "auto-single.json")
		if err := os.WriteFile(singleFile, []byte(`{"auto": "single"}`), 0600); err != nil {
			b.Fatal(err)
		}

		err := watcher.Watch(singleFile, func(event argus.ChangeEvent) {
			mu.Lock()
			totalEvents++
			mu.Unlock()
		})
		if err != nil {
			b.Fatal(err)
		}

		time.Sleep(30 * time.Millisecond) // Allow adaptation

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			content := fmt.Sprintf(`{"auto": "single", "iteration": %d}`, i)
			if err := os.WriteFile(singleFile, []byte(content), 0600); err != nil {
				b.Fatal(err)
			}
			time.Sleep(15 * time.Millisecond)
		}
	})

	b.Run("MultipleFiles", func(b *testing.B) {
		// Should adapt to SmallBatch behavior
		fileCount := 6
		files := make([]string, fileCount)

		for i := 0; i < fileCount; i++ {
			filename := fmt.Sprintf("auto-multi-%d.json", i)
			filePath := filepath.Join(tempDir, filename)
			files[i] = filePath

			initialContent := fmt.Sprintf(`{"auto": "multi", "id": %d}`, i)
			if err := os.WriteFile(filePath, []byte(initialContent), 0600); err != nil {
				b.Fatal(err)
			}

			err := watcher.Watch(filePath, func(event argus.ChangeEvent) {
				mu.Lock()
				totalEvents++
				mu.Unlock()
			})
			if err != nil {
				b.Fatal(err)
			}
		}

		time.Sleep(50 * time.Millisecond) // Allow adaptation

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Update multiple files
			for j, filePath := range files {
				content := fmt.Sprintf(`{"auto": "multi", "id": %d, "iteration": %d}`, j, i)
				if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
					b.Fatal(err)
				}
			}
			time.Sleep(35 * time.Millisecond)
		}
	})

	mu.Lock()
	finalEventCount := totalEvents
	mu.Unlock()

	b.ReportMetric(float64(finalEventCount), "total_adaptive_events")
}

// BenchmarkStrategyComparison compares all strategies under identical conditions
func BenchmarkStrategyComparison(b *testing.B) {
	strategies := []struct {
		name     string
		strategy argus.OptimizationStrategy
		capacity int64
		interval time.Duration
	}{
		{"SingleEvent", argus.OptimizationSingleEvent, 64, 10 * time.Millisecond},
		{"SmallBatch", argus.OptimizationSmallBatch, 128, 25 * time.Millisecond},
		{"LargeBatch", argus.OptimizationLargeBatch, 256, 50 * time.Millisecond},
		{"Auto", argus.OptimizationAuto, 0, 20 * time.Millisecond},
	}

	// Common test parameters
	fileCount := 10 // Moderate workload for fair comparison

	for _, strategy := range strategies {
		b.Run(strategy.name, func(b *testing.B) {
			config := argus.Config{
				PollInterval:         strategy.interval,
				OptimizationStrategy: strategy.strategy,
				BoreasLiteCapacity:   strategy.capacity,
			}

			watcher := argus.New(*config.WithDefaults())
			defer watcher.Stop()

			tempDir, err := os.MkdirTemp("", fmt.Sprintf("bench_comp_%s", strategy.name))
			if err != nil {
				b.Fatal(err)
			}
			defer os.RemoveAll(tempDir)

			var eventCount int64
			var mu sync.Mutex

			// Setup identical workload for all strategies
			files := make([]string, fileCount)
			for i := 0; i < fileCount; i++ {
				filename := fmt.Sprintf("comp-test-%d.json", i)
				filePath := filepath.Join(tempDir, filename)
				files[i] = filePath

				initialContent := fmt.Sprintf(`{"strategy": "%s", "file": %d}`, strategy.name, i)
				if err := os.WriteFile(filePath, []byte(initialContent), 0600); err != nil {
					b.Fatal(err)
				}

				err := watcher.Watch(filePath, func(event argus.ChangeEvent) {
					mu.Lock()
					eventCount++
					mu.Unlock()
				})
				if err != nil {
					b.Fatal(err)
				}
			}

			if err := watcher.Start(); err != nil {
				b.Fatal(err)
			}

			// Allow strategy-specific setup time
			setupTime := strategy.interval + 20*time.Millisecond
			time.Sleep(setupTime)

			// Reset event counter after setup to avoid counting setup events
			mu.Lock()
			eventCount = 0
			mu.Unlock()

			b.ResetTimer()
			b.ReportAllocs()

			// Identical benchmark workload for all strategies
			for i := 0; i < b.N; i++ {
				// Update all files
				for j, filePath := range files {
					content := fmt.Sprintf(`{"strategy": "%s", "file": %d, "iteration": %d}`,
						strategy.name, j, i)
					if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
						b.Fatal(err)
					}
				}

				// Wait appropriate time for this strategy
				time.Sleep(strategy.interval + 10*time.Millisecond)
			}

			b.StopTimer()

			mu.Lock()
			finalCount := eventCount
			mu.Unlock()

			// Report strategy-specific metrics
			b.ReportMetric(float64(finalCount), "events_detected")

			expectedEvents := float64(b.N * fileCount)
			// Cap detection rate at 100% as values >100% indicate timing issues or duplicate events
			detectionRate := float64(finalCount) / expectedEvents * 100
			if detectionRate > 100 {
				detectionRate = 100
			}
			b.ReportMetric(detectionRate, "detection_rate_%")
			b.ReportMetric(float64(finalCount)/expectedEvents, "events_per_expected")
		})
	}
}

// BenchmarkMemoryUsage benchmarks memory allocation patterns for each strategy
func BenchmarkMemoryUsage(b *testing.B) {
	strategies := []struct {
		name     string
		strategy argus.OptimizationStrategy
		capacity int64
	}{
		{"SingleEvent", argus.OptimizationSingleEvent, 64},
		{"SmallBatch", argus.OptimizationSmallBatch, 128},
		{"LargeBatch", argus.OptimizationLargeBatch, 256},
		{"Auto", argus.OptimizationAuto, 0},
	}

	for _, strategy := range strategies {
		b.Run(strategy.name+"_Memory", func(b *testing.B) {
			tempDir, err := os.MkdirTemp("", fmt.Sprintf("bench_mem_%s", strategy.name))
			if err != nil {
				b.Fatal(err)
			}
			defer os.RemoveAll(tempDir)

			b.ResetTimer()
			b.ReportAllocs()

			// Measure memory allocations during watcher creation and operation
			for i := 0; i < b.N; i++ {
				config := argus.Config{
					PollInterval:         20 * time.Millisecond,
					OptimizationStrategy: strategy.strategy,
					BoreasLiteCapacity:   strategy.capacity,
				}

				watcher := argus.New(*config.WithDefaults())

				// Create a test file
				testFile := filepath.Join(tempDir, fmt.Sprintf("mem-test-%d.json", i))
				if err := os.WriteFile(testFile, []byte(`{"memory": "test"}`), 0600); err != nil {
					b.Fatal(err)
				}

				// Setup watcher (this is what we're measuring)
				err := watcher.Watch(testFile, func(event argus.ChangeEvent) {
					// Minimal callback to avoid skewing results
					_ = event
				})
				if err != nil {
					b.Fatal(err)
				}

				watcher.Stop()
			}
		})
	}
}

// BenchmarkLatencyDistribution measures latency distribution for each strategy
func BenchmarkLatencyDistribution(b *testing.B) {
	config := argus.Config{
		PollInterval:         15 * time.Millisecond,
		OptimizationStrategy: argus.OptimizationSingleEvent, // Fastest for latency testing
		BoreasLiteCapacity:   64,
	}

	watcher := argus.New(*config.WithDefaults())
	defer watcher.Stop()

	tempDir, err := os.MkdirTemp("", "bench_latency")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "latency-test.json")
	if err := os.WriteFile(testFile, []byte(`{"latency": "test"}`), 0600); err != nil {
		b.Fatal(err)
	}

	var latencies []time.Duration
	var mu sync.Mutex
	detected := make(chan time.Time, b.N)

	err = watcher.Watch(testFile, func(event argus.ChangeEvent) {
		detected <- time.Now()
	})
	if err != nil {
		b.Fatal(err)
	}

	if err := watcher.Start(); err != nil {
		b.Fatal(err)
	}

	time.Sleep(20 * time.Millisecond)

	b.ResetTimer()

	// Measure individual event latencies
	for i := 0; i < b.N; i++ {
		startTime := time.Now()

		content := fmt.Sprintf(`{"latency": "test", "iteration": %d}`, i)
		if err := os.WriteFile(testFile, []byte(content), 0600); err != nil {
			b.Fatal(err)
		}

		// Wait for detection with timeout
		select {
		case detectionTime := <-detected:
			latency := detectionTime.Sub(startTime)
			mu.Lock()
			latencies = append(latencies, latency)
			mu.Unlock()
		case <-time.After(100 * time.Millisecond):
			// Timeout, skip this measurement
		}

		// Small delay between iterations
		time.Sleep(20 * time.Millisecond)
	}

	b.StopTimer()

	// Calculate latency statistics
	if len(latencies) > 0 {
		var total time.Duration
		var min, max time.Duration = latencies[0], latencies[0]

		for _, lat := range latencies {
			total += lat
			if lat < min {
				min = lat
			}
			if lat > max {
				max = lat
			}
		}

		avg := total / time.Duration(len(latencies))

		b.ReportMetric(float64(len(latencies)), "successful_detections")
		b.ReportMetric(float64(avg.Nanoseconds())/1e6, "avg_latency_ms")
		b.ReportMetric(float64(min.Nanoseconds())/1e6, "min_latency_ms")
		b.ReportMetric(float64(max.Nanoseconds())/1e6, "max_latency_ms")
	}
}
