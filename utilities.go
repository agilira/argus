// utilities.go: Testing Argus Utilities
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"log"
	"os"

	"github.com/agilira/go-errors"
)

// copyMap creates a deep copy of a map for audit trail purposes.
// Used to preserve configuration state for before/after comparisons in audit logs.
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

// UniversalConfigWatcherWithConfig creates a watcher for ANY configuration format with custom config.
// Provides fine-grained control over watcher behavior while maintaining universal format support.
//
// Parameters:
//   - configPath: Path to configuration file (format auto-detected from extension)
//   - callback: Function called when configuration changes
//   - config: Custom Argus configuration for performance tuning
//
// Returns:
//   - *Watcher: Configured and started watcher
//   - error: Any initialization or file access errors
func UniversalConfigWatcherWithConfig(configPath string, callback func(config map[string]interface{}), config Config) (*Watcher, error) {
	// Detect format from file extension
	format := DetectFormat(configPath)
	if format == FormatUnknown {
		return nil, errors.New(ErrCodeConfigNotFound, "unsupported config format for file: "+configPath)
	}

	// Configure watcher
	watcher := setupUniversalWatcher(config)

	// Track current config for audit trail
	var currentConfig map[string]interface{}

	// Create watch callback
	watchCallback := createUniversalWatchCallback(format, callback, watcher, &currentConfig)

	// Setup file watching
	if err := watcher.Watch(configPath, watchCallback); err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "failed to watch config file")
	}

	// Initialize and start watcher
	if err := initializeUniversalWatcher(watcher, configPath, format, callback, &currentConfig); err != nil {
		return nil, err
	}

	return watcher, nil
}

// setupUniversalWatcher configures a new watcher with defaults
func setupUniversalWatcher(config Config) *Watcher {
	// Set default error handler if none provided
	if config.ErrorHandler == nil {
		config.ErrorHandler = func(err error, path string) {
			log.Printf("Argus: error in file %s: %v", path, err)
		}
	}
	return New(config)
}

// createUniversalWatchCallback creates the file change callback
func createUniversalWatchCallback(format ConfigFormat, callback func(config map[string]interface{}), watcher *Watcher, currentConfig *map[string]interface{}) func(ChangeEvent) {
	return func(event ChangeEvent) {
		if event.IsDelete {
			// AUDIT: Log file deletion
			if auditor := watcher.auditLogger; auditor != nil {
				auditor.LogFileWatch("config_deleted", event.Path)
			}
			return
		}

		newConfig, err := readAndParseConfig(event.Path, format)
		if err != nil {
			if watcher.config.ErrorHandler != nil {
				watcher.config.ErrorHandler(err, event.Path)
			}
			return
		}

		// AUDIT: Log configuration change with before/after values
		if auditor := watcher.auditLogger; auditor != nil {
			auditor.LogConfigChange(event.Path, *currentConfig, newConfig)
		}

		// Update current config for next comparison
		*currentConfig = copyMap(newConfig)

		callback(newConfig)
	}
}

// readAndParseConfig reads and parses a config file
func readAndParseConfig(path string, format ConfigFormat) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeFileNotFound, "failed to read config file")
	}

	newConfig, err := ParseConfig(data, format)
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "failed to parse "+format.String()+" config")
	}

	return newConfig, nil
}

// initializeUniversalWatcher loads initial config and starts watching
func initializeUniversalWatcher(watcher *Watcher, configPath string, format ConfigFormat, callback func(config map[string]interface{}), currentConfig *map[string]interface{}) error {
	// Load initial configuration and start watcher
	if _, err := os.Stat(configPath); err == nil {
		initialConfig, err := readAndParseConfig(configPath, format) // #nosec G304 -- configPath is user-provided intentionally
		if err != nil {
			return errors.Wrap(err, ErrCodeInvalidConfig, "failed to read initial config")
		}

		// Set current config for audit trail
		*currentConfig = copyMap(initialConfig)

		// Auto-start the watcher (convenience feature)
		if err := watcher.Start(); err != nil {
			return errors.Wrap(err, ErrCodeWatcherBusy, "failed to start watcher")
		}

		// Call callback with initial config
		callback(initialConfig)
	} else {
		// File doesn't exist yet, start watcher anyway
		if err := watcher.Start(); err != nil {
			return errors.Wrap(err, ErrCodeWatcherBusy, "failed to start watcher")
		}
	}

	return nil
}

// GenericConfigWatcher creates a watcher for JSON configuration (backward compatibility).
// DEPRECATED: Use UniversalConfigWatcher for better format support and future-proofing.
// This function is maintained for existing codebases but new code should use UniversalConfigWatcher.
func GenericConfigWatcher(configPath string, callback func(config map[string]interface{})) (*Watcher, error) {
	return UniversalConfigWatcher(configPath, callback)
}

// SimpleFileWatcher creates a basic file watcher with minimal configuration.
// Useful for simple use cases where you just want to know when a file changes,
// without the complexity of configuration parsing or format detection.
//
// Parameters:
//   - filePath: Path to file to watch
//   - callback: Function called with file path when changes occur
//
// Returns:
//   - *Watcher: Configured watcher (not automatically started)
//   - error: Any initialization errors
func SimpleFileWatcher(filePath string, callback func(path string)) (*Watcher, error) {
	watcher := New(Config{})

	watchCallback := func(event ChangeEvent) {
		if !event.IsDelete {
			callback(event.Path)
		}
	}

	if err := watcher.Watch(filePath, watchCallback); err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "failed to watch file")
	}

	return watcher, nil
}
