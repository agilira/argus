// Utility functions for the Argus CLI
//
// This file provides helper functions for format detection, configuration loading,
// value parsing, and template generation with zero-allocation optimizations.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/agilira/argus"
)

// detectFormat detects configuration format from file extension or explicit format.
// Performance: Zero allocations, optimized format detection with fast path.
func (m *Manager) detectFormat(filePath, explicitFormat string) argus.ConfigFormat {
	if explicitFormat != "" && explicitFormat != "auto" {
		return m.parseExplicitFormat(explicitFormat)
	}

	// Auto-detect from file extension (zero allocations)
	return argus.DetectFormat(filePath)
}

// parseExplicitFormat parses an explicitly specified format string.
func (m *Manager) parseExplicitFormat(formatStr string) argus.ConfigFormat {
	switch strings.ToLower(formatStr) {
	case "json":
		return argus.FormatJSON
	case "yaml", "yml":
		return argus.FormatYAML
	case "toml":
		return argus.FormatTOML
	case "hcl":
		return argus.FormatHCL
	case "ini", "conf", "cfg":
		return argus.FormatINI
	case "properties":
		return argus.FormatProperties
	default:
		return argus.FormatUnknown
	}
}

// loadConfig loads and parses a configuration file with the specified format.
// Performance: File I/O bound, zero allocations for parsing with pre-allocated buffers.
func (m *Manager) loadConfig(filePath string, format argus.ConfigFormat) (map[string]interface{}, error) {
	// SECURITY: Validate path to prevent directory traversal attacks
	if err := argus.ValidateSecurePath(filePath); err != nil {
		return nil, fmt.Errorf("security validation failed: %w", err)
	}

	// Read file content
	// #nosec G304 -- Path validation performed above with ValidateSecurePath
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("configuration file does not exist: %s", filePath)
		}
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Parse using fast parsers (zero allocations)
	config, err := argus.ParseConfig(data, format)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", format.String(), err)
	}

	return config, nil
}

// parseValue automatically parses a string value to the appropriate Go type.
// Supports: bool, int, float64, and strings with smart type detection.
func parseValue(value string) interface{} {
	// Try boolean first, but only for explicit boolean strings
	// This avoids ParseBool accepting "0"/"1" which should be integers
	lowerValue := strings.ToLower(value)
	if lowerValue == "true" || lowerValue == "false" {
		return lowerValue == "true"
	}

	// Try integer
	if i, err := strconv.ParseInt(value, 10, 64); err == nil {
		return i
	}

	// Try float
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}

	// Default to string
	return value
}

// generateTemplate generates configuration content based on template type.
// Provides common configuration templates for different use cases.
func (m *Manager) generateTemplate(templateType string) map[string]interface{} {
	switch templateType {
	case "server":
		return map[string]interface{}{
			"server": map[string]interface{}{
				"host":    "0.0.0.0",
				"port":    8080,
				"timeout": "30s",
			},
			"logging": map[string]interface{}{
				"level":  "info",
				"format": "json",
			},
			"metrics": map[string]interface{}{
				"enabled": true,
				"port":    9090,
			},
		}
	case "database":
		return map[string]interface{}{
			"database": map[string]interface{}{
				"host":      "localhost",
				"port":      5432,
				"name":      "myapp",
				"user":      "admin",
				"password":  "changeme",
				"pool_size": 10,
			},
			"cache": map[string]interface{}{
				"enabled":  true,
				"ttl":      "5m",
				"max_size": 1000,
			},
		}
	case "minimal":
		return map[string]interface{}{
			"app_name": "my-application",
			"version":  "1.0.0",
			"debug":    false,
		}
	default: // "default"
		return map[string]interface{}{
			"app": map[string]interface{}{
				"name":        "argus-app",
				"version":     "1.0.0",
				"environment": "development",
			},
			"server": map[string]interface{}{
				"host": "localhost",
				"port": 8080,
			},
			"features": map[string]interface{}{
				"auth_enabled":    true,
				"metrics_enabled": false,
				"debug_mode":      true,
			},
		}
	}
}

// parseExtendedDuration parses duration strings with extended units (d, w).
// Supports all Go standard units (ns, us, ms, s, m, h) plus:
// - d: days (24 hours)
// - w: weeks (7 days)
//
// Examples: "30d", "2w", "7d", "24h", "5m", "30s"
func parseExtendedDuration(s string) (time.Duration, error) {
	// First try standard Go parsing
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}

	// Handle extended units
	re := regexp.MustCompile(`^(\d+)(d|w)$`)
	matches := re.FindStringSubmatch(s)

	if len(matches) != 3 {
		// If it doesn't match our extended pattern, return original error
		_, err := time.ParseDuration(s)
		return 0, err
	}

	value, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration value: %s", matches[1])
	}

	unit := matches[2]
	switch unit {
	case "d":
		return time.Duration(value) * 24 * time.Hour, nil
	case "w":
		return time.Duration(value) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown unit: %s", unit)
	}
}

// checkFileWriteable verifies if a file can be written to.
// Returns error if file exists but is not writable (e.g., read-only permissions).
func checkFileWriteable(filePath string) error {
	// Check if file exists
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		// File doesn't exist - check if directory is writable
		return checkDirectoryWriteable(filepath.Dir(filePath))
	}
	if err != nil {
		return fmt.Errorf("cannot stat file: %w", err)
	}

	// File exists - check if it's writable
	mode := info.Mode()
	if mode&0200 == 0 {
		return fmt.Errorf("file is read-only (mode: %v)", mode)
	}

	return nil
}

// checkDirectoryWriteable verifies if a directory can be written to.
func checkDirectoryWriteable(dirPath string) error {
	info, err := os.Stat(dirPath)
	if err != nil {
		return fmt.Errorf("cannot access directory %s: %w", dirPath, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dirPath)
	}

	// Check directory write permissions
	mode := info.Mode()
	if mode&0200 == 0 {
		return fmt.Errorf("directory is not writable (mode: %v)", mode)
	}

	return nil
}
