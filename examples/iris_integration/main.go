// iris_integration_demo.go: Example of integrating Argus with iris logger
//
// This demonstrates how to use Argus to watch iris configuration files
// and dynamically update log levels at runtime.
//
// Copyright (c) 2025 AGILira
// Series: AGILira System Libraries
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// Note: In a real application, you would import argus as an external package:
// import "github.com/agilira/argus"
// For this demo, we'll define the interfaces we need

// ---- Mock Argus Interface (replace with real import) ----

type Config struct {
	PollInterval time.Duration
}

type ChangeEvent struct {
	Path     string
	IsDelete bool
}

type UpdateCallback func(event ChangeEvent)

type Watcher interface {
	Watch(filePath string, callback UpdateCallback) error
	Unwatch(filePath string) error
	Start()
	Stop()
}

// ---- End Mock Interface ----

// IrisConfig represents a minimal iris configuration for level watching
type IrisConfig struct {
	Level string `json:"level"`
}

// AtomicLevel interface allows argus to work with any atomic level implementation
// This makes the integration generic and not tied specifically to iris
type AtomicLevel interface {
	SetLevel(level interface{})
}

// LevelParser converts string level names to the appropriate level type
type LevelParser func(levelStr string) interface{}

// MockAtomicLevel simulates an iris atomic level for this demo
type MockAtomicLevel struct {
	currentLevel interface{}
}

func (m *MockAtomicLevel) SetLevel(level interface{}) {
	m.currentLevel = level
	log.Printf("MockAtomicLevel: Level changed to %v", level)
}

// NewIrisLevelWatcher creates a specialized watcher for iris log level changes
//
// This convenience function sets up Argus specifically for watching iris
// configuration files and updating atomic log levels when they change.
func NewIrisLevelWatcher(
	configPath string,
	atomicLevel AtomicLevel,
	levelParser LevelParser,
	watcher Watcher,
) error {
	callback := func(event ChangeEvent) {
		if event.IsDelete {
			log.Printf("Config file %s was deleted", event.Path)
			return
		}

		// Read and parse the updated configuration
		data, err := os.ReadFile(event.Path)
		if err != nil {
			log.Printf("Argus: failed to read config file %s: %v", event.Path, err)
			return
		}

		var irisConfig IrisConfig
		if err := json.Unmarshal(data, &irisConfig); err != nil {
			log.Printf("Argus: failed to parse config file %s: %v", event.Path, err)
			return
		}

		// Parse and update the level
		if irisConfig.Level != "" {
			newLevel := levelParser(irisConfig.Level)
			atomicLevel.SetLevel(newLevel)
			log.Printf("Argus: updated log level to %s", irisConfig.Level)
		}
	}

	return watcher.Watch(configPath, callback)
}

// StandardIrisLevelParser provides a standard parser for iris log levels
func StandardIrisLevelParser(levelStr string) interface{} {
	switch strings.ToLower(levelStr) {
	case "debug":
		return 0 // iris.Debug
	case "info":
		return 1 // iris.Info
	case "warn", "warning":
		return 2 // iris.Warn
	case "error":
		return 3 // iris.Error
	case "panic":
		return 4 // iris.Panic
	case "fatal":
		return 5 // iris.Fatal
	default:
		return 1 // Default to Info
	}
}

// MockWatcher simulates an Argus watcher for this demo
type MockWatcher struct {
	callbacks map[string]UpdateCallback
	running   bool
}

func NewMockWatcher() *MockWatcher {
	return &MockWatcher{
		callbacks: make(map[string]UpdateCallback),
	}
}

func (w *MockWatcher) Watch(filePath string, callback UpdateCallback) error {
	w.callbacks[filePath] = callback
	log.Printf("MockWatcher: Now watching %s", filePath)
	return nil
}

func (w *MockWatcher) Unwatch(filePath string) error {
	delete(w.callbacks, filePath)
	log.Printf("MockWatcher: Stopped watching %s", filePath)
	return nil
}

func (w *MockWatcher) Start() {
	w.running = true
	log.Printf("MockWatcher: Started")
}

func (w *MockWatcher) Stop() {
	w.running = false
	log.Printf("MockWatcher: Stopped")
}

// Simulate a file change for demo purposes
func (w *MockWatcher) SimulateChange(filePath string) {
	if callback, exists := w.callbacks[filePath]; exists {
		callback(ChangeEvent{
			Path:     filePath,
			IsDelete: false,
		})
	}
}

func main() {
	fmt.Println("🔍 Iris + Argus Integration Demo")
	fmt.Println("===================================")

	// Create a mock config file
	configPath := "/tmp/iris_config.json"
	config := IrisConfig{Level: "info"}
	configData, _ := json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, configData, 0644)

	// Create mock components
	atomicLevel := &MockAtomicLevel{}
	watcher := NewMockWatcher()

	// Set up the iris level watcher
	err := NewIrisLevelWatcher(configPath, atomicLevel, StandardIrisLevelParser, watcher)
	if err != nil {
		log.Fatalf("Failed to create iris level watcher: %v", err)
	}

	// Start watching
	watcher.Start()

	// Simulate some configuration changes
	fmt.Println("\n📝 Simulating config changes...")

	// Change to debug level
	config.Level = "debug"
	configData, _ = json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, configData, 0644)
	watcher.SimulateChange(configPath)

	time.Sleep(100 * time.Millisecond)

	// Change to error level
	config.Level = "error"
	configData, _ = json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, configData, 0644)
	watcher.SimulateChange(configPath)

	time.Sleep(100 * time.Millisecond)

	// Change to invalid level (should default to info)
	config.Level = "invalid"
	configData, _ = json.MarshalIndent(config, "", "  ")
	os.WriteFile(configPath, configData, 0644)
	watcher.SimulateChange(configPath)

	// Clean up
	watcher.Stop()
	os.Remove(configPath)

	fmt.Println("\n✅ Demo completed!")
	fmt.Println("\nTo use with real iris logger:")
	fmt.Println("1. Import: github.com/agilira/argus")
	fmt.Println("2. Replace MockWatcher with argus.New()")
	fmt.Println("3. Replace MockAtomicLevel with logger.Level()")
}
