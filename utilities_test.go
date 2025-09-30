// utilities_test.go: Testing Argus Utilities
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"
)

func TestGenericConfigWatcher(t *testing.T) {
	// Create a temporary config file
	tmpfile, err := os.CreateTemp("", "test_config_*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(tmpfile.Name()); err != nil {
			t.Logf("Failed to remove tmpfile: %v", err)
		}
	}()

	// Initial config
	config := map[string]interface{}{
		"level": "info",
		"port":  8080,
	}
	data, _ := json.Marshal(config)
	if _, err := tmpfile.Write(data); err != nil {
		t.Logf("Failed to write to tmpfile: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Logf("Failed to close tmpfile: %v", err)
	}

	// Track callback calls with mutex protection
	var mu sync.Mutex
	callCount := 0
	var lastConfig map[string]interface{}

	// Create watcher with faster polling for testing
	watcher := New(Config{PollInterval: 50 * time.Millisecond})

	configCallback := func(config map[string]interface{}) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		lastConfig = config
	}

	// Set up the generic config watcher manually
	watchCallback := func(event ChangeEvent) {
		if event.IsDelete {
			return
		}

		data, err := os.ReadFile(event.Path)
		if err != nil {
			return
		}

		var config map[string]interface{}
		if err := json.Unmarshal(data, &config); err != nil {
			return
		}

		configCallback(config)
	}

	err = watcher.Watch(tmpfile.Name(), watchCallback)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := watcher.Stop(); err != nil {
			t.Logf("Failed to stop watcher: %v", err)
		}
	}()

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Update the config file
	config["level"] = "debug"
	config["port"] = 9000
	data, _ = json.Marshal(config)
	if err := os.WriteFile(tmpfile.Name(), data, 0644); err != nil {
		t.Logf("Failed to write file: %v", err)
	}

	// Wait longer for the change to be detected (our polling is every 50ms)
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	currentCallCount := callCount
	currentLastConfig := lastConfig
	mu.Unlock()

	if currentCallCount == 0 {
		t.Error("Expected at least one callback call")
	}

	if currentLastConfig != nil {
		if currentLastConfig["level"] != "debug" {
			t.Errorf("Expected level to be 'debug', got %v", currentLastConfig["level"])
		}
		if currentLastConfig["port"] != float64(9000) { // JSON unmarshals numbers as float64
			t.Errorf("Expected port to be 9000, got %v", currentLastConfig["port"])
		}
	}
}

func TestSimpleFileWatcher(t *testing.T) {
	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "test_simple_*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Remove(tmpfile.Name()); err != nil {
			t.Logf("Failed to remove tmpfile: %v", err)
		}
	}()

	if _, err := tmpfile.WriteString("initial content"); err != nil {
		t.Logf("Failed to write string to tmpfile: %v", err)
	}
	if err := tmpfile.Close(); err != nil {
		t.Logf("Failed to close tmpfile: %v", err)
	}

	// Track callback calls with mutex protection
	var mu sync.Mutex
	callCount := 0
	var lastPath string

	// Create watcher with faster polling for testing
	watcher := New(Config{PollInterval: 50 * time.Millisecond})

	pathCallback := func(path string) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
		lastPath = path
	}

	// Set up the simple file watcher manually
	watchCallback := func(event ChangeEvent) {
		if !event.IsDelete {
			pathCallback(event.Path)
		}
	}

	err = watcher.Watch(tmpfile.Name(), watchCallback)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := watcher.Stop(); err != nil {
			t.Logf("Failed to stop watcher: %v", err)
		}
	}()

	if err := watcher.Start(); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}

	// Give initial time for setup (important in CI)
	time.Sleep(150 * time.Millisecond)

	// Update the file
	if err := os.WriteFile(tmpfile.Name(), []byte("updated content"), 0644); err != nil {
		t.Logf("Failed to write file: %v", err)
	}

	// Wait with retry logic for CI environments
	maxWait := 10 // 10 attempts of 100ms = 1 second max
	for i := 0; i < maxWait; i++ {
		mu.Lock()
		currentCallCount := callCount
		mu.Unlock()

		if currentCallCount > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	mu.Lock()
	finalCallCount := callCount
	finalLastPath := lastPath
	mu.Unlock()

	if finalCallCount == 0 {
		t.Error("Expected at least one callback call")
	}

	if finalLastPath != tmpfile.Name() {
		t.Errorf("Expected path to be %s, got %s", tmpfile.Name(), finalLastPath)
	}
}

// TestUtilitiesFunctions adds test for uncovered utilities functions
func TestUtilitiesFunctions(t *testing.T) {
	t.Run("copyMap_nil_input", func(t *testing.T) {
		// Test copyMap with nil input (should return nil)
		result := copyMap(nil)
		if result != nil {
			t.Errorf("copyMap(nil) should return nil, got %v", result)
		}
	})

	t.Run("copyMap_empty_map", func(t *testing.T) {
		// Test copyMap with empty map
		original := make(map[string]interface{})
		result := copyMap(original)
		if result == nil {
			t.Error("copyMap should not return nil for empty map")
		}
		if len(result) != 0 {
			t.Errorf("Expected empty result map, got %d items", len(result))
		}
	})

	t.Run("copyMap_with_data", func(t *testing.T) {
		// Test copyMap with actual data
		original := map[string]interface{}{
			"string": "value",
			"number": 42,
			"bool":   true,
			"float":  3.14,
		}
		result := copyMap(original)

		if len(result) != len(original) {
			t.Errorf("Expected %d items, got %d", len(original), len(result))
		}

		for key, value := range original {
			if result[key] != value {
				t.Errorf("Key %s: expected %v, got %v", key, value, result[key])
			}
		}

		// Verify it's a copy, not the same map
		result["new_key"] = "new_value"
		if _, exists := original["new_key"]; exists {
			t.Error("Modification to copy affected original map")
		}
	})

	t.Run("SimpleFileWatcher_error_path", func(t *testing.T) {
		// Test SimpleFileWatcher with invalid path to exercise error handling
		watcher, err := SimpleFileWatcher("/this/path/does/not/exist", func(path string) {
			// Should not be called
		})

		// This should still succeed but will fail when Start is called
		if err != nil {
			t.Logf("SimpleFileWatcher with invalid path returned error: %v", err)
		} else if watcher != nil {
			// If watcher was created, it should fail on start
			err := watcher.Start()
			if err == nil {
				// Clean up if it somehow started
				if stopErr := watcher.Stop(); stopErr != nil {
					t.Logf("Failed to stop watcher: %v", stopErr)
				}
			}
		}
	})

	t.Run("UniversalConfigWatcher_unsupported_format", func(t *testing.T) {
		// Test UniversalConfigWatcher with unsupported file format
		tempFile := t.TempDir() + "/test.unsupported"

		// Create file with unsupported extension
		if err := os.WriteFile(tempFile, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		watcher, err := UniversalConfigWatcher(tempFile, func(config map[string]interface{}) {
			// Should not be called
		})

		if err == nil {
			t.Error("Expected error for unsupported format")
			if watcher != nil {
				if stopErr := watcher.Stop(); stopErr != nil {
					t.Logf("Failed to stop watcher: %v", stopErr)
				}
			}
		} else {
			t.Logf("Got expected error for unsupported format: %v", err)
		}
	})

	t.Run("GenericConfigWatcher_compatibility", func(t *testing.T) {
		// Test GenericConfigWatcher (deprecated function) for backward compatibility
		tempDir := t.TempDir()
		tempFile := tempDir + "/test.json"

		// Create valid JSON config
		if err := os.WriteFile(tempFile, []byte(`{"test": "value"}`), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		watcher, err := GenericConfigWatcher(tempFile, func(config map[string]interface{}) {
			// Callback for testing
		})

		if err != nil {
			t.Errorf("GenericConfigWatcher failed: %v", err)
		}

		if watcher != nil {
			defer func() {
				if err := watcher.Stop(); err != nil {
					t.Logf("Failed to stop watcher: %v", err)
				}
			}()
		}
	})
}
