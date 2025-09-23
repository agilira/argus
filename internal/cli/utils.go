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
	"strconv"
	"strings"

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
	// Read file content
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
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
