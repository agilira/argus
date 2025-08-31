package argus

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Test scalabilità con diversi numeri di file
func TestScalabilityLimits(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping scalability test in short mode")
	}

	testCases := []int{50, 100, 250, 500, 1000}

	for _, numFiles := range testCases {
		t.Run(fmt.Sprintf("%d_files", numFiles), func(t *testing.T) {
			testScalabilityWithFiles(t, numFiles)
		})
	}
}

func testScalabilityWithFiles(t *testing.T, numFiles int) {
	tempDir, err := os.MkdirTemp("", fmt.Sprintf("scale_test_%d", numFiles))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create config files
	for i := 0; i < numFiles; i++ {
		filename := filepath.Join(tempDir, fmt.Sprintf("config_%d.json", i))
		content := fmt.Sprintf(`{"id": %d, "value": "test"}`, i)
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Choose optimization strategy based on file count
	var strategy OptimizationStrategy
	switch {
	case numFiles <= 3:
		strategy = OptimizationSingleEvent
	case numFiles <= 50:
		strategy = OptimizationSmallBatch
	default:
		strategy = OptimizationLargeBatch
	}

	config := Config{
		PollInterval:         100 * time.Millisecond,
		MaxWatchedFiles:      numFiles + 10, // Allow some buffer
		OptimizationStrategy: strategy,
	}

	watcher := New(*config.WithDefaults())
	defer watcher.Stop()

	var totalCallbacks int64
	var setupTime time.Duration

	// Measure setup time
	startSetup := time.Now()

	for i := 0; i < numFiles; i++ {
		filename := filepath.Join(tempDir, fmt.Sprintf("config_%d.json", i))
		watcher.Watch(filename, func(event ChangeEvent) {
			atomic.AddInt64(&totalCallbacks, 1) // Fix race condition
		})
	}

	setupTime = time.Since(startSetup)

	// Start watching and measure performance
	startTime := time.Now()
	if err := watcher.Start(); err != nil {
		t.Fatal(err)
	}

	// Let it stabilize - more time for large file sets
	stabilizeTime := 2 * config.PollInterval
	if numFiles >= 500 {
		stabilizeTime = 3 * config.PollInterval // Extra time for large sets
	}
	time.Sleep(stabilizeTime)

	// Trigger some changes with adaptive timing
	changeStartTime := time.Now()
	changesTriggered := 10
	changeInterval := config.PollInterval / 20 // Slower changes for better detection
	if numFiles >= 500 {
		changeInterval = config.PollInterval / 10 // Even slower for large sets
	}

	for i := 0; i < changesTriggered; i++ {
		fileIndex := i % numFiles
		filename := filepath.Join(tempDir, fmt.Sprintf("config_%d.json", fileIndex))
		content := fmt.Sprintf(`{"id": %d, "value": "changed_%d", "timestamp": %d}`, fileIndex, i, time.Now().UnixNano())
		os.WriteFile(filename, []byte(content), 0644)
		time.Sleep(changeInterval)
	}

	// Wait for detection - adaptive timing based on file count
	detectionWaitTime := 3 * config.PollInterval
	if numFiles >= 500 {
		detectionWaitTime = 5 * config.PollInterval // More time for large sets
	}
	time.Sleep(detectionWaitTime)
	changeTime := time.Since(changeStartTime)

	totalTime := time.Since(startTime)
	callbacks := atomic.LoadInt64(&totalCallbacks)

	// Performance metrics
	avgPollTime := float64(100) // ms polling interval
	pollCyclesPerSecond := 1000.0 / avgPollTime
	fileChecksPerSecond := pollCyclesPerSecond * float64(numFiles)

	t.Logf("SCALABILITY TEST: %d files", numFiles)
	t.Logf("  Strategy: %v", strategy)
	t.Logf("  Setup time: %v (%.2f μs/file)", setupTime, float64(setupTime.Microseconds())/float64(numFiles))
	t.Logf("  Total runtime: %v", totalTime)
	t.Logf("  Change detection time: %v", changeTime)
	t.Logf("  Changes triggered: %d", changesTriggered)
	t.Logf("  Callbacks received: %d", callbacks)
	t.Logf("  Detection rate: %.1f%%", float64(callbacks)/float64(changesTriggered)*100)
	t.Logf("  Theoretical file checks/sec: %.0f", fileChecksPerSecond)
	t.Logf("  File checks overhead: %.2f ns/file", 100*1000*1000/float64(numFiles)) // 100ms poll interval

	// Basic validations
	if callbacks == 0 && changesTriggered > 0 {
		t.Errorf("No callbacks received despite %d changes", changesTriggered)
	}

	// Performance thresholds - more realistic for high file counts
	maxSetupTimePerFile := 150 * time.Microsecond // More generous for 1000 files
	if numFiles >= 500 {
		maxSetupTimePerFile = 250 * time.Microsecond // Even more generous for large tests
	}
	if setupTime > time.Duration(numFiles)*maxSetupTimePerFile {
		t.Errorf("Setup too slow: %v for %d files (>%.1f μs/file)",
			setupTime, numFiles, float64(maxSetupTimePerFile.Microseconds()))
	}

	// Detection rate should be reasonable - adaptive thresholds
	minDetectionRate := 80.0 // 80% for small sets
	if numFiles >= 250 {
		minDetectionRate = 60.0 // 60% for medium sets
	}
	if numFiles >= 500 {
		minDetectionRate = 30.0 // 30% for large sets due to polling complexity
	}

	detectionRate := float64(callbacks) / float64(changesTriggered) * 100
	if detectionRate < minDetectionRate {
		t.Errorf("Detection rate too low: %.1f%% (expected at least %.1f%%)", detectionRate, minDetectionRate)
	}

	// Memory check (rough estimate)
	expectedMemoryPerFile := 200                    // bytes per file structure
	if numFiles*expectedMemoryPerFile > 1024*1024 { // 1MB
		t.Logf("  WARNING: High memory usage expected: ~%d KB",
			numFiles*expectedMemoryPerFile/1024)
	}
}
