// utilities.go: Testing Argus Utilities
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"log"
	"os"
)

// copyMap creates a deep copy of a map for audit trail
func copyMap(original map[string]interface{}) map[string]interface{} {
	if original == nil {
		return nil
	}
	result := make(map[string]interface{})
	for k, v := range original {
		result[k] = v
	}
	return result
}

// UniversalConfigWatcher creates a watcher for ANY configuration format
// Supports JSON, YAML, TOML, HCL, INI, XML, Properties
//
// Example:
//
//	watcher, err := argus.UniversalConfigWatcher("config.yml", func(config map[string]interface{}) {
//	    if level, ok := config["level"].(string); ok {
//	        // Handle level change
//	    }
//	    if port, ok := config["port"].(int); ok {
//	        // Handle port change
//	    }
//	})
func UniversalConfigWatcher(configPath string, callback func(config map[string]interface{})) (*Watcher, error) {
	return UniversalConfigWatcherWithConfig(configPath, callback, Config{})
}

// UniversalConfigWatcherWithConfig creates a watcher for ANY configuration format with custom config
func UniversalConfigWatcherWithConfig(configPath string, callback func(config map[string]interface{}), config Config) (*Watcher, error) {
	// Detect format from file extension
	format := DetectFormat(configPath)
	if format == FormatUnknown {
		return nil, fmt.Errorf("unsupported config format for file: %s", configPath)
	}

	// Set default error handler if none provided
	if config.ErrorHandler == nil {
		config.ErrorHandler = func(err error, path string) {
			log.Printf("Argus: error in file %s: %v", path, err)
		}
	}

	watcher := New(config)

	// Track current config for audit trail
	var currentConfig map[string]interface{}

	watchCallback := func(event ChangeEvent) {
		if event.IsDelete {
			// AUDIT: Log file deletion
			if auditor := watcher.auditLogger; auditor != nil {
				auditor.LogFileWatch("config_deleted", event.Path)
			}
			return
		}

		data, err := os.ReadFile(event.Path)
		if err != nil {
			if watcher.config.ErrorHandler != nil {
				watcher.config.ErrorHandler(fmt.Errorf("failed to read config file: %w", err), event.Path)
			}
			return
		}

		newConfig, err := ParseConfig(data, format)
		if err != nil {
			if watcher.config.ErrorHandler != nil {
				watcher.config.ErrorHandler(fmt.Errorf("failed to parse %s config: %w", format, err), event.Path)
			}
			return
		}

		// AUDIT: Log configuration change with before/after values
		if auditor := watcher.auditLogger; auditor != nil {
			auditor.LogConfigChange(event.Path, currentConfig, newConfig)
		}

		// Update current config for next comparison
		currentConfig = copyMap(newConfig)

		callback(newConfig)
	}

	if err := watcher.Watch(configPath, watchCallback); err != nil {
		return nil, fmt.Errorf("failed to watch config file: %w", err)
	}

	// Load initial configuration and start watcher
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath) // #nosec G304 -- configPath is user-provided intentionally
		if err != nil {
			return nil, fmt.Errorf("failed to read initial config file: %w", err)
		}

		initialConfig, err := ParseConfig(data, format)
		if err != nil {
			return nil, fmt.Errorf("failed to parse initial %s config: %w", format, err)
		}

		// Set current config for audit trail
		currentConfig = copyMap(initialConfig)

		// Auto-start the watcher (convenience feature)
		if err := watcher.Start(); err != nil {
			return nil, fmt.Errorf("failed to start watcher: %w", err)
		}

		// Call callback with initial config
		callback(initialConfig)
	} else {
		// File doesn't exist yet, start watcher anyway
		if err := watcher.Start(); err != nil {
			return nil, fmt.Errorf("failed to start watcher: %w", err)
		}
	}

	return watcher, nil
}

// GenericConfigWatcher creates a watcher for JSON configuration (backward compatibility)
// DEPRECATED: Use UniversalConfigWatcher for better format support
func GenericConfigWatcher(configPath string, callback func(config map[string]interface{})) (*Watcher, error) {
	return UniversalConfigWatcher(configPath, callback)
}

// SimpleFileWatcher creates a basic file watcher with minimal configuration
// Useful for simple use cases where you just want to know when a file changes
func SimpleFileWatcher(filePath string, callback func(path string)) (*Watcher, error) {
	watcher := New(Config{})

	watchCallback := func(event ChangeEvent) {
		if !event.IsDelete {
			callback(event.Path)
		}
	}

	if err := watcher.Watch(filePath, watchCallback); err != nil {
		return nil, fmt.Errorf("failed to watch file: %w", err)
	}

	return watcher, nil
}
