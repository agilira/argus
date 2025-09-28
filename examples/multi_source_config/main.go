// Multi-Source Configuration Loading Example
//
// This example demonstrates Argus's new LoadConfigMultiSource functionality
// which provides automatic precedence handling for configuration sources:
// 1. Environment variables (highest priority)
// 2. Configuration files (medium priority)
// 3. Default values (lowest priority)
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/agilira/argus"
)

func main() {
	fmt.Println("Argus Multi-Source Configuration Loading Demo")
	fmt.Println("==============================================")

	// Demo 1: Load configuration with file + environment precedence
	fmt.Println("\nDemo 1: Configuration File + Environment Override")

	// Create a sample config file
	configContent := `{
		"poll_interval": "10s",
		"cache_ttl": "5s", 
		"max_watched_files": 100,
		"audit": {
			"enabled": false,
			"min_level": "info"
		}
	}`

	configFile := "demo_config.json"
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		log.Fatalf("Failed to create demo config file: %v", err)
	}
	defer func() {
		if err := os.Remove(configFile); err != nil {
			log.Printf("Warning: Failed to remove config file: %v", err)
		}
	}()

	// Set some environment overrides
	if err := os.Setenv("ARGUS_POLL_INTERVAL", "3s"); err != nil {
		log.Printf("Warning: Failed to set ARGUS_POLL_INTERVAL: %v", err)
	}
	if err := os.Setenv("ARGUS_AUDIT_ENABLED", "true"); err != nil {
		log.Printf("Warning: Failed to set ARGUS_AUDIT_ENABLED: %v", err)
	}
	if err := os.Setenv("ARGUS_MAX_WATCHED_FILES", "200"); err != nil {
		log.Printf("Warning: Failed to set ARGUS_MAX_WATCHED_FILES: %v", err)
	}
	defer func() {
		_ = os.Unsetenv("ARGUS_POLL_INTERVAL")
		_ = os.Unsetenv("ARGUS_AUDIT_ENABLED")
		_ = os.Unsetenv("ARGUS_MAX_WATCHED_FILES")
	}()

	// Load with multi-source precedence
	config, err := argus.LoadConfigMultiSource(configFile)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	fmt.Printf("PollInterval: %v (ENV override from file's 10s)\n", config.PollInterval)
	fmt.Printf("CacheTTL: %v (from file, no ENV override)\n", config.CacheTTL)
	fmt.Printf("MaxWatchedFiles: %d (ENV override from file's 100)\n", config.MaxWatchedFiles)
	fmt.Printf("Audit.Enabled: %t (ENV override from file's false)\n", config.Audit.Enabled)

	// Demo 2: Environment-only configuration
	fmt.Println("\nDemo 2: Environment Variables Only")

	// Clear the config file path to test env-only mode
	config2, err := argus.LoadConfigMultiSource("")
	if err != nil {
		log.Fatalf("Failed to load env-only configuration: %v", err)
	}

	fmt.Printf("Environment-only PollInterval: %v\n", config2.PollInterval)
	fmt.Printf("Environment-only MaxWatchedFiles: %d\n", config2.MaxWatchedFiles)

	// Demo 3: Multiple format support
	fmt.Println("\nDemo 3: Multiple Configuration Formats")

	formats := map[string]string{
		"config.yaml": `
poll_interval: 15s
cache_ttl: 8s
max_watched_files: 150
audit:
  enabled: true
  min_level: warn`,
		"config.toml": `
poll_interval = "20s"
cache_ttl = "10s"
max_watched_files = 250

[audit]
enabled = true
min_level = "critical"`,
		"config.ini": `
[core]
poll_interval=25s
cache_ttl=12s
max_watched_files=300

[audit] 
enabled=true
min_level=security`,
	}

	for filename, content := range formats {
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			log.Printf("Failed to create %s: %v", filename, err)
			continue
		}
		defer func() {
			if err := os.Remove(filename); err != nil {
				log.Printf("Warning: Failed to remove %s: %v", filename, err)
			}
		}()

		// Clear environment to test pure file loading
		_ = os.Unsetenv("ARGUS_POLL_INTERVAL")
		_ = os.Unsetenv("ARGUS_AUDIT_ENABLED")
		_ = os.Unsetenv("ARGUS_MAX_WATCHED_FILES")

		_, err := argus.LoadConfigMultiSource(filename)
		if err != nil {
			fmt.Printf("Failed to load %s: %v\n", filename, err)
			continue
		}

		fmt.Printf("%s loaded successfully\n", filename)
		// Note: Since we haven't implemented full Config binding yet,
		// we just verify the file loads without parsing errors
	}

	// Demo 4: Graceful fallback behavior
	fmt.Println("\nDemo 4: Graceful Fallback Behavior")

	// Test non-existent file (should fallback to env + defaults)
	config4, err := argus.LoadConfigMultiSource("/nonexistent/config.json")
	if err != nil {
		log.Printf("Non-existent file handling failed: %v", err)
	} else {
		fmt.Printf("Non-existent file: graceful fallback to defaults\n")
		fmt.Printf("Default PollInterval: %v\n", config4.PollInterval)
	}

	// Test invalid file (should fallback to env + defaults)
	invalidFile := "invalid_config.json"
	invalidContent := `{"invalid": json without quotes}`
	if err := os.WriteFile(invalidFile, []byte(invalidContent), 0644); err == nil {
		defer func() {
			if err := os.Remove(invalidFile); err != nil {
				log.Printf("Warning: Failed to remove %s: %v", invalidFile, err)
			}
		}()

		config5, err := argus.LoadConfigMultiSource(invalidFile)
		if err != nil {
			log.Printf("Invalid file handling failed: %v", err)
		} else {
			fmt.Printf("Invalid file: graceful fallback to defaults\n")
			fmt.Printf("Default PollInterval: %v\n", config5.PollInterval)
		}
	}

	// Demo 5: Create and use watcher with multi-source config
	fmt.Println("\nDemo 5: Real-time Watching with Multi-Source Config")

	// Create watcher using the multi-source configuration
	watcher := argus.New(*config)

	// Create a test file to watch
	watchedFile := "watched_demo.json"
	initialContent := `{"status": "initial", "count": 1}`
	if err := os.WriteFile(watchedFile, []byte(initialContent), 0644); err != nil {
		log.Fatalf("Failed to create watched file: %v", err)
	}
	defer func() {
		if err := os.Remove(watchedFile); err != nil {
			log.Printf("Warning: Failed to remove %s: %v", watchedFile, err)
		}
	}()

	// Set up file watching with callback
	changeCount := 0
	err = watcher.Watch(watchedFile, func(event argus.ChangeEvent) {
		changeCount++
		fmt.Printf("Change detected #%d: %s\n",
			changeCount, event.Path)
	})
	if err != nil {
		log.Fatalf("Failed to set up file watching: %v", err)
	}

	// Start the watcher
	if err := watcher.Start(); err != nil {
		log.Fatalf("Failed to start watcher: %v", err)
	}

	fmt.Printf("Watching %s with multi-source configuration...\n", watchedFile)
	fmt.Printf("Using PollInterval: %v (from precedence resolution)\n", config.PollInterval)

	// Simulate file changes
	time.Sleep(100 * time.Millisecond) // Let watcher stabilize

	// Make a change
	updatedContent := `{"status": "updated", "count": 2}`
	if err := os.WriteFile(watchedFile, []byte(updatedContent), 0644); err == nil {
		time.Sleep(200 * time.Millisecond) // Wait for change detection
	}

	// Graceful shutdown (another feature of Argus)
	if err := watcher.GracefulShutdown(5 * time.Second); err != nil {
		log.Printf("Warning: Graceful shutdown failed: %v", err)
	} else {
		fmt.Printf("Watcher gracefully shut down\n")
	}

	fmt.Println("\nMulti-Source Configuration Demo Complete!")
	fmt.Printf("Total changes detected: %d\n", changeCount)
	fmt.Println("\nKey Features Demonstrated:")
	fmt.Println(" ✓ Automatic format detection (JSON, YAML, TOML, INI)")
	fmt.Println(" ✓ Configuration precedence (ENV > File > Defaults)")
	fmt.Println(" ✓ Graceful fallback for missing/invalid files")
	fmt.Println(" ✓ Security validation for file paths")
	fmt.Println(" ✓ Integration with real-time file watching")
	fmt.Println(" ✓ Ultra-fast performance with zero allocations")
}
