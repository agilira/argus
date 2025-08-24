// path_limits_test.go: Testing Argus Path Length Limits
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBoreasLite_PathLimits(t *testing.T) {
	// Test with a simple, short path first
	tempDir, err := os.MkdirTemp("", "test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.json")
	t.Logf("Testing with path: %s (len=%d)", testFile, len(testFile))

	// Start watching BEFORE creating the file (like integration test)
	config := Config{
		PollInterval:         50 * time.Millisecond, // Polling più frequente per test
		CacheTTL:             25 * time.Millisecond,
		OptimizationStrategy: OptimizationSingleEvent, // Strategia per test singolo file
	}
	watcher := New(*config.WithDefaults())
	defer watcher.Stop()

	var events []ChangeEvent

	if err := watcher.Watch(testFile, func(event ChangeEvent) {
		events = append(events, event)
		t.Logf("Event: Path=%s (len=%d)", event.Path, len(event.Path))
	}); err != nil {
		t.Fatal(err)
	}

	watcher.Start()
	time.Sleep(100 * time.Millisecond)

	t.Logf("Watcher started, now creating file...")

	// NOW create the file (like integration test)
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Logf("File created, waiting for events...")

	time.Sleep(200 * time.Millisecond) // Let first event settle

	t.Logf("Now modifying file...")

	// MODIFY the file (this should definitely trigger)
	if err := os.WriteFile(testFile, []byte("modified content"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Logf("File modified, waiting for events...")

	time.Sleep(500 * time.Millisecond) // Wait longer

	// Check BoreasLite stats
	if watcher.eventRing != nil {
		stats := watcher.eventRing.Stats()
		t.Logf("BoreasLite stats: %+v", stats)
	}

	if len(events) == 0 {
		t.Error("No events received!")
	} else {
		t.Logf("SUCCESS: Got %d events", len(events))
	}
}
