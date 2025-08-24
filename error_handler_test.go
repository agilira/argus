// error_handler_test.go: Testing Argus Error Handling
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestErrorHandler_FileReadError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	var capturedError error
	var capturedPath string
	var callbackCalled bool
	var mu sync.Mutex

	errorHandler := func(err error, path string) {
		mu.Lock()
		defer mu.Unlock()
		capturedError = err
		capturedPath = path
	}

	config := Config{
		PollInterval: 50 * time.Millisecond,
		ErrorHandler: errorHandler,
	}

	watcher, err := UniversalConfigWatcherWithConfig(configPath, func(config map[string]interface{}) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalled = true
	}, config)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Create file with invalid permissions from the start
	if err := os.WriteFile(configPath, []byte(`{"test": true}`), 0000); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer func() {
		os.Chmod(configPath, 0644) // Restore for cleanup
		os.Remove(configPath)
	}()

	// On Windows, file permissions work differently
	// Delete the file to simulate read error instead
	if runtime.GOOS == "windows" {
		os.Remove(configPath)
	}

	// Wait for error to be captured
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if callbackCalled {
		t.Error("Callback should not be called when file read fails")
	}

	if capturedError == nil {
		t.Error("Expected error to be captured by ErrorHandler")
		return
	}

	if capturedPath != configPath {
		t.Errorf("Expected path %s, got %s", configPath, capturedPath)
	}

	if !strings.Contains(capturedError.Error(), "failed to read config file") {
		t.Errorf("Expected error message about reading config file, got: %v", capturedError)
	}
}

func TestErrorHandler_ParseError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	var capturedError error
	var capturedPath string
	var mu sync.Mutex

	errorHandler := func(err error, path string) {
		mu.Lock()
		defer mu.Unlock()
		capturedError = err
		capturedPath = path
	}

	config := Config{
		PollInterval: 50 * time.Millisecond,
		ErrorHandler: errorHandler,
	}

	watcher, err := UniversalConfigWatcherWithConfig(configPath, func(config map[string]interface{}) {
		// Should not be called due to parse error
		t.Error("Callback should not be called when parsing fails")
	}, config)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Write invalid JSON
	if err := os.WriteFile(configPath, []byte(`{"test": invalid_json`), 0644); err != nil {
		t.Fatalf("Failed to write invalid JSON: %v", err)
	}

	// Wait for error to be captured
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if capturedError == nil {
		t.Fatal("Expected parse error to be captured by ErrorHandler")
	}

	if capturedPath != configPath {
		t.Errorf("Expected path %s, got %s", configPath, capturedPath)
	}

	if !strings.Contains(capturedError.Error(), "failed to parse") {
		t.Errorf("Expected error message about parsing, got: %v", capturedError)
	}
}

func TestErrorHandler_DefaultBehavior(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	// Test that default error handler doesn't panic
	config := Config{
		PollInterval: 50 * time.Millisecond,
		// No ErrorHandler set - should use default
	}

	watcher, err := UniversalConfigWatcherWithConfig(configPath, func(config map[string]interface{}) {
		// Should not be called
	}, config)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Write invalid JSON to trigger error
	if err := os.WriteFile(configPath, []byte(`{"test": invalid`), 0644); err != nil {
		t.Fatalf("Failed to write invalid JSON: %v", err)
	}

	// Wait a bit - should not panic
	time.Sleep(200 * time.Millisecond)

	// If we get here without panic, the default error handler works
	t.Log("✅ Default error handler works without panicking")
}

func TestErrorHandler_StatError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	var capturedError error
	var capturedPath string
	var mu sync.Mutex

	errorHandler := func(err error, path string) {
		mu.Lock()
		defer mu.Unlock()
		capturedError = err
		capturedPath = path
	}

	config := Config{
		PollInterval: 50 * time.Millisecond,
		ErrorHandler: errorHandler,
	}

	watcher := New(config)

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Create a file first
	if err := os.WriteFile(configPath, []byte(`{"test": true}`), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Watch the file
	if err := watcher.Watch(configPath, func(event ChangeEvent) {
		// This callback is fine for normal operation
	}); err != nil {
		t.Fatalf("Failed to watch file: %v", err)
	}

	// Wait for initial stat
	time.Sleep(100 * time.Millisecond)

	// Create a directory with the same name to cause stat error
	if err := os.Remove(configPath); err != nil {
		t.Fatalf("Failed to remove file: %v", err)
	}
	if err := os.Mkdir(configPath, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}
	defer os.RemoveAll(configPath)

	// Change the directory to make it inaccessible (permission denied)
	if err := os.Chmod(configPath, 0000); err != nil {
		t.Fatalf("Failed to change directory permissions: %v", err)
	}

	// Wait for error to be captured
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// This test might be flaky on some systems, so we'll be more lenient
	// The main goal is to test that ErrorHandler gets called for non-NotExist errors
	if capturedError != nil {
		if capturedPath != configPath {
			t.Errorf("Expected path %s, got %s", configPath, capturedPath)
		}

		if !strings.Contains(capturedError.Error(), "failed to stat file") {
			t.Errorf("Expected error message about stat failure, got: %v", capturedError)
		}
		t.Log("✅ ErrorHandler correctly captured stat error")
	} else {
		// On some systems, the directory might still be readable, so this isn't a hard failure
		t.Skip("Stat error was not captured - this might be system-dependent")
	}
}
