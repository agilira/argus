// single_event_bench_test.go: Testing Argus Single Event Processing
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
	"time"
)

// Benchmark specifico per scenari 1-2 files (single event processing)
func BenchmarkBoreasLite_SingleEvent(b *testing.B) {
	var processed int64
	processor := func(*FileChangeEvent) {
		processed++
	}

	boreas := NewBoreasLite(256, OptimizationSingleEvent, processor)
	defer boreas.Stop()
	// Non avviare RunProcessor in background per il benchmark

	event := FileChangeEvent{
		ModTime: time.Now().UnixNano(),
		Size:    1024,
		Flags:   FileEventModify,
		PathLen: 9,
	}
	copy(event.Path[:], []byte("test.json"))

	b.ResetTimer()

	// Simula scenario reale: scrivi 1 evento, processa immediatamente
	for i := 0; i < b.N; i++ {
		// Scrivi singolo evento (come 1-file scenario)
		if !boreas.WriteFileEvent(&event) {
			b.Fatal("Failed to write event")
		}

		// Processa manualmente per il benchmark
		boreas.ProcessBatch()
	}
}

// Benchmark per confrontare la velocità di processing batch vs single
func BenchmarkBoreasLite_ProcessingStrategy(b *testing.B) {
	var processed int64
	processor := func(*FileChangeEvent) {
		processed++
	}

	b.Run("ProcessBatch_SingleEvent", func(b *testing.B) {
		boreas := NewBoreasLite(256, OptimizationSingleEvent, processor)
		defer boreas.Stop()

		// Pre-popola con un singolo evento
		event := FileChangeEvent{
			ModTime: time.Now().UnixNano(),
			Size:    1024,
			Flags:   FileEventModify,
			PathLen: 9,
		}
		copy(event.Path[:], []byte("test.json"))

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simula il pattern: scrivi 1, processa 1
			boreas.WriteFileEvent(&event)
			boreas.ProcessBatch() // Dovrebbe usare il fast path
		}
	})

	b.Run("ProcessBatch_MultipleEvents", func(b *testing.B) {
		boreas := NewBoreasLite(256, OptimizationSingleEvent, processor)
		defer boreas.Stop()

		events := make([]FileChangeEvent, 8)
		for i := range events {
			events[i] = FileChangeEvent{
				ModTime: time.Now().UnixNano(),
				Size:    1024,
				Flags:   FileEventModify,
				PathLen: 9,
			}
			copy(events[i].Path[:], []byte("test.json"))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simula il pattern: scrivi molti, processa tutti
			for j := range events {
				boreas.WriteFileEvent(&events[j])
			}
			boreas.ProcessBatch() // Dovrebbe usare l'unrolled path
		}
	})
}
