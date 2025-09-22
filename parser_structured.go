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

// parseTOML parses TOML configuration using a simple line-based implementation.
// Handles basic key-value pairs for standard use cases. For advanced TOML features
// like tables, arrays, or datetime handling, use a plugin parser.
// Supports quoted string values and basic type inference.
func parseTOML(data []byte) (map[string]interface{}, error) {
	config := getConfigMap()
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes from strings
		if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") {
			value = strings.Trim(value, "\"")
		}

		config[key] = parseValue(value)
	}

	return config, nil
}
