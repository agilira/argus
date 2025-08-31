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
		err := watcher.Watch(filename, func(event ChangeEvent) {
			atomic.AddInt64(&totalCallbacks, 1)
		})
		if err != nil {
			t.Fatalf("Failed to watch file %d: %v", i, err)
		}
	}

	setupTime = time.Since(startSetup)

	// Start watching and measure performance
	startTime := time.Now()
	if err := watcher.Start(); err != nil {
		t.Fatal(err)
	}

	// Let it run for a bit to measure steady-state
	time.Sleep(500 * time.Millisecond)

	// Trigger some changes
	changeStartTime := time.Now()
	changesTriggered := 10
	for i := 0; i < changesTriggered; i++ {
		fileIndex := i % numFiles
		filename := filepath.Join(tempDir, fmt.Sprintf("config_%d.json", fileIndex))
		content := fmt.Sprintf(`{"id": %d, "value": "changed_%d"}`, fileIndex, i)
		os.WriteFile(filename, []byte(content), 0644)
		time.Sleep(50 * time.Millisecond) // Space out changes
	}

	// Wait for callbacks
	time.Sleep(time.Duration(numFiles)*time.Millisecond + 500*time.Millisecond)
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

	// Performance thresholds
	maxSetupTimePerFile := 100 * time.Microsecond // 100μs per file is generous
	if setupTime > time.Duration(numFiles)*maxSetupTimePerFile {
		t.Errorf("Setup too slow: %v for %d files (>%.1f μs/file)",
			setupTime, numFiles, float64(maxSetupTimePerFile.Microseconds()))
	}

	// Memory check (rough estimate)
	expectedMemoryPerFile := 200                    // bytes per file structure
	if numFiles*expectedMemoryPerFile > 1024*1024 { // 1MB
		t.Logf("  WARNING: High memory usage expected: ~%d KB",
			numFiles*expectedMemoryPerFile/1024)
	}
}
