// utilities_test.go: Testing Argus Utilities
//
// Copyright (c) 2025 AGILira
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
	defer os.Remove(tmpfile.Name())

	// Initial config
	config := map[string]interface{}{
		"level": "info",
		"port":  8080,
	}
	data, _ := json.Marshal(config)
	tmpfile.Write(data)
	tmpfile.Close()

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
	defer watcher.Stop()

	watcher.Start()

	// Update the config file
	config["level"] = "debug"
	config["port"] = 9000
	data, _ = json.Marshal(config)
	os.WriteFile(tmpfile.Name(), data, 0644)

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
	defer os.Remove(tmpfile.Name())

	tmpfile.WriteString("initial content")
	tmpfile.Close()

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
	defer watcher.Stop()

	watcher.Start()

	// Give initial time for setup (important in CI)
	time.Sleep(150 * time.Millisecond)

	// Update the file
	os.WriteFile(tmpfile.Name(), []byte("updated content"), 0644)

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
