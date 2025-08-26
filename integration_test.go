// integration_test.go: Integration test for Environment Variables Support
//
// This test verifies that environment variables support is fully integrated
// into Argus and working correctly with the existing watcher system.
//
// Copyright (c) 2025 AGILira
// Series: AGILira System Libraries
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestFullEnvironmentIntegration(t *testing.T) {
	// Test environment variables integration with actual file watching
	tempDir, err := os.MkdirTemp("", "argus-env-integration-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "test-config.json")

	// Set environment variables
	envVars := map[string]string{
		"ARGUS_POLL_INTERVAL":         "100ms", // Fast for testing
		"ARGUS_MAX_WATCHED_FILES":     "5",
		"ARGUS_OPTIMIZATION_STRATEGY": "singleevent",
		"ARGUS_AUDIT_ENABLED":         "true",
		"ARGUS_AUDIT_MIN_LEVEL":       "info",
		"ARGUS_CACHE_TTL":             "50ms",
	}

	// Set environment variables
	for key, value := range envVars {
		os.Setenv(key, value)
	}

	// Clean up after test
	defer func() {
		for key := range envVars {
			os.Unsetenv(key)
		}
	}()

	// Load configuration from environment
	config, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("Failed to load config from env: %v", err)
	}

	// Verify environment variables were applied
	if config.PollInterval != 100*time.Millisecond {
		t.Errorf("Expected PollInterval 100ms, got %v", config.PollInterval)
	}

	if config.MaxWatchedFiles != 5 {
		t.Errorf("Expected MaxWatchedFiles 5, got %d", config.MaxWatchedFiles)
	}

	if config.OptimizationStrategy != OptimizationSingleEvent {
		t.Errorf("Expected OptimizationSingleEvent, got %v", config.OptimizationStrategy)
	}

	if !config.Audit.Enabled {
		t.Error("Expected audit to be enabled")
	}

	// Create watcher with environment configuration
	watcher := New(*config)
	if watcher == nil {
		t.Fatal("Failed to create watcher with environment config")
	}

	// Test that watcher works with environment configuration
	changeDetected := false
	watcher.Watch(configFile, func(event ChangeEvent) {
		changeDetected = true
		t.Logf("Change detected: %v", event)
	})

	// Start watcher
	err = watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Create initial file
	initialContent := `{"test": "initial"}`
	err = os.WriteFile(configFile, []byte(initialContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write initial file: %v", err)
	}

	// Wait for change detection
	time.Sleep(300 * time.Millisecond) // 3x poll interval

	// Modify file to trigger change
	modifiedContent := `{"test": "modified"}`
	err = os.WriteFile(configFile, []byte(modifiedContent), 0644)
	if err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Wait for change detection
	time.Sleep(300 * time.Millisecond) // 3x poll interval

	// Stop watcher
	watcher.Stop()

	if !changeDetected {
		t.Error("No change was detected with environment-configured watcher")
	}
}

func TestMultiSourceIntegrationWithRealFile(t *testing.T) {
	// Test multi-source configuration with actual file
	tempDir, err := os.MkdirTemp("", "argus-multisource-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, "config.json")
	watchedFile := filepath.Join(tempDir, "watched.json")

	// Create a config file (this would be loaded if we had file loading implemented)
	configContent := `{"poll_interval": "200ms"}`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variables to override
	os.Setenv("ARGUS_POLL_INTERVAL", "100ms") // Override
	os.Setenv("ARGUS_AUDIT_ENABLED", "true")  // Additional
	defer func() {
		os.Unsetenv("ARGUS_POLL_INTERVAL")
		os.Unsetenv("ARGUS_AUDIT_ENABLED")
	}()

	// Load multi-source configuration
	config, err := LoadConfigMultiSource(configFile)
	if err != nil {
		t.Fatalf("Failed to load multi-source config: %v", err)
	}

	// Environment should override
	if config.PollInterval != 100*time.Millisecond {
		t.Errorf("Expected environment override 100ms, got %v", config.PollInterval)
	}

	// Environment addition should work
	if !config.Audit.Enabled {
		t.Error("Expected audit enabled from environment")
	}

	// Defaults should be present for unset values
	if config.MaxWatchedFiles != 100 { // Default value
		t.Errorf("Expected default MaxWatchedFiles 100, got %d", config.MaxWatchedFiles)
	}

	// Test that watcher works with multi-source configuration
	watcher := New(*config)
	if watcher == nil {
		t.Fatal("Failed to create watcher with multi-source config")
	}

	changeCount := int64(0)
	watcher.Watch(watchedFile, func(event ChangeEvent) {
		atomic.AddInt64(&changeCount, 1)
		t.Logf("Multi-source change detected: %v", event)
	})

	err = watcher.Start()
	if err != nil {
		t.Fatalf("Failed to start multi-source watcher: %v", err)
	}

	// Create and modify watched file
	content := `{"data": "test"}`
	err = os.WriteFile(watchedFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to write watched file: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	modifiedContent := `{"data": "modified"}`
	err = os.WriteFile(watchedFile, []byte(modifiedContent), 0644)
	if err != nil {
		t.Fatalf("Failed to modify watched file: %v", err)
	}

	time.Sleep(300 * time.Millisecond)

	watcher.Stop()

	if atomic.LoadInt64(&changeCount) == 0 {
		t.Error("No changes detected with multi-source configured watcher")
	}
}

func TestEnvironmentVariableValidation(t *testing.T) {
	// Test that invalid environment variables are properly handled
	testCases := []struct {
		name        string
		envVar      string
		envValue    string
		expectError bool
	}{
		{"valid duration", "ARGUS_POLL_INTERVAL", "5s", false},
		{"invalid duration", "ARGUS_POLL_INTERVAL", "invalid", true},
		{"valid int", "ARGUS_MAX_WATCHED_FILES", "100", false},
		{"invalid int", "ARGUS_MAX_WATCHED_FILES", "not-a-number", true},
		{"valid strategy", "ARGUS_OPTIMIZATION_STRATEGY", "auto", false},
		{"invalid strategy", "ARGUS_OPTIMIZATION_STRATEGY", "invalid-strategy", true},
		{"valid capacity", "ARGUS_BOREAS_CAPACITY", "256", false},
		{"invalid capacity", "ARGUS_BOREAS_CAPACITY", "not-a-number", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clear all env vars first
			clearArgusEnvVars()

			// Set the test env var
			os.Setenv(tc.envVar, tc.envValue)
			defer os.Unsetenv(tc.envVar)

			// Try to load config
			_, err := LoadConfigFromEnv()

			if tc.expectError && err == nil {
				t.Errorf("Expected error for %s=%s, but got none", tc.envVar, tc.envValue)
			}

			if !tc.expectError && err != nil {
				t.Errorf("Unexpected error for %s=%s: %v", tc.envVar, tc.envValue, err)
			}
		})
	}
}

func TestEnvironmentVariableHelpers(t *testing.T) {
	// Test the utility helper functions

	// Test GetEnvWithDefault
	os.Setenv("TEST_STRING", "test-value")
	defer os.Unsetenv("TEST_STRING")

	result := GetEnvWithDefault("TEST_STRING", "default")
	if result != "test-value" {
		t.Errorf("Expected 'test-value', got %q", result)
	}

	result = GetEnvWithDefault("NONEXISTENT_STRING", "default")
	if result != "default" {
		t.Errorf("Expected 'default', got %q", result)
	}

	// Test GetEnvDurationWithDefault
	os.Setenv("TEST_DURATION", "30s")
	defer os.Unsetenv("TEST_DURATION")

	duration := GetEnvDurationWithDefault("TEST_DURATION", time.Minute)
	if duration != 30*time.Second {
		t.Errorf("Expected 30s, got %v", duration)
	}

	duration = GetEnvDurationWithDefault("NONEXISTENT_DURATION", time.Minute)
	if duration != time.Minute {
		t.Errorf("Expected 1m, got %v", duration)
	}

	// Test GetEnvIntWithDefault
	os.Setenv("TEST_INT", "42")
	defer os.Unsetenv("TEST_INT")

	intVal := GetEnvIntWithDefault("TEST_INT", 100)
	if intVal != 42 {
		t.Errorf("Expected 42, got %d", intVal)
	}

	intVal = GetEnvIntWithDefault("NONEXISTENT_INT", 100)
	if intVal != 100 {
		t.Errorf("Expected 100, got %d", intVal)
	}

	// Test GetEnvBoolWithDefault
	os.Setenv("TEST_BOOL", "true")
	defer os.Unsetenv("TEST_BOOL")

	boolVal := GetEnvBoolWithDefault("TEST_BOOL", false)
	if !boolVal {
		t.Error("Expected true, got false")
	}

	boolVal = GetEnvBoolWithDefault("NONEXISTENT_BOOL", false)
	if boolVal {
		t.Error("Expected false, got true")
	}
}

// Helper function to clear all Argus environment variables
func clearArgusEnvVars() {
	envVars := []string{
		"ARGUS_POLL_INTERVAL",
		"ARGUS_CACHE_TTL",
		"ARGUS_MAX_WATCHED_FILES",
		"ARGUS_OPTIMIZATION_STRATEGY",
		"ARGUS_BOREAS_CAPACITY",
		"ARGUS_AUDIT_ENABLED",
		"ARGUS_AUDIT_OUTPUT_FILE",
		"ARGUS_AUDIT_MIN_LEVEL",
		"ARGUS_AUDIT_BUFFER_SIZE",
		"ARGUS_AUDIT_FLUSH_INTERVAL",
	}

	for _, envVar := range envVars {
		os.Unsetenv(envVar)
	}
}
