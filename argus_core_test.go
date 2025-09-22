// argus_core_test.go - Comprehensive test suite for Argus Dynamic Configuration Framework
//
// Test Philosophy:
// - DRY principle: Common test utilities and helpers
// - OS-aware: Works on Windows, Linux, macOS
// - Smart assertions: Meaningful error messages
// - No false positives: Proper timing and synchronization
// - Comprehensive coverage: All public APIs and edge cases
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test configuration constants
const (
	// Fast test intervals for CI/dev environments
	testPollInterval = 50 * time.Millisecond
	testCacheTTL     = 25 * time.Millisecond
	testWaitTime     = 100 * time.Millisecond
	testTimeout      = 2 * time.Second

	// Test file content
	initialTestContent = `{"version": 1, "enabled": true}`
	updatedTestContent = `{"version": 2, "enabled": false}`
)

// testHelper provides common test utilities following DRY principle
type testHelper struct {
	t         *testing.T
	tempDir   string
	tempFiles []string
	cleanup   []func()
}

// newTestHelper creates a new test helper with OS-aware temp directory
func newTestHelper(t *testing.T) *testHelper {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "argus_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	return &testHelper{
		t:       t,
		tempDir: tempDir,
		cleanup: make([]func(), 0),
	}
}

// createTestFile creates a temporary test file with given content
func (h *testHelper) createTestFile(name string, content string) string {
	h.t.Helper()

	filePath := filepath.Join(h.tempDir, name)
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		h.t.Fatalf("Failed to create test file %s: %v", filePath, err)
	}

	h.tempFiles = append(h.tempFiles, filePath)
	return filePath
}

// updateTestFile updates an existing test file with new content
func (h *testHelper) updateTestFile(filePath string, content string) {
	h.t.Helper()

	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		h.t.Fatalf("Failed to update test file %s: %v", filePath, err)
	}
}

// deleteTestFile removes a test file (for deletion tests)
func (h *testHelper) deleteTestFile(filePath string) {
	h.t.Helper()

	if err := os.Remove(filePath); err != nil {
		h.t.Fatalf("Failed to delete test file %s: %v", filePath, err)
	}
}

// createWatcher creates a watcher with test-optimized configuration
func (h *testHelper) createWatcher() *Watcher {
	h.t.Helper()

	config := Config{
		PollInterval:    testPollInterval,
		CacheTTL:        testCacheTTL,
		MaxWatchedFiles: 100,
	}

	watcher := New(config)
	h.cleanup = append(h.cleanup, func() {
		if watcher.IsRunning() {
			watcher.Stop()
		}
	})

	return watcher
}

// waitForChanges waits for expected number of changes with timeout
func (h *testHelper) waitForChanges(changesChan <-chan ChangeEvent, expectedCount int, timeout time.Duration) []ChangeEvent {
	h.t.Helper()

	var changes []ChangeEvent
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for len(changes) < expectedCount {
		select {
		case change := <-changesChan:
			changes = append(changes, change)
			h.t.Logf("Change detected: %+v", change)
		case <-timer.C:
			h.t.Fatalf("Timeout waiting for changes. Expected %d, got %d changes: %+v",
				expectedCount, len(changes), changes)
		}
	}

	return changes
}

// waitWithNoChanges waits and ensures no unexpected changes occur
func (h *testHelper) waitWithNoChanges(changesChan <-chan ChangeEvent, duration time.Duration) {
	h.t.Helper()

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case change := <-changesChan:
		h.t.Fatalf("Unexpected change detected: %+v", change)
	case <-timer.C:
		// Expected - no changes
	}
}

// Close cleans up test resources
func (h *testHelper) Close() {
	// Run cleanup functions in reverse order
	for i := len(h.cleanup) - 1; i >= 0; i-- {
		h.cleanup[i]()
	}

	// Remove temp directory
	if h.tempDir != "" {
		os.RemoveAll(h.tempDir)
	}
}

// Test configuration validation and defaults
func TestConfig_WithDefaults(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		input          Config
		expectedFields map[string]interface{}
	}{
		{
			name:  "empty_config_gets_defaults",
			input: Config{},
			expectedFields: map[string]interface{}{
				"PollInterval":    5 * time.Second,
				"CacheTTL":        2500 * time.Millisecond, // PollInterval / 2
				"MaxWatchedFiles": 100,
			},
		},
		{
			name: "partial_config_preserves_values",
			input: Config{
				PollInterval: 1 * time.Second,
			},
			expectedFields: map[string]interface{}{
				"PollInterval":    1 * time.Second,
				"CacheTTL":        500 * time.Millisecond, // PollInterval / 2
				"MaxWatchedFiles": 100,
			},
		},
		{
			name: "custom_config_preserved",
			input: Config{
				PollInterval:    10 * time.Second,
				CacheTTL:        5 * time.Second,
				MaxWatchedFiles: 50,
			},
			expectedFields: map[string]interface{}{
				"PollInterval":    10 * time.Second,
				"CacheTTL":        5 * time.Second,
				"MaxWatchedFiles": 50,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result := tc.input.WithDefaults()

			// Verify specific fields
			if result.PollInterval != tc.expectedFields["PollInterval"] {
				t.Errorf("PollInterval: expected %v, got %v",
					tc.expectedFields["PollInterval"], result.PollInterval)
			}
			if result.CacheTTL != tc.expectedFields["CacheTTL"] {
				t.Errorf("CacheTTL: expected %v, got %v",
					tc.expectedFields["CacheTTL"], result.CacheTTL)
			}
			if result.MaxWatchedFiles != tc.expectedFields["MaxWatchedFiles"] {
				t.Errorf("MaxWatchedFiles: expected %v, got %v",
					tc.expectedFields["MaxWatchedFiles"], result.MaxWatchedFiles)
			}
		})
	}
}

// Test watcher creation and basic state
func TestWatcher_New(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		config Config
	}{
		{
			name:   "default_config",
			config: Config{},
		},
		{
			name: "custom_config",
			config: Config{
				PollInterval:    1 * time.Second,
				CacheTTL:        500 * time.Millisecond,
				MaxWatchedFiles: 50,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			watcher := New(tc.config)

			// Verify initial state
			if watcher == nil {
				t.Fatal("New() returned nil watcher")
			}

			if watcher.IsRunning() {
				t.Error("New watcher should not be running")
			}

			if watcher.WatchedFiles() != 0 {
				t.Errorf("New watcher should have 0 watched files, got %d", watcher.WatchedFiles())
			}

			// Verify cache stats
			stats := watcher.GetCacheStats()
			if stats.Entries != 0 {
				t.Errorf("New watcher cache should be empty, got %d entries", stats.Entries)
			}
		})
	}
}

// Test file watching lifecycle (core functionality)
func TestWatcher_FileWatchingLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping lifecycle test in short mode")
	}

	helper := newTestHelper(t)
	defer helper.Close()

	// Create test file
	testFile := helper.createTestFile("test_config.json", initialTestContent)

	// Create watcher
	watcher := helper.createWatcher()

	// Set up change tracking
	changesChan := make(chan ChangeEvent, 10)
	var changesCount int32

	err := watcher.Watch(testFile, func(event ChangeEvent) {
		atomic.AddInt32(&changesCount, 1)
		select {
		case changesChan <- event:
		default:
			t.Error("Changes channel overflow")
		}
	})

	if err != nil {
		t.Fatalf("Failed to watch file: %v", err)
	}

	// Verify file is being watched
	if watcher.WatchedFiles() != 1 {
		t.Errorf("Expected 1 watched file, got %d", watcher.WatchedFiles())
	}

	// Start watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	if !watcher.IsRunning() {
		t.Error("Watcher should be running after Start()")
	}

	// Wait for initial stabilization
	time.Sleep(testWaitTime)

	// Modify file and wait for change detection
	helper.updateTestFile(testFile, updatedTestContent)
	changes := helper.waitForChanges(changesChan, 1, testTimeout)

	// Verify change event
	if len(changes) != 1 {
		t.Fatalf("Expected 1 change, got %d", len(changes))
	}

	change := changes[0]
	if change.Path != testFile {
		t.Errorf("Expected change path %s, got %s", testFile, change.Path)
	}
	if change.IsDelete {
		t.Error("File modification should not be marked as deletion")
	}

	// Stop watcher
	if err := watcher.Stop(); err != nil {
		t.Fatalf("Failed to stop watcher: %v", err)
	}

	if watcher.IsRunning() {
		t.Error("Watcher should not be running after Stop()")
	}

	// Verify no more changes are detected after stopping
	helper.updateTestFile(testFile, initialTestContent)
	helper.waitWithNoChanges(changesChan, testWaitTime*2)

	finalChangesCount := atomic.LoadInt32(&changesCount)
	t.Logf("Total changes detected: %d", finalChangesCount)
}

// Test cache behavior and TTL expiration
func TestWatcher_CacheBehaviorAndTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cache test in short mode")
	}

	helper := newTestHelper(t)
	defer helper.Close()

	// Create test file
	testFile := helper.createTestFile("cache_test.json", initialTestContent)

	// Create watcher with very short TTL for testing
	config := Config{
		PollInterval:    testPollInterval,
		CacheTTL:        testCacheTTL, // Very short TTL
		MaxWatchedFiles: 100,
	}
	watcher := New(config)
	defer func() {
		if watcher.IsRunning() {
			watcher.Stop()
		}
	}()

	// Add file to watcher
	changesChan := make(chan ChangeEvent, 10)
	err := watcher.Watch(testFile, func(event ChangeEvent) {
		changesChan <- event
	})
	if err != nil {
		t.Fatalf("Failed to watch file: %v", err)
	}

	// Start watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for initial cache population
	time.Sleep(testWaitTime)

	// Check initial cache stats
	stats := watcher.GetCacheStats()
	if stats.Entries == 0 {
		t.Error("Cache should contain at least one entry after initial scan")
	}

	t.Logf("Initial cache stats: entries=%d, oldest=%v, newest=%v",
		stats.Entries, stats.OldestAge, stats.NewestAge)

	// Wait for cache TTL to expire
	time.Sleep(testCacheTTL * 3)

	// Trigger another poll cycle
	time.Sleep(testPollInterval * 2)

	// Check cache stats again - entries may change due to TTL expiration and repopulation
	newStats := watcher.GetCacheStats()
	t.Logf("Post-TTL cache stats: entries=%d, oldest=%v, newest=%v",
		newStats.Entries, newStats.OldestAge, newStats.NewestAge)

	// Cache should still be functioning
	if newStats.Entries == 0 {
		t.Error("Cache should still contain entries after TTL cycle")
	}

	// Test manual cache clearing
	watcher.ClearCache()
	clearedStats := watcher.GetCacheStats()

	if clearedStats.Entries != 0 {
		t.Errorf("Cache should be empty after ClearCache(), got %d entries", clearedStats.Entries)
	}
}

// Test file creation and deletion detection
func TestWatcher_FileCreationAndDeletion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping creation/deletion test in short mode")
	}

	helper := newTestHelper(t)
	defer helper.Close()

	// Create initial test file
	testFile := helper.createTestFile("lifecycle_test.json", initialTestContent)

	// Create watcher
	watcher := helper.createWatcher()

	// Set up change tracking
	changesChan := make(chan ChangeEvent, 10)

	err := watcher.Watch(testFile, func(event ChangeEvent) {
		changesChan <- event
	})
	if err != nil {
		t.Fatalf("Failed to watch file: %v", err)
	}

	// Start watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for initial stabilization
	time.Sleep(testWaitTime)

	// Delete file
	helper.deleteTestFile(testFile)

	// Wait for deletion detection
	changes := helper.waitForChanges(changesChan, 1, testTimeout)

	// Verify deletion event
	if len(changes) != 1 {
		t.Fatalf("Expected 1 deletion event, got %d", len(changes))
	}

	change := changes[0]
	if change.Path != testFile {
		t.Errorf("Expected deletion path %s, got %s", testFile, change.Path)
	}
	if !change.IsDelete {
		t.Error("File deletion should be marked as deletion")
	}

	// Recreate file
	helper.createTestFile(filepath.Base(testFile), updatedTestContent)

	// Wait for recreation detection
	recreationChanges := helper.waitForChanges(changesChan, 1, testTimeout)

	// Verify recreation event
	if len(recreationChanges) != 1 {
		t.Fatalf("Expected 1 recreation event, got %d", len(recreationChanges))
	}

	recreationChange := recreationChanges[0]
	if recreationChange.IsDelete {
		t.Error("File recreation should not be marked as deletion")
	}
}

// Test multiple files watching
func TestWatcher_MultipleFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping multiple files test in short mode")
	}

	helper := newTestHelper(t)
	defer helper.Close()

	// Create multiple test files
	file1 := helper.createTestFile("config1.json", initialTestContent)
	file2 := helper.createTestFile("config2.json", initialTestContent)
	file3 := helper.createTestFile("config3.json", initialTestContent)

	// Create watcher
	watcher := helper.createWatcher()

	// Track changes per file
	changes := make(map[string][]ChangeEvent)
	var changesMutex sync.Mutex

	// Watch all files
	for _, file := range []string{file1, file2, file3} {
		err := watcher.Watch(file, func(event ChangeEvent) {
			changesMutex.Lock()
			changes[event.Path] = append(changes[event.Path], event)
			changesMutex.Unlock()
		})
		if err != nil {
			t.Fatalf("Failed to watch file %s: %v", file, err)
		}
	}

	// Verify all files are being watched
	if watcher.WatchedFiles() != 3 {
		t.Errorf("Expected 3 watched files, got %d", watcher.WatchedFiles())
	}

	// Start watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Wait for initial stabilization
	time.Sleep(testWaitTime)

	// Modify files sequentially with delays to ensure distinct events
	helper.updateTestFile(file1, updatedTestContent)
	time.Sleep(testWaitTime / 2)

	helper.updateTestFile(file2, updatedTestContent)
	time.Sleep(testWaitTime / 2)

	helper.updateTestFile(file3, updatedTestContent)

	// Wait for all changes to be detected
	time.Sleep(testTimeout)

	// Verify changes for each file
	changesMutex.Lock()
	defer changesMutex.Unlock()

	for _, file := range []string{file1, file2, file3} {
		fileChanges, exists := changes[file]
		if !exists || len(fileChanges) == 0 {
			t.Errorf("No changes detected for file %s", file)
		} else {
			t.Logf("File %s: detected %d changes", file, len(fileChanges))
		}
	}
}

// Test unwatch functionality
func TestWatcher_Unwatch(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.Close()

	// Create test file
	testFile := helper.createTestFile("unwatch_test.json", initialTestContent)

	// Create watcher
	watcher := helper.createWatcher()

	// Watch file
	changesChan := make(chan ChangeEvent, 10)
	err := watcher.Watch(testFile, func(event ChangeEvent) {
		changesChan <- event
	})
	if err != nil {
		t.Fatalf("Failed to watch file: %v", err)
	}

	// Verify file is being watched
	if watcher.WatchedFiles() != 1 {
		t.Errorf("Expected 1 watched file, got %d", watcher.WatchedFiles())
	}

	// Unwatch file
	err = watcher.Unwatch(testFile)
	if err != nil {
		t.Fatalf("Failed to unwatch file: %v", err)
	}

	// Verify file is no longer being watched
	if watcher.WatchedFiles() != 0 {
		t.Errorf("Expected 0 watched files after unwatch, got %d", watcher.WatchedFiles())
	}

	// Test unwatching non-existent file (should not error)
	err = watcher.Unwatch("/non/existent/file.json")
	if err != nil {
		t.Errorf("Unwatching non-existent file should not error, got: %v", err)
	}
}

// Test error conditions and edge cases
func TestWatcher_ErrorConditions(t *testing.T) {
	t.Parallel()

	helper := newTestHelper(t)
	defer helper.Close()

	watcher := helper.createWatcher()

	t.Run("watch_non_existent_file", func(t *testing.T) {
		// Watching non-existent file should NOT error (Argus watches paths, not just existing files)
		err := watcher.Watch("/non/existent/file.json", func(event ChangeEvent) {})
		if err != nil {
			t.Errorf("Watching non-existent file should not return error, got: %v", err)
		}
	})

	t.Run("watch_with_nil_callback", func(t *testing.T) {
		err := watcher.Watch("/some/path.json", nil)
		if err == nil {
			t.Error("Watching with nil callback should return error")
		}
	})

	t.Run("double_start", func(t *testing.T) {
		if err := watcher.Start(); err != nil {
			t.Fatalf("First start failed: %v", err)
		}

		// Second start should return error (not idempotent)
		if err := watcher.Start(); err == nil {
			t.Error("Second start should return error")
		}

		watcher.Stop()
	})

	t.Run("stop_without_start", func(t *testing.T) {
		freshWatcher := helper.createWatcher()

		// Stopping without starting should return error
		if err := freshWatcher.Stop(); err == nil {
			t.Error("Stop without start should return error")
		}
	})
}

// Test OS-specific behavior
func TestWatcher_OSSpecificBehavior(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.Close()

	t.Run("path_handling", func(t *testing.T) {
		watcher := helper.createWatcher()

		// Test with OS-specific path separators
		var testPath string
		if runtime.GOOS == "windows" {
			testPath = filepath.Join(helper.tempDir, "test\\config.json")
		} else {
			testPath = filepath.Join(helper.tempDir, "test/config.json")
		}

		// Create directory if needed
		dir := filepath.Dir(testPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		// Create file
		if err := os.WriteFile(testPath, []byte(initialTestContent), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		// Should be able to watch regardless of path format
		err := watcher.Watch(testPath, func(event ChangeEvent) {})
		if err != nil {
			t.Errorf("Should be able to watch OS-specific path %s: %v", testPath, err)
		}
	})
}

func TestWatcher_Close(t *testing.T) {
	helper := newTestHelper(t)
	defer helper.Close()

	t.Run("close_alias_for_stop", func(t *testing.T) {
		watcher := helper.createWatcher()

		// Start watcher
		if err := watcher.Start(); err != nil {
			t.Fatalf("Failed to start watcher: %v", err)
		}

		// Close should work like Stop
		if err := watcher.Close(); err != nil {
			t.Errorf("Close() failed: %v", err)
		}

		// Should be stopped now
		if watcher.IsRunning() {
			t.Error("Watcher should be stopped after Close()")
		}
	})
}
