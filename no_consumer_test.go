// no_consumer_test.go: Testing Argus consumer overhead
//
// Copyright (c) 2025 AGILira - A. Giordano
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

// Specific benchmark to measure overhead when Argus is set up but no consumer is running
func BenchmarkArgus_NoConsumer(b *testing.B) {
	tempDir, err := os.MkdirTemp("", "no_consumer_test")
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			b.Logf("Failed to remove tempDir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "logger.json")
	initialConfig := `{"level": "info", "output": "stdout", "format": "json"}`

	if err := os.WriteFile(configFile, []byte(initialConfig), 0644); err != nil {
		b.Fatal(err)
	}

	var logCount atomic.Int64

	// Setup Argus without starting the consumer
	config := Config{
		PollInterval:         100 * time.Millisecond,
		OptimizationStrategy: OptimizationSingleEvent,
	}

	watcher := New(*config.WithDefaults())
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Logf("Failed to stop watcher: %v", err)
		}
	}()

	if err := watcher.Watch(configFile, func(event ChangeEvent) {
		// Empty callback
	}); err != nil {
		b.Fatalf("Failed to watch config file: %v", err)
	}

	// NO watcher.Start() - no consumer in background
	time.Sleep(10 * time.Millisecond)

	logEntry := func(level string, message string) {
		logCount.Add(1)
		_ = level + ": " + message
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		logEntry("INFO", "Request processed")
	}

	b.StopTimer()
	b.Logf("Logged %d entries", logCount.Load())
}
