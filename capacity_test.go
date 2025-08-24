// capacity_test.go: Testing Argus Capacity
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestArgus_FileCapacity(t *testing.T) {
	// Test della capacità di Argus con BoreasLite
	tempDir, err := os.MkdirTemp("", "argus_capacity_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Config con limiti aumentati per il test
	config := Config{
		PollInterval:    100 * time.Millisecond, // Polling veloce per test
		CacheTTL:        50 * time.Millisecond,
		MaxWatchedFiles: 500, // Aumentiamo per il test
	}

	watcher := New(config)
	defer watcher.Stop()

	// Contatori thread-safe
	var eventCount atomic.Int64
	var fileCount atomic.Int64
	var mu sync.Mutex
	receivedEvents := make(map[string]bool)

	// Callback per contare eventi
	callback := func(event ChangeEvent) {
		eventCount.Add(1)
		mu.Lock()
		receivedEvents[event.Path] = true
		mu.Unlock()
		if eventCount.Load() <= 10 { // Log solo i primi
			t.Logf("Event %d: %s", eventCount.Load(), filepath.Base(event.Path))
		}
	}

	// Creiamo molti file e li aggiungiamo al watcher
	maxFiles := 200 // Test con 200 files
	filePaths := make([]string, maxFiles)

	t.Logf("Creating and watching %d files...", maxFiles)

	for i := 0; i < maxFiles; i++ {
		fileName := fmt.Sprintf("test_file_%04d.json", i)
		filePath := filepath.Join(tempDir, fileName)
		filePaths[i] = filePath

		// Crea il file
		if err := os.WriteFile(filePath, []byte(fmt.Sprintf(`{"id": %d}`, i)), 0644); err != nil {
			t.Fatal(err)
		}

		// Aggiungi al watcher
		if err := watcher.Watch(filePath, callback); err != nil {
			t.Fatalf("Failed to watch file %d: %v", i, err)
		}

		fileCount.Add(1)

		if i > 0 && i%50 == 0 {
			t.Logf("Added %d files to watcher", i)
		}
	}

	t.Logf("Starting watcher with %d files...", fileCount.Load())
	watcher.Start()
	time.Sleep(200 * time.Millisecond) // Inizializzazione

	// Ora modifichiamo tutti i file
	t.Logf("Modifying all %d files...", maxFiles)
	startTime := time.Now()

	for i, filePath := range filePaths {
		content := fmt.Sprintf(`{"id": %d, "modified": %d}`, i, time.Now().UnixNano())
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		if i > 0 && i%50 == 0 {
			t.Logf("Modified %d files", i)
		}
	}

	modifyDuration := time.Since(startTime)
	t.Logf("Modified %d files in %v", maxFiles, modifyDuration)

	// Aspettiamo che tutti gli eventi arrivino
	timeout := 10 * time.Second
	startWait := time.Now()

	for time.Since(startWait) < timeout {
		events := eventCount.Load()
		mu.Lock()
		uniqueFiles := len(receivedEvents)
		mu.Unlock()

		t.Logf("Events received: %d, Unique files: %d", events, uniqueFiles)

		if uniqueFiles >= maxFiles {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	finalEvents := eventCount.Load()
	mu.Lock()
	uniqueFiles := len(receivedEvents)
	mu.Unlock()

	// Report finale
	t.Logf("\n=== ARGUS CAPACITY TEST RESULTS ===")
	t.Logf("Files watched: %d", fileCount.Load())
	t.Logf("Events received: %d", finalEvents)
	t.Logf("Unique files with events: %d", uniqueFiles)
	t.Logf("Event rate: %.2f events/second", float64(finalEvents)/time.Since(startTime).Seconds())

	// Statistiche BoreasLite
	if watcher.eventRing != nil {
		stats := watcher.eventRing.Stats()
		t.Logf("\nBoreasLite Stats:")
		for key, value := range stats {
			t.Logf("  %s: %d", key, value)
		}

		// Controllo performance
		processed := stats["items_processed"]
		dropped := stats["items_dropped"]

		if dropped > 0 {
			t.Errorf("PERFORMANCE ISSUE: %d events dropped!", dropped)
		}

		if processed < int64(maxFiles) {
			t.Logf("WARNING: Expected at least %d processed events, got %d", maxFiles, processed)
		}
	}

	// Verifica che la maggior parte dei file abbia ricevuto eventi
	successRate := float64(uniqueFiles) / float64(maxFiles) * 100
	t.Logf("Success rate: %.1f%% (%d/%d files)", successRate, uniqueFiles, maxFiles)

	if successRate < 90 {
		t.Errorf("Low success rate: %.1f%% - Expected at least 90%%", successRate)
	} else {
		t.Logf("✅ SUCCESS: Argus handled %d files with %.1f%% success rate", maxFiles, successRate)
	}
}
