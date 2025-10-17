// boreaslite_test.go - BoreasLite - Xantos Powered 3rd tier. - test suite
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/agilira/go-timecache"
)

// TestFileChangeEventSize ensures FileChangeEvent is exactly 128 bytes
// This is critical for performance - the struct must fit exactly in 2 cache lines
func TestFileChangeEventSize(t *testing.T) {
	var event FileChangeEvent
	actualSize := unsafe.Sizeof(event)
	expectedSize := uintptr(128)

	if actualSize != expectedSize {
		t.Errorf("FileChangeEvent size is %d bytes, expected exactly %d bytes", actualSize, expectedSize)
		t.Errorf("Current struct layout:")
		t.Errorf("  ModTime: int64    = %d bytes", unsafe.Sizeof(event.ModTime))
		t.Errorf("  Size:    int64    = %d bytes", unsafe.Sizeof(event.Size))
		t.Errorf("  Path:    [110]byte = %d bytes", unsafe.Sizeof(event.Path))
		t.Errorf("  PathLen: uint8    = %d bytes", unsafe.Sizeof(event.PathLen))
		t.Errorf("  Flags:   uint8    = %d bytes", unsafe.Sizeof(event.Flags))
		t.Errorf("This breaks the 2-cache-line optimization and may impact performance")
	}
}

// TestFileChangeEventFieldAssignment ensures field reordering doesn't break functionality
func TestFileChangeEventFieldAssignment(t *testing.T) {
	testPath := "/test/config.json"
	testModTime := int64(1234567890)
	testSize := int64(2048)
	testFlags := uint8(FileEventModify | FileEventCreate)

	event := FileChangeEvent{
		ModTime: testModTime,
		Size:    testSize,
		PathLen: uint8(len(testPath)),
		Flags:   testFlags,
	}
	copy(event.Path[:], []byte(testPath))

	// Verify all fields are correctly assigned
	if event.ModTime != testModTime {
		t.Errorf("ModTime not assigned correctly: got %d, expected %d", event.ModTime, testModTime)
	}
	if event.Size != testSize {
		t.Errorf("Size not assigned correctly: got %d, expected %d", event.Size, testSize)
	}
	if event.PathLen != uint8(len(testPath)) {
		t.Errorf("PathLen not assigned correctly: got %d, expected %d", event.PathLen, len(testPath))
	}
	if event.Flags != testFlags {
		t.Errorf("Flags not assigned correctly: got %d, expected %d", event.Flags, testFlags)
	}

	// Verify path is correctly copied
	retrievedPath := string(event.Path[:event.PathLen])
	if retrievedPath != testPath {
		t.Errorf("Path not copied correctly: got %s, expected %s", retrievedPath, testPath)
	}
}

// Benchmark BoreasLite write performance
func BenchmarkBoreasLite_WriteFileEvent(b *testing.B) {
	processor := func(*FileChangeEvent) {
		// Minimal processing for pure write benchmark
	}

	boreas := NewBoreasLite(256, OptimizationSmallBatch, processor)
	defer boreas.Stop()

	// Start processor in background
	go boreas.RunProcessor()

	event := FileChangeEvent{
		ModTime: timecache.CachedTimeNano(),
		Size:    1024,
		Flags:   FileEventModify,
		PathLen: 11,
	}
	copy(event.Path[:], []byte("config.json"))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		boreas.WriteFileEvent(&event)
	}
}

// Benchmark BoreasLite convenience method
func BenchmarkBoreasLite_WriteFileChange(b *testing.B) {
	processor := func(*FileChangeEvent) {
		// Minimal processing
	}

	boreas := NewBoreasLite(256, OptimizationSmallBatch, processor)
	defer boreas.Stop()

	go boreas.RunProcessor()

	modTime := timecache.CachedTime()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		boreas.WriteFileChange("config.json", modTime, 1024, false, false, true)
	}
}

// Benchmark current Argus direct callback approach (for comparison)
func BenchmarkArgus_DirectCallback(b *testing.B) {
	var callbackCount int64
	callback := func(_ ChangeEvent) {
		// Simulate minimal callback processing
		callbackCount++
	}

	event := ChangeEvent{
		Path:     "config.json",
		ModTime:  timecache.CachedTime(),
		Size:     1024,
		IsModify: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		callback(event)
	}
}

// Benchmark BoreasLite vs Go channels (traditional approach)
func BenchmarkBoreasLite_vsChannels(b *testing.B) {
	b.Run("BoreasLite", func(b *testing.B) {
		var processed int64
		processor := func(*FileChangeEvent) {
			processed++
		}

		boreas := NewBoreasLite(256, OptimizationSmallBatch, processor)
		defer boreas.Stop()

		go boreas.RunProcessor()

		event := FileChangeEvent{
			ModTime: time.Now().UnixNano(),
			Size:    1024,
			Flags:   FileEventModify,
			PathLen: 11,
		}
		copy(event.Path[:], []byte("config.json"))

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			boreas.WriteFileEvent(&event)
		}
	})

	b.Run("GoChannels", func(b *testing.B) {
		var processed int64
		eventCh := make(chan ChangeEvent, 256)

		// Start processor goroutine
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for event := range eventCh {
				_ = event // Simulate processing
				processed++
			}
		}()

		event := ChangeEvent{
			Path:     "config.json",
			ModTime:  time.Now(),
			Size:     1024,
			IsModify: true,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			eventCh <- event
		}

		close(eventCh)
		wg.Wait()
	})
}

// Benchmark concurrent writers (MPSC test)
func BenchmarkBoreasLite_MPSC(b *testing.B) {
	processor := func(*FileChangeEvent) {
		// Minimal processing
	}

	boreas := NewBoreasLite(1024, OptimizationSmallBatch, processor)
	defer boreas.Stop()

	go boreas.RunProcessor()

	event := FileChangeEvent{
		ModTime: time.Now().UnixNano(),
		Size:    1024,
		Flags:   FileEventModify,
		PathLen: 11,
	}
	copy(event.Path[:], []byte("config.json"))

	// Test with multiple concurrent writers
	numWriters := runtime.GOMAXPROCS(0)

	b.ResetTimer()

	var wg sync.WaitGroup
	for w := 0; w < numWriters; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/numWriters; i++ {
				boreas.WriteFileEvent(&event)
			}
		}()
	}
	wg.Wait()
}

// Benchmark batch processing performance
func BenchmarkBoreasLite_ProcessBatch(b *testing.B) {
	processor := func(*FileChangeEvent) {
		// Minimal processing
		runtime.KeepAlive("processed")
	}

	boreas := NewBoreasLite(256, OptimizationSmallBatch, processor)
	defer boreas.Stop()

	// Fill buffer with events
	event := FileChangeEvent{
		ModTime: time.Now().UnixNano(),
		Size:    1024,
		Flags:   FileEventModify,
		PathLen: 11,
	}
	copy(event.Path[:], []byte("config.json"))

	// Pre-fill buffer
	for i := 0; i < 100; i++ {
		boreas.WriteFileEvent(&event)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		boreas.ProcessBatch()
		// Refill for next iteration
		boreas.WriteFileEvent(&event)
	}
}

// Test conversion performance
func BenchmarkBoreasLite_Conversion(b *testing.B) {
	changeEvent := ChangeEvent{
		Path:     "config.json",
		ModTime:  time.Now(),
		Size:     1024,
		IsModify: true,
	}

	b.Run("ToFileEvent", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = ConvertChangeEventToFileEvent(changeEvent)
		}
	})

	fileEvent := ConvertChangeEventToFileEvent(changeEvent)

	b.Run("FromFileEvent", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = ConvertFileEventToChangeEvent(fileEvent)
		}
	})
}

func TestConvertChangeEventToFileEvent(t *testing.T) {
	// Test complete event conversion
	changeEvent := ChangeEvent{
		Path:     "/test/config.json",
		ModTime:  time.Now(),
		Size:     1024,
		IsCreate: true,
		IsDelete: false,
	}

	fileEvent := ConvertChangeEventToFileEvent(changeEvent)

	// Verify conversion
	if fileEvent.ModTime != changeEvent.ModTime.UnixNano() {
		t.Error("ModTime not converted correctly")
	}
	if fileEvent.Size != changeEvent.Size {
		t.Error("Size not converted correctly")
	}
	if fileEvent.Flags&FileEventCreate == 0 {
		t.Error("Create flag not set")
	}
}
