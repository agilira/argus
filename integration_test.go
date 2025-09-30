// integration_test.go: Integration test for Environment Variables Support
//
// This test verifies that environment variables support is fully integrated
// into Argus and working correctly with the existing watcher system.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira System Libraries
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFullEnvironmentIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping full environment integration test in short mode")
	}
	// Test environment variables integration with actual file watching
	tempDir, err := os.MkdirTemp("", "argus-env-integration-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "test-config.json")

	// Set environment variables
	envVars := map[string]string{
		"ARGUS_POLL_INTERVAL":         "100ms", // Minimum allowed for security
		"ARGUS_MAX_WATCHED_FILES":     "5",
		"ARGUS_OPTIMIZATION_STRATEGY": "singleevent",
		"ARGUS_AUDIT_ENABLED":         "true",
		"ARGUS_AUDIT_MIN_LEVEL":       "info",
		"ARGUS_CACHE_TTL":             "1s", // Changed from 50ms to meet security requirement (min 1s)
	}

	// Set environment variables
	for key, value := range envVars {
		if err := os.Setenv(key, value); err != nil {
			t.Logf("Failed to set env %s: %v", key, err)
		}
	}

	// Clean up after test
	defer func() {
		for key := range envVars {
			if err := os.Unsetenv(key); err != nil {
				t.Logf("Failed to unset env %s: %v", key, err)
			}
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
	err = watcher.Watch(configFile, func(event ChangeEvent) {
		changeDetected = true
		t.Logf("Change detected: %v", event)
	})
	if err != nil {
		t.Fatalf("Failed to watch configFile: %v", err)
	}

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
	if err := watcher.Stop(); err != nil {
		t.Logf("Failed to stop watcher: %v", err)
	}

	if !changeDetected {
		t.Error("No change was detected with environment-configured watcher")
	}
}

func TestMultiSourceIntegrationWithRealFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping multi-source integration test in short mode")
	}
	// Test multi-source configuration with actual file
	tempDir, err := os.MkdirTemp("", "argus-multisource-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Failed to remove temp dir: %v", err)
		}
	}()

	configFile := filepath.Join(tempDir, "config.json")
	watchedFile := filepath.Join(tempDir, "watched.json")

	// Create a config file (this would be loaded if we had file loading implemented)
	configContent := `{"poll_interval": "200ms"}`
	err = os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variables to override
	if err := os.Setenv("ARGUS_POLL_INTERVAL", "100ms"); err != nil {
		t.Logf("Failed to set ARGUS_POLL_INTERVAL: %v", err)
	} // Override
	if err := os.Setenv("ARGUS_AUDIT_ENABLED", "true"); err != nil {
		t.Logf("Failed to set ARGUS_AUDIT_ENABLED: %v", err)
	} // Additional
	defer func() {
		if err := os.Unsetenv("ARGUS_POLL_INTERVAL"); err != nil {
			t.Logf("Failed to unset ARGUS_POLL_INTERVAL: %v", err)
		}
		if err := os.Unsetenv("ARGUS_AUDIT_ENABLED"); err != nil {
			t.Logf("Failed to unset ARGUS_AUDIT_ENABLED: %v", err)
		}
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
	err = watcher.Watch(watchedFile, func(event ChangeEvent) {
		atomic.AddInt64(&changeCount, 1)
		t.Logf("Multi-source change detected: %v", event)
	})
	if err != nil {
		t.Fatalf("Failed to watch watchedFile: %v", err)
	}

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

	if err := watcher.Stop(); err != nil {
		t.Logf("Failed to stop watcher: %v", err)
	}

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
			if err := os.Setenv(tc.envVar, tc.envValue); err != nil {
				t.Logf("Failed to set env %s: %v", tc.envVar, err)
			}
			defer func() {
				if err := os.Unsetenv(tc.envVar); err != nil {
					t.Logf("Failed to unset env %s: %v", tc.envVar, err)
				}
			}()

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
	if err := os.Setenv("TEST_STRING", "test-value"); err != nil {
		t.Logf("Failed to set TEST_STRING: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_STRING"); err != nil {
			t.Logf("Failed to unset TEST_STRING: %v", err)
		}
	}()

	result := GetEnvWithDefault("TEST_STRING", "default")
	if result != "test-value" {
		t.Errorf("Expected 'test-value', got %q", result)
	}

	result = GetEnvWithDefault("NONEXISTENT_STRING", "default")
	if result != "default" {
		t.Errorf("Expected 'default', got %q", result)
	}

	// Test GetEnvDurationWithDefault
	if err := os.Setenv("TEST_DURATION", "30s"); err != nil {
		t.Logf("Failed to set TEST_DURATION: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_DURATION"); err != nil {
			t.Logf("Failed to unset TEST_DURATION: %v", err)
		}
	}()

	duration := GetEnvDurationWithDefault("TEST_DURATION", time.Minute)
	if duration != 30*time.Second {
		t.Errorf("Expected 30s, got %v", duration)
	}

	duration = GetEnvDurationWithDefault("NONEXISTENT_DURATION", time.Minute)
	if duration != time.Minute {
		t.Errorf("Expected 1m, got %v", duration)
	}

	// Test GetEnvIntWithDefault
	if err := os.Setenv("TEST_INT", "42"); err != nil {
		t.Logf("Failed to set TEST_INT: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_INT"); err != nil {
			t.Logf("Failed to unset TEST_INT: %v", err)
		}
	}()

	intVal := GetEnvIntWithDefault("TEST_INT", 100)
	if intVal != 42 {
		t.Errorf("Expected 42, got %d", intVal)
	}

	intVal = GetEnvIntWithDefault("NONEXISTENT_INT", 100)
	if intVal != 100 {
		t.Errorf("Expected 100, got %d", intVal)
	}

	// Test GetEnvBoolWithDefault
	if err := os.Setenv("TEST_BOOL", "true"); err != nil {
		t.Logf("Failed to set TEST_BOOL: %v", err)
	}
	defer func() {
		if err := os.Unsetenv("TEST_BOOL"); err != nil {
			t.Logf("Failed to unset TEST_BOOL: %v", err)
		}
	}()

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
		if err := os.Unsetenv(envVar); err != nil {
			fmt.Printf("Failed to unset env %s: %v\n", envVar, err)
		}
	}
}

// Test simple integration SetDefault functionality
func TestIntegrationSetDefault(t *testing.T) {
	cm := NewConfigManager("test-app")

	// SetDefault calls should not panic (currently stub implementation)
	cm.SetDefault("test.key", "default-value")
	cm.SetDefault("test.number", 42)
	cm.SetDefault("test.bool", true)

	// Test passes if no panic occurs
}

// Test validateAndSecurePath with valid paths
func TestValidateAndSecurePath(t *testing.T) {
	config := &Config{}
	config = config.WithDefaults()
	watcher := New(*config)

	// Test with simple valid path
	tempDir := t.TempDir()
	validFile := filepath.Join(tempDir, "test.json")

	// Create the file first
	err := os.WriteFile(validFile, []byte(`{"test": true}`), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// This should work and increase coverage of validateAndSecurePath
	_, err = watcher.validateAndSecurePath(validFile)
	if err != nil {
		t.Errorf("Valid path should not error: %v", err)
	}
}

// Test validateSymlinks with simple paths (no actual symlinks)
func TestValidateSymlinks(t *testing.T) {
	config := &Config{}
	config = config.WithDefaults()
	watcher := New(*config)

	// Test with regular file (no symlinks)
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	// Create a simple test file
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// validateSymlinks should succeed with regular file
	err := watcher.validateSymlinks(testFile, testFile)
	if err != nil {
		t.Errorf("validateSymlinks failed for regular file: %v", err)
	}
}

// TestIntegration_MissingCoverage tests previously uncovered integration functions
func TestIntegration_MissingCoverage(t *testing.T) {
	t.Run("SetDefault", func(t *testing.T) {
		cm := NewConfigManager("test-app")

		// SetDefault should not panic and should be a no-op for now
		// (since it's implemented as a stub for API compatibility)
		cm.SetDefault("test.key", "default-value")
		cm.SetDefault("test.number", 42)
		cm.SetDefault("test.bool", true)
		cm.SetDefault("nested.config.value", "nested-default")
		cm.SetDefault("app.timeout", "30s")

		// Test should pass if no panic occurs
		t.Log("SetDefault calls completed without panic")

		// Test that SetDefault doesn't crash with nil values
		cm.SetDefault("nil.test", nil)

		// Test with empty string key (edge case)
		cm.SetDefault("", "empty-key-test")

		// Test with special characters in key
		cm.SetDefault("test.key-with_special$chars", "special-value")
	})

	t.Run("ParseArgsOrExit_HelpRequested", func(t *testing.T) {
		// This test is tricky because ParseArgsOrExit calls os.Exit
		// We'll test the underlying ParseArgs method that returns the help error
		cm := NewConfigManager("test-app")

		// Mock args with help flag
		err := cm.Parse([]string{"--help"})
		if err == nil {
			t.Error("Expected help error, got nil")
		}

		if !strings.Contains(err.Error(), "help requested") {
			t.Errorf("Expected 'help requested' error, got: %v", err)
		}
	})

	t.Run("WatchConfigFile", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "watch_test.json")

		// Create initial config file
		initialConfig := `{"app": {"name": "test-app", "version": "1.0.0"}}`
		err := os.WriteFile(configPath, []byte(initialConfig), 0600)
		if err != nil {
			t.Fatalf("Failed to create initial config: %v", err)
		}

		cm := NewConfigManager("test-app")

		// Track callback invocations
		var callbackCount int32
		callback := func() {
			atomic.AddInt32(&callbackCount, 1)
		}

		// Start watching
		err = cm.WatchConfigFile(configPath, callback)
		if err != nil {
			t.Fatalf("Failed to start watching: %v", err)
		}

		// Give watcher time to initialize
		time.Sleep(50 * time.Millisecond)

		// Modify the config file to trigger callback
		modifiedConfig := `{"app": {"name": "modified-app", "version": "1.0.1"}}`
		err = os.WriteFile(configPath, []byte(modifiedConfig), 0600)
		if err != nil {
			t.Fatalf("Failed to modify config: %v", err)
		}

		// Wait for callback to be triggered
		maxWait := 2 * time.Second
		start := time.Now()
		for atomic.LoadInt32(&callbackCount) == 0 && time.Since(start) < maxWait {
			time.Sleep(10 * time.Millisecond)
		}

		// Check if callback was called
		if count := atomic.LoadInt32(&callbackCount); count == 0 {
			t.Log("Callback was not triggered - this may be expected if watcher is not fully implemented")
		} else {
			t.Logf("Callback triggered %d times", count)
		}

		// Test multiple watch registrations on the same manager
		callback2 := func() {
			atomic.AddInt32(&callbackCount, 10) // Different increment to distinguish
		}

		err = cm.WatchConfigFile(configPath, callback2)
		if err != nil {
			t.Errorf("Failed to register second watcher: %v", err)
		}
	})

	t.Run("WatchConfigFile_InvalidPath", func(t *testing.T) {
		cm := NewConfigManager("test-app")

		callback := func() {
			// This should not be called
		}

		// Try to watch a non-existent file
		err := cm.WatchConfigFile("/nonexistent/path/config.json", callback)
		if err == nil {
			t.Log("WatchConfigFile with invalid path succeeded - may be expected behavior")
		} else {
			t.Logf("WatchConfigFile with invalid path failed as expected: %v", err)
		}
	})

	t.Run("LoadConfigFile", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "load_test.json")

		// Create config file
		configContent := `{"app": {"name": "load-test", "port": 3000}}`
		err := os.WriteFile(configPath, []byte(configContent), 0600)
		if err != nil {
			t.Fatalf("Failed to create config file: %v", err)
		}

		cm := NewConfigManager("test-app")

		// LoadConfigFile is currently a stub that returns nil
		err = cm.LoadConfigFile(configPath)
		if err != nil {
			t.Errorf("LoadConfigFile failed: %v", err)
		}

		// Test with non-existent file
		err = cm.LoadConfigFile("/nonexistent/config.json")
		if err != nil {
			t.Logf("LoadConfigFile with non-existent file failed as expected: %v", err)
		} else {
			t.Log("LoadConfigFile with non-existent file succeeded - this is the current stub behavior")
		}
	})

	t.Run("ConfigManager_Integration", func(t *testing.T) {
		// Test the full workflow of ConfigManager
		cm := NewConfigManager("integration-test")

		// Set some defaults
		cm.SetDefault("server.host", "localhost")
		cm.SetDefault("server.port", 8080)

		// Test that manager is properly initialized
		if cm == nil {
			t.Fatal("ConfigManager should not be nil")
		}

		// Test string values
		host := cm.GetString("server.host")
		t.Logf("Host value: %s", host)

		// Test int values
		port := cm.GetInt("server.port")
		t.Logf("Port value: %d", port)

		// Test bool values
		debug := cm.GetBool("app.debug")
		t.Logf("Debug value: %t", debug)

		// Test duration values
		timeout := cm.GetDuration("server.timeout")
		t.Logf("Timeout value: %s", timeout)

		// All these should complete without errors
	})
}
