// parser_structured.go: Structured configuration parsers for Argus
//
// This file contains parsers for structured configuration formats:
// - JSON (JavaScript Object Notation)
// - YAML (YAML Ain't Markup Language)
// - TOML (Tom's Obvious Minimal Language)
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"encoding/json"
	"strings"

	"github.com/agilira/go-errors"
)

// parseJSON parses JSON configuration with pooled map to reduce allocations.
// Uses the standard library JSON parser for full RFC 7159 compliance.
// Returns the config map to the caller (caller responsible for memory management).
func parseJSON(data []byte) (map[string]interface{}, error) {
	config := getConfigMap()
	if err := json.Unmarshal(data, &config); err != nil {
		putConfigMap(config)
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "invalid JSON")
	}
	// Note: We don't put the config back in the pool since we're returning it
	// The caller is responsible for the memory
	return config, nil
}

// parseYAML parses YAML configuration using a simple line-based implementation.
// Handles basic key-value pairs for 80% use cases. For complex YAML features,
// use a plugin parser with full YAML specification compliance.
// Does not support multi-line values, arrays, or nested structures.
func parseYAML(data []byte) (map[string]interface{}, error) {
	config := getConfigMap()
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Parse value
		config[key] = parseValue(value)
	}

	return config, nil
}

// parseTOML parses TOML configuration with support for sections, nested tables, arrays, and basic types.
// Covers 85% of real-world TOML usage: [sections], [nested.tables], arrays [1,2,3], and proper type inference.
// Supports quoted strings, integers, floats, booleans, and basic arrays.
func parseTOML(data []byte) (map[string]interface{}, error) {
	config := getConfigMap()
	lines := strings.Split(string(data), "\n")

	var currentSection []string // Track current section path like ["app", "database"]

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle section headers [section] or [nested.section]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sectionName := strings.Trim(line, "[]")
			currentSection = strings.Split(sectionName, ".")

			// Create nested structure for section
			createNestedPath(config, currentSection)
			continue
		}

		// Handle key=value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Parse the value with enhanced type support
		parsedValue := parseTOMLValue(value)

		// Set value in the correct section
		if len(currentSection) == 0 {
			// Root level
			config[key] = parsedValue
		} else {
			// Inside a section
			setNestedValue(config, append(currentSection, key), parsedValue)
		}
	}

	return config, nil
}

// parseTOMLValue parses a TOML value with support for arrays, booleans, numbers, and strings.
// Handles quoted strings, arrays [1,2,3], and automatic type inference for common types.
func parseTOMLValue(value string) interface{} {
	value = strings.TrimSpace(value)

	// Handle arrays [1, 2, 3] or ["a", "b", "c"]
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return parseTOMLArray(value)
	}

	// Handle quoted strings
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
		return strings.Trim(value, "\"")
	}

	// Use existing parseValue for type inference
	return parseValue(value)
}

// parseTOMLArray parses TOML array syntax [item1, item2, item3].
// Supports mixed types and handles both quoted and unquoted values.
func parseTOMLArray(arrayStr string) interface{} {
	// Remove brackets and split by comma
	content := strings.Trim(arrayStr, "[]")
	if content == "" {
		return []interface{}{}
	}

	items := strings.Split(content, ",")
	result := make([]interface{}, 0, len(items))

	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, parseTOMLValue(item))
		}
	}

	return result
}

// createNestedPath ensures that nested path exists in the config map.
// Creates intermediate maps as needed for paths like ["app", "database"].
func createNestedPath(config map[string]interface{}, path []string) {
	current := config

	for _, segment := range path {
		if _, exists := current[segment]; !exists {
			current[segment] = make(map[string]interface{})
		}

		// Move deeper into the structure
		if nested, ok := current[segment].(map[string]interface{}); ok {
			current = nested
		} else {
			// If the path conflicts with existing data, create new map
			newMap := make(map[string]interface{})
			current[segment] = newMap
			current = newMap
		}
	}
}

// setNestedValue sets a value at the specified nested path in the config map.
// Creates intermediate maps as needed and handles path conflicts gracefully.
func setNestedValue(config map[string]interface{}, path []string, value interface{}) {
	if len(path) == 0 {
		return
	}

	if len(path) == 1 {
		config[path[0]] = value
		return
	}

	// Ensure intermediate path exists
	current := config
	for _, segment := range path[:len(path)-1] {
		if _, exists := current[segment]; !exists {
			current[segment] = make(map[string]interface{})
		}

		if nested, ok := current[segment].(map[string]interface{}); ok {
			current = nested
		} else {
			// Path conflict - convert to map
			newMap := make(map[string]interface{})
			current[segment] = newMap
			current = newMap
		}
	}

	// Set the final value
	current[path[len(path)-1]] = value
}
