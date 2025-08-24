// long_path_test.go: Testing Argus Long Path Handling
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBoreasLite_PathLengths(t *testing.T) {
	// Test various path lengths to verify our 110-byte buffer works correctly

	// Test 1: Short path (like integration test that works)
	tempDir1, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir1)

	shortFile := filepath.Join(tempDir1, "file.json")
	t.Logf("Test 1 - Short path: %s (len=%d)", shortFile, len(shortFile))

	testPath(t, shortFile, "short path test")

	// Test 2: Medium path (around 80 chars)
	tempDir2, err := os.MkdirTemp("", "longer_directory_name_for_testing")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir2)

	mediumFile := filepath.Join(tempDir2, "file_with_somewhat_longer_name_for_testing.json")
	t.Logf("Test 2 - Medium path: %s (len=%d)", mediumFile, len(mediumFile))

	// Skip if path is too long for our buffer (especially on Windows)
	if len(mediumFile) >= 110 {
		t.Logf("Medium path length (%d) exceeds buffer capacity (110 bytes) - skipping", len(mediumFile))
		t.Skip("Skipping medium path test due to platform path length limitations")
		return
	}

	testPath(t, mediumFile, "medium path test")

	// Test 3: Long path (close to 109 chars limit)
	tempDir3, err := os.MkdirTemp("", "very_long_directory_name_for_buffer_testing")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir3)

	// Create a filename that gets us close to 109 characters
	baseLength := len(tempDir3) + 1     // +1 for path separator
	remainingLength := 105 - baseLength // Leave some margin

	var longFileName string
	if remainingLength > 20 {
		longFileName = "very_long_filename_" + strings.Repeat("a", remainingLength-20) + ".json"
	} else {
		longFileName = "long.json"
	}

	longFile := filepath.Join(tempDir3, longFileName)
	t.Logf("Test 3 - Long path: %s (len=%d)", longFile, len(longFile))

	if len(longFile) >= 110 {
		t.Logf("Path length (%d) exceeds buffer capacity (110 bytes) - this is an expected limitation on macOS", len(longFile))
		t.Skip("Skipping long path test due to platform path length limitations")
		return
	}

	testPath(t, longFile, "long path test")
}

func testPath(t *testing.T, filePath, testName string) {
	t.Logf("Starting %s for: %s", testName, filePath)

	// Create file FIRST (like integration test)
	if err := os.WriteFile(filePath, []byte("initial content"), 0644); err != nil {
		t.Fatalf("%s failed to create file: %v", testName, err)
	}

	// Use slower config for CI reliability
	watcher := New(Config{
		PollInterval: 200 * time.Millisecond, // Slower for CI
		CacheTTL:     100 * time.Millisecond,
	})
	defer watcher.Stop()

	var events []ChangeEvent
	var eventsMutex sync.Mutex

	if err := watcher.Watch(filePath, func(event ChangeEvent) {
		eventsMutex.Lock()
		events = append(events, event)
		eventsMutex.Unlock()
		t.Logf("%s - Event: Path=%s", testName, event.Path)
	}); err != nil {
		t.Fatalf("%s failed to watch: %v", testName, err)
	}

	watcher.Start()
	time.Sleep(300 * time.Millisecond) // Longer setup for CI

	// Clear initial events (like integration test)
	eventsMutex.Lock()
	events = nil
	eventsMutex.Unlock()

	// Modify file (should trigger modify event)
	if err := os.WriteFile(filePath, []byte("modified content"), 0644); err != nil {
		t.Fatalf("%s failed to modify file: %v", testName, err)
	}

	// Wait for events with extended retry logic for CI environments
	maxWait := 30 // Up to 6 seconds for slow CI
	for i := 0; i < maxWait; i++ {
		eventsMutex.Lock()
		eventsLength := len(events)
		eventsMutex.Unlock()
		if eventsLength > 0 {
			break
		}
		time.Sleep(200 * time.Millisecond) // Longer intervals
	}

	// Check stats
	if watcher.eventRing != nil {
		stats := watcher.eventRing.Stats()
		t.Logf("%s - BoreasLite stats: %+v", testName, stats)
	}

	eventsMutex.Lock()
	eventsLength := len(events)
	var firstEvent ChangeEvent
	if eventsLength > 0 {
		firstEvent = events[0]
	}
	eventsMutex.Unlock()

	if eventsLength == 0 {
		t.Errorf("%s: No events received!", testName)
	} else {
		t.Logf("%s: SUCCESS - Got %d events", testName, eventsLength)

		// Verify path integrity
		if firstEvent.Path != filePath {
			t.Errorf("%s: Path mismatch - expected %s, got %s", testName, filePath, firstEvent.Path)
		} else {
			t.Logf("%s: Path integrity verified", testName)
		}
	}
}
