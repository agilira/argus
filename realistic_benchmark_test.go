// realistic_benchmark_test.go: Testing Argus Realistic Benchmarking
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Benchmark realistico: confronta architetture complete di file watching
func BenchmarkRealWorldArchitectures(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "realistic_bench")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Crea file di test
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

// Traditional polling approach (come facevano i vecchi file watchers)
func benchmarkTraditionalPolling(b *testing.B, testFile string) {
	var eventCount atomic.Int64
	callback := func(_ string) {
		eventCount.Add(1)
	}

	// Simula polling tradizionale
	lastStat, _ := os.Stat(testFile)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simula il ciclo completo di polling tradizionale
		stat, err := os.Stat(testFile)
		if err == nil && !stat.ModTime().Equal(lastStat.ModTime()) {
			callback(testFile)
			lastStat = stat
		}
	}
}

// BoreasLite con strategia SingleEvent (scenario realistico)
func benchmarkBoreasLiteSingle(b *testing.B, testFile string) {
	var eventCount atomic.Int64
	processor := func(event *FileChangeEvent) {
		eventCount.Add(1)
	}

	boreas := NewBoreasLite(64, OptimizationSingleEvent, processor)
	defer boreas.Stop()

	// Event realistico
	event := FileChangeEvent{
		ModTime: time.Now().UnixNano(),
		Size:    1024,
		Flags:   FileEventModify,
		PathLen: uint8(len(testFile)),
	}
	copy(event.Path[:], []byte(testFile))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simula il ciclo completo: write + process
		boreas.WriteFileEvent(&event)
		boreas.ProcessBatch()
	}
}

// BoreasLite con strategia SmallBatch (scenario realistico)
func benchmarkBoreasLiteSmall(b *testing.B, testFile string) {
	var eventCount atomic.Int64
	processor := func(event *FileChangeEvent) {
		eventCount.Add(1)
	}

	boreas := NewBoreasLite(128, OptimizationSmallBatch, processor)
	defer boreas.Stop()

	// Event realistico
	event := FileChangeEvent{
		ModTime: time.Now().UnixNano(),
		Size:    1024,
		Flags:   FileEventModify,
		PathLen: uint8(len(testFile)),
	}
	copy(event.Path[:], []byte(testFile))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simula il ciclo completo: write + process
		boreas.WriteFileEvent(&event)
		boreas.ProcessBatch()
	}
}

// DirectCallback - MA con context realistico per confronto teorico
func benchmarkDirectCallbackTheoretical(b *testing.B) {
	var eventCount atomic.Int64
	callback := func(event ChangeEvent) {
		// Simula processing realistico del callback
		eventCount.Add(1)
		_ = event.Path
		_ = event.ModTime
		_ = event.Size
	}

	event := ChangeEvent{
		Path:     "config.json",
		ModTime:  time.Now(),
		Size:     1024,
		IsModify: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// ATTENZIONE: Questo è SOLO il costo del callback
		// NON include file system polling, detection, conversion
		callback(event)
	}
}

// Benchmark end-to-end realistico con file system reale
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
		// Modifica file
		content := []byte(`{"benchmark": true, "iteration": ` + string(rune(i)) + `}`)
		os.WriteFile(testFile, content, 0644)

		// Polling tradizionale
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
		// Modifica file reale
		content := []byte(`{"benchmark": true, "iteration": ` + string(rune(i)) + `}`)
		os.WriteFile(testFile, content, 0644)

		// Piccola pausa per permettere il detection
		time.Sleep(2 * time.Millisecond)
	}

	// Aspetta che tutti gli eventi siano processati
	time.Sleep(10 * time.Millisecond)
}
