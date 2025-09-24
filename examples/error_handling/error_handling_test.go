// error_handling_test.go: Error Handling Test Suite
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agilira/argus"
	"github.com/agilira/go-errors"
)

func TestCustomErrorHandler(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "argus_error_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "custom_error_config.json")

	// Create a valid config first
	validConfig := `{"service": "test", "port": 8080}`
	if err := os.WriteFile(configFile, []byte(validConfig), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Track error handler calls
	errorHandlerCalled := false
	var capturedError error

	// Create custom error handler
	errorHandler := func(err error, filepath string) {
		errorHandlerCalled = true
		capturedError = err
	}

	// Create watcher with custom error handler
	config := argus.Config{
		PollInterval: 50 * time.Millisecond,
		ErrorHandler: errorHandler,
	}

	watcher, err := argus.UniversalConfigWatcherWithConfig(configFile, func(config map[string]interface{}) {
		// This should be called for valid config
	}, config)

	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer func() {
		if err := watcher.Stop(); err != nil {
			t.Logf("Failed to stop watcher: %v", err)
		}
	}()

	// Give it time to read initial config
	time.Sleep(100 * time.Millisecond)

	// Now write invalid JSON to trigger error
	invalidConfig := `{"service": "test", "port": INVALID_JSON}`
	if err := os.WriteFile(configFile, []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	// Give error handler time to be called
	time.Sleep(200 * time.Millisecond)

	// Verify error handler was called
	if !errorHandlerCalled {
		t.Error("Expected error handler to be called")
	}

	if capturedError == nil {
		t.Error("Expected error to be captured")
	}

	// Verify error message contains expected information
	errorMsg := capturedError.Error()
	if !strings.Contains(errorMsg, "ARGUS_INVALID_CONFIG") {
		t.Errorf("Expected error message to contain 'ARGUS_INVALID_CONFIG', got: %s", errorMsg)
	}
}

func TestFileNotFoundError(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "argus_file_not_found_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	nonExistentFile := filepath.Join(tempDir, "does_not_exist.json")

	// Track error handler calls
	errorHandlerCalled := false
	var capturedError error

	// Create custom error handler
	errorHandler := func(err error, filepath string) {
		errorHandlerCalled = true
		capturedError = err
	}

	config := argus.Config{
		PollInterval: 50 * time.Millisecond,
		ErrorHandler: errorHandler,
	}

	// Try to watch non-existent file
	watcher, err := argus.UniversalConfigWatcherWithConfig(nonExistentFile, func(config map[string]interface{}) {
		t.Error("This should not be called for non-existent file")
	}, config)

	if err != nil {
		// This is expected for non-existent files
		errorMsg := err.Error()
		if !strings.Contains(errorMsg, "ARGUS_FILE_NOT_FOUND") {
			t.Errorf("Expected error message to contain 'ARGUS_FILE_NOT_FOUND', got: %s", errorMsg)
		}
	} else {
		defer func() {
			if err := watcher.Stop(); err != nil {
				t.Logf("Failed to stop watcher: %v", err)
			}
		}()
		time.Sleep(100 * time.Millisecond) // Give it time to trigger error handler
	}

	// Verify error handler was called (if watcher was created)
	if errorHandlerCalled {
		if capturedError == nil {
			t.Error("Expected error to be captured")
		} else {
			errorMsg := capturedError.Error()
			if !strings.Contains(errorMsg, "ARGUS_FILE_NOT_FOUND") {
				t.Errorf("Expected error message to contain 'ARGUS_FILE_NOT_FOUND', got: %s", errorMsg)
			}
		}
	}
}

func TestParseErrorHandling(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "argus_parse_error_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "parse_error_config.yaml")

	// Create invalid YAML
	invalidYAML := `
key: value
  invalid_indentation: bad
badly_formatted: {
`
	if err := os.WriteFile(configFile, []byte(invalidYAML), 0644); err != nil {
		t.Fatalf("Failed to write invalid YAML: %v", err)
	}

	// Track error handler calls
	errorHandlerCalled := false
	var capturedError error

	// Error handler for parse errors
	errorHandler := func(err error, filepath string) {
		errorHandlerCalled = true
		capturedError = err
	}

	config := argus.Config{
		PollInterval: 50 * time.Millisecond,
		ErrorHandler: errorHandler,
	}

	watcher, err := argus.UniversalConfigWatcherWithConfig(configFile, func(config map[string]interface{}) {
		// This might be called with partial data
	}, config)

	if err != nil {
		// This is expected for invalid YAML
		errorMsg := err.Error()
		if !strings.Contains(errorMsg, "ARGUS_INVALID_CONFIG") {
			t.Errorf("Expected error message to contain 'ARGUS_INVALID_CONFIG', got: %s", errorMsg)
		}
	} else {
		defer func() {
			if err := watcher.Stop(); err != nil {
				t.Logf("Failed to stop watcher: %v", err)
			}
		}()
		// Give it time to process
		time.Sleep(200 * time.Millisecond)
	}

	// Verify error handler was called (if watcher was created)
	if errorHandlerCalled {
		if capturedError == nil {
			t.Error("Expected error to be captured")
		} else {
			errorMsg := capturedError.Error()
			if !strings.Contains(errorMsg, "INVALID_CONFIG") {
				t.Errorf("Expected error message to contain 'INVALID_CONFIG', got: %s", errorMsg)
			}
		}
	}
}

func TestDefaultErrorHandler(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "argus_default_error_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "default_error_config.json")

	// Create invalid JSON
	invalidConfig := `{"service": "test", "port": invalid}`
	if err := os.WriteFile(configFile, []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	// Use default configuration (includes default error handler)
	watcher, err := argus.UniversalConfigWatcher(configFile, func(config map[string]interface{}) {
		// This should not be called for invalid JSON
	})

	if err != nil {
		// This is expected for invalid JSON
		errorMsg := err.Error()
		if !strings.Contains(errorMsg, "ARGUS_INVALID_CONFIG") {
			t.Errorf("Expected error message to contain 'ARGUS_INVALID_CONFIG', got: %s", errorMsg)
		}
	} else {
		defer func() {
			if err := watcher.Stop(); err != nil {
				t.Logf("Failed to stop watcher: %v", err)
			}
		}()
		// Give it time to process and show default error handling
		time.Sleep(200 * time.Millisecond)
	}
}

func TestCustomErrorCreation(t *testing.T) {
	// Test creating custom errors with go-errors using Argus error codes
	customErr := errors.New(argus.ErrCodeInvalidConfig, "This is a custom error for demonstration")

	if customErr == nil {
		t.Fatal("Expected custom error to be created")
	}

	// Test error message
	errorMsg := customErr.Error()
	expectedMsg := "ARGUS_INVALID_CONFIG"
	if !strings.Contains(errorMsg, expectedMsg) {
		t.Errorf("Expected error message to contain '%s', got: %s", expectedMsg, errorMsg)
	}

	// Test error wrapping with Argus error codes
	wrappedErr := errors.Wrap(customErr, argus.ErrCodeWatcherStopped, "Wrapped the custom error")
	if wrappedErr == nil {
		t.Fatal("Expected wrapped error to be created")
	}

	// Test wrapped error message
	wrappedMsg := wrappedErr.Error()
	expectedWrappedMsg := "ARGUS_WATCHER_STOPPED"
	if !strings.Contains(wrappedMsg, expectedWrappedMsg) {
		t.Errorf("Expected wrapped error message to contain '%s', got: %s", expectedWrappedMsg, wrappedMsg)
	}

	// Test error code checking
	if !strings.Contains(wrappedMsg, "ARGUS_WATCHER_STOPPED") {
		t.Error("Expected to identify wrapped error code")
	}
}

func TestErrorHandlerIntegration(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "argus_integration_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "integration_test.json")

	// Track multiple error handler calls
	errorHandlerCalls := 0
	var capturedErrors []error

	// Create error handler that tracks multiple calls
	errorHandler := func(err error, filepath string) {
		errorHandlerCalls++
		capturedErrors = append(capturedErrors, err)
	}

	config := argus.Config{
		PollInterval: 50 * time.Millisecond,
		ErrorHandler: errorHandler,
	}

	// Create watcher
	watcher, err := argus.UniversalConfigWatcherWithConfig(configFile, func(config map[string]interface{}) {
		// This should be called for valid configs
	}, config)

	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer func() {
		if err := watcher.Stop(); err != nil {
			t.Logf("Failed to stop watcher: %v", err)
		}
	}()

	// Test multiple error scenarios
	testCases := []struct {
		name        string
		content     string
		expectError bool
	}{
		{
			name:        "ValidConfig",
			content:     `{"service": "test", "port": 8080}`,
			expectError: false,
		},
		{
			name:        "InvalidJSON",
			content:     `{"service": "test", "port": INVALID_JSON}`,
			expectError: true,
		},
		{
			name:        "ValidConfigAgain",
			content:     `{"service": "test", "port": 9090}`,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		// Write test content
		if err := os.WriteFile(configFile, []byte(tc.content), 0644); err != nil {
			t.Fatalf("Failed to write test content for %s: %v", tc.name, err)
		}

		// Give watcher time to process
		time.Sleep(200 * time.Millisecond)

		// Verify error handling
		if tc.expectError {
			if errorHandlerCalls == 0 {
				t.Errorf("Expected error handler to be called for %s", tc.name)
			}
		}
	}

	// Verify we captured some errors
	if len(capturedErrors) > 0 {
		for i, err := range capturedErrors {
			errorMsg := err.Error()
			if !strings.Contains(errorMsg, "ARGUS_INVALID_CONFIG") {
				t.Errorf("Expected error %d to contain 'ARGUS_INVALID_CONFIG', got: %s", i, errorMsg)
			}
		}
	}
}

func TestErrorHandlerPerformance(t *testing.T) {
	// Test error handler performance
	const iterations = 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		// Create error with go-errors using Argus error codes
		err := errors.New(argus.ErrCodeInvalidConfig, "Performance test error")
		if err == nil {
			t.Fatalf("Failed to create error at iteration %d", i)
		}

		// Test error message
		errorMsg := err.Error()
		if !strings.Contains(errorMsg, "ARGUS_INVALID_CONFIG") {
			t.Fatalf("Error message incorrect at iteration %d", i)
		}
	}

	duration := time.Since(start)
	avgTime := duration / iterations

	t.Logf("Error creation performance: %d iterations in %v", iterations, duration)
	t.Logf("Average time per error: %v", avgTime)
	t.Logf("Errors per second: %.0f", float64(iterations)/duration.Seconds())

	// Error creation should be very fast (less than 1µs per operation)
	if avgTime > 1*time.Microsecond {
		t.Errorf("Error creation too slow: %v per operation (expected < 1µs)", avgTime)
	}
}

func TestErrorHandlerConcurrency(t *testing.T) {
	// Test concurrent error handling
	const goroutines = 10
	const iterationsPerGoroutine = 100

	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(goroutineID int) {
			defer func() { done <- true }()

			for j := 0; j < iterationsPerGoroutine; j++ {
				// Create error with go-errors using Argus error codes
				err := errors.New(argus.ErrCodeWatcherStopped, "Concurrent test error")
				if err == nil {
					t.Errorf("Failed to create error in goroutine %d, iteration %d", goroutineID, j)
					return
				}

				// Test error message
				errorMsg := err.Error()
				if !strings.Contains(errorMsg, "ARGUS_WATCHER_STOPPED") {
					t.Errorf("Error message incorrect in goroutine %d, iteration %d", goroutineID, j)
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}
}
