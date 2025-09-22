// parser_text.go: Text-based configuration parsers for Argus
//
// This file contains parsers for text-based configuration formats:
// - HCL (HashiCorp Configuration Language)
// - INI files (with sections)
// - Java Properties files
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"bufio"
	"strings"
)

// parseHCL parses HCL (HashiCorp Configuration Language) files using a simple implementation.
// Supports basic key-value pairs and quoted strings. For advanced HCL features
// like blocks, functions, or complex expressions, use the official HCL parser plugin.
// Handles both # and // comment styles commonly used in HCL files.
func parseHCL(data []byte) (map[string]interface{}, error) {
	config := make(map[string]interface{})
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
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

// parseINI parses INI configuration files with section support.
// Handles traditional INI format with [section] headers and key=value pairs.
// Section names are prefixed to keys with dot notation (e.g., "database.host").
// Supports both ; and # comment styles. Empty sections are handled gracefully.
func parseINI(data []byte) (map[string]interface{}, error) {
	config := make(map[string]interface{})
	lines := strings.Split(string(data), "\n")
	currentSection := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}

		// Check for section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.Trim(line, "[]") + "."
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Prefix with section if we have one
		if currentSection != "" {
			key = currentSection + key
		}

		config[key] = parseValue(value)
	}

	return config, nil
}

// parseProperties parses Java-style properties files with line-based processing.
// Supports key=value format with # and ! comment styles (Java standard).
// Uses bufio.Scanner for efficient line processing of large property files.
// Handles whitespace trimming and empty line skipping automatically.
func parseProperties(data []byte) (map[string]interface{}, error) {
	config := make(map[string]interface{})
	scanner := bufio.NewScanner(strings.NewReader(string(data)))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		config[key] = parseValue(value)
	}

	return config, nil
}
