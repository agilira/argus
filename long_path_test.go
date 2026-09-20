// long_path_test.go: Testing Argus Long Path Handling
//
// Copyright (c) 2025 AGILira - A. Giordano
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
	// Test various path lengths, including one past the inline buffer

	// Test 1: Short path (like integration test that works)
	tempDir1, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir1); err != nil {
			t.Logf("Failed to remove tempDir1: %v", err)
		}
	}()

	shortFile := filepath.Join(tempDir1, "file.json")
	t.Logf("Test 1 - Short path: %s (len=%d)", shortFile, len(shortFile))

	testPath(t, shortFile, "short path test")

	// Test 2: Medium path (around 80 chars)
	tempDir2, err := os.MkdirTemp("", "longer_directory_name_for_testing")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir2); err != nil {
			t.Logf("Failed to remove tempDir2: %v", err)
		}
	}()

	mediumFile := filepath.Join(tempDir2, "file_with_somewhat_longer_name_for_testing.json")
	t.Logf("Test 2 - Medium path: %s (len=%d)", mediumFile, len(mediumFile))

	// No skip: events are resolved by WatchID, so delivery no longer depends on
	// the path fitting the inline buffer. Skipping here is what hid the bug.
	testPath(t, mediumFile, "medium path test")

	// Test 3: Long path (close to the inline buffer limit)
	tempDir3, err := os.MkdirTemp("", "very_long_directory_name_for_buffer_testing")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir3); err != nil {
			t.Logf("Failed to remove tempDir3: %v", err)
		}
	}()

	// Create a filename that gets us close to the inline buffer limit
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

	testPath(t, longFile, "long path test")

	// Test 4: A path that deliberately exceeds the inline buffer. This is the
	// case the suite used to skip; it must deliver events like any other.
	tempDir4, err := os.MkdirTemp("", "path_longer_than_the_inline_event_buffer")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir4); err != nil {
			t.Logf("Failed to remove tempDir4: %v", err)
		}
	}()

	deep := tempDir4
	for len(deep) < 140 {
		deep = filepath.Join(deep, "nested_directory_segment")
	}
	if err := os.MkdirAll(deep, 0o750); err != nil {
		t.Fatal(err)
	}
	overflowFile := filepath.Join(deep, "config.json")
	t.Logf("Test 4 - Overflow path: %s (len=%d)", overflowFile, len(overflowFile))
	if len(overflowFile) <= maxInlinePathLen {
		t.Fatalf("test setup: path is only %d bytes", len(overflowFile))
	}

	testPath(t, overflowFile, "overflow path test")
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
	defer func() {
		if err := watcher.Stop(); err != nil {
			t.Logf("Failed to stop watcher: %v", err)
		}
	}()

	var events []ChangeEvent
	var eventsMutex sync.Mutex

	err := watcher.Watch(filePath, func(event ChangeEvent) {
		eventsMutex.Lock()
		events = append(events, event)
		eventsMutex.Unlock()
		t.Logf("%s - Event: Path=%s", testName, event.Path)
	})
	if err != nil {
		t.Fatalf("%s failed to watch: %v", testName, err)
	}
	if err := watcher.Start(); err != nil {
		t.Fatalf("%s failed to start watcher: %v", testName, err)
	}
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
