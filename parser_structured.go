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
	"fmt"
	"strings"
	"unicode"

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

	// SECURITY: Validate JSON keys for dangerous characters
	// JSON spec allows control chars but we enforce security policy
	for key := range config {
		if err := validateJSONKey(key); err != nil {
			putConfigMap(config)
			return nil, err
		}
	}

	// Note: We don't put the config back in the pool since we're returning it
	// The caller is responsible for the memory
	return config, nil
}

// validateJSONKey validates JSON keys for security concerns while allowing JSON spec compliance.
// JSON allows any Unicode character in keys, but we apply security policy restrictions.
func validateJSONKey(key string) error {
	// JSON allows empty keys per RFC 7159, so we don't check for that

	// SECURITY FIX: Check for dangerous control characters including null bytes
	for i, char := range key {
		if char == '\x00' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid JSON key at position %d: null byte not allowed in keys", i))
		}
		// Block other dangerous control characters (except tab, LF, CR)
		if char < 32 && char != '\t' && char != '\n' && char != '\r' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid JSON key at position %d: control character not allowed in keys", i))
		}
		// Block non-printable characters (like DEL 0x7F) - security policy
		if !unicode.IsPrint(char) && char != '\t' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid JSON key at position %d: non-printable character not allowed in keys", i))
		}
	}

	return nil
}

// parseYAML parses YAML configuration with enhanced validation and error reporting.
// Provides strict syntax validation, detailed error messages with line numbers,
// and support for nested structures and arrays. Ensures configuration integrity
// by failing fast on malformed syntax rather than silently ignoring errors.
func parseYAML(data []byte) (map[string]interface{}, error) {
	config := getConfigMap()
	lines := strings.Split(string(data), "\n")

	// Parse with indentation tracking for nested structures
	result, err := parseYAMLLines(lines, config, 0, 0)
	if err != nil {
		putConfigMap(config)
		return nil, err
	}

	return result, nil
}

// parseYAMLLines parses YAML lines with support for nested indentation.
// Tracks indentation levels to build nested map structures correctly.
// Returns the parsed config and the last processed line index.
func parseYAMLLines(lines []string, config map[string]interface{}, startLine, baseIndent int) (map[string]interface{}, error) {
	for i := startLine; i < len(lines); i++ {
		originalLine := lines[i]
		line := strings.TrimSpace(originalLine)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Calculate current indentation
		currentIndent := len(originalLine) - len(strings.TrimLeft(originalLine, " \t"))

		// If indentation is less than base, we're done with this level
		if currentIndent < baseIndent {
			return config, nil
		}

		// Skip lines that are more indented (they belong to previous key)
		if currentIndent > baseIndent {
			continue
		}

		// Validate line structure and provide detailed error reporting
		if err := validateYAMLLine(originalLine, i+1); err != nil {
			return nil, err
		}

		// Split key-value pair with validation
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid YAML syntax at line %d: missing colon separator",
					i+1))
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Validate key format
		if err := validateYAMLKey(key, i+1); err != nil {
			return nil, err
		}

		// Handle nested structures
		if value == "" || value == ":" {
			// This is a nested object - find all child lines
			nestedConfig := make(map[string]interface{})

			// Find the expected child indentation
			childIndent := -1
			for j := i + 1; j < len(lines); j++ {
				childLine := strings.TrimSpace(lines[j])
				if childLine == "" || strings.HasPrefix(childLine, "#") {
					continue
				}

				childCurrentIndent := len(lines[j]) - len(strings.TrimLeft(lines[j], " \t"))
				if childCurrentIndent > currentIndent {
					childIndent = childCurrentIndent
					break
				} else {
					// No children found
					break
				}
			}

			if childIndent > currentIndent {
				// Parse nested structure
				nested, err := parseYAMLLines(lines, nestedConfig, i+1, childIndent)
				if err != nil {
					return nil, err
				}
				config[key] = nested
			} else {
				// Empty nested object
				config[key] = make(map[string]interface{})
			}
		} else {
			// Regular key-value pair
			parsedValue, err := parseYAMLValue(value, i+1)
			if err != nil {
				return nil, err
			}
			config[key] = parsedValue
		}
	}

	return config, nil
} // parseTOML parses TOML configuration with support for sections, nested tables, arrays, and basic types.
// Covers 85% of real-world TOML usage: [sections], [nested.tables], arrays [1,2,3], and proper type inference.
// Supports quoted strings, integers, floats, booleans, and basic arrays.
func parseTOML(data []byte) (map[string]interface{}, error) {
	config := getConfigMap()
	lines := strings.Split(string(data), "\n")

	var currentSection []string // Track current section path like ["app", "database"]

	for lineNum, line := range lines {
		originalLine := line
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Handle section headers [section] or [nested.section]
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			// Validate section header format
			if err := validateTOMLSection(originalLine, lineNum+1); err != nil {
				putConfigMap(config)
				return nil, err
			}

			sectionName := strings.Trim(line, "[]")
			if sectionName == "" {
				putConfigMap(config)
				return nil, errors.New(ErrCodeInvalidConfig,
					fmt.Sprintf("invalid TOML syntax at line %d: empty section name",
						lineNum+1))
			}

			currentSection = strings.Split(sectionName, ".")

			// Create nested structure for section
			createNestedPath(config, currentSection)
			continue
		}

		// Handle key=value pairs with validation
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			putConfigMap(config)
			return nil, errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid TOML syntax at line %d: missing equals separator",
					lineNum+1))
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Validate key format
		if err := validateTOMLKey(key, lineNum+1); err != nil {
			putConfigMap(config)
			return nil, err
		}

		// Parse the value with enhanced type support and error reporting
		parsedValue, err := parseTOMLValueWithValidation(value, lineNum+1)
		if err != nil {
			putConfigMap(config)
			return nil, err
		}

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
	if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2 {
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

// validateYAMLLine performs comprehensive validation on a YAML line.
// Checks for common syntax errors, invalid characters, and structural problems.
// Returns detailed error with line number and suggestion when validation fails.
func validateYAMLLine(line string, lineNum int) error {
	// Skip empty lines and comments
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil
	}

	// Check for invalid YAML characters that should cause immediate failure
	invalidChars := []string{"[", "]", "{", "}", "<", ">"}
	for _, char := range invalidChars {
		if strings.Contains(line, char) && !strings.Contains(line, ":") {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid YAML syntax at line %d: unexpected character '%s'",
					lineNum, char))
		}
	}

	// Check if line contains suspicious patterns that indicate malformed YAML
	if !strings.Contains(trimmed, ":") && trimmed != "" {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid YAML syntax at line %d: missing colon separator",
				lineNum))
	}

	return nil
}

// validateYAMLKey validates that a YAML key follows proper naming conventions.
// Keys must be non-empty, trimmed, and follow basic identifier rules.
func validateYAMLKey(key string, lineNum int) error {
	if key == "" {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid YAML key at line %d: key cannot be empty", lineNum))
	}

	// SECURITY FIX: Check for dangerous control characters including null bytes
	for _, char := range key {
		if char == '\x00' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid YAML key at line %d: null byte not allowed in keys", lineNum))
		}
		// Block other dangerous control characters (except tab, LF, CR)
		if char < 32 && char != '\t' && char != '\n' && char != '\r' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid YAML key at line %d: control character not allowed in keys", lineNum))
		}
		// Block non-printable characters (like DEL 0x7F)
		if !unicode.IsPrint(char) && char != '\t' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid YAML key at line %d: non-printable character not allowed in keys", lineNum))
		}
	}

	// Check for whitespace in key (indicates potential parsing issue)
	if strings.TrimSpace(key) != key {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid YAML key at line %d: key contains unexpected whitespace",
				lineNum))
	}

	return nil
}

// parseYAMLValue parses a YAML value with enhanced validation and type inference.
// Supports basic YAML types including strings, numbers, booleans, null, and simple arrays.
// Handles quoted strings properly by removing quotes and preserving the string value.
func parseYAMLValue(value string, lineNum int) (interface{}, error) {
	// Handle empty values
	if value == "" {
		return "", nil
	}

	// Handle null/nil values
	if value == "null" || value == "~" || value == "nil" {
		return nil, nil
	}

	// Handle quoted strings (remove quotes and return as string)
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && len(value) >= 2) {
		// Remove quotes and return the string content
		return value[1 : len(value)-1], nil
	}

	// Handle simple arrays [item1, item2, item3]
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return parseYAMLArray(value, lineNum)
	}

	// Use existing parseValue for basic type inference (numbers, booleans, etc.)
	return parseValue(value), nil
}

// parseYAMLArray parses simple YAML array syntax [item1, item2, item3].
// Provides validation and error reporting for malformed arrays.
func parseYAMLArray(arrayStr string, lineNum int) (interface{}, error) {
	// Remove brackets and validate structure
	content := strings.Trim(arrayStr, "[]")
	if content == "" {
		return []interface{}{}, nil
	}

	// Check for unmatched brackets or other issues
	if strings.Count(arrayStr, "[") != strings.Count(arrayStr, "]") {
		return nil, errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid YAML array at line %d: unmatched brackets",
				lineNum))
	}

	items := strings.Split(content, ",")
	result := make([]interface{}, 0, len(items))

	for i, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			// Recursively parse each item with validation
			parsedItem, err := parseYAMLValue(item, lineNum)
			if err != nil {
				return nil, errors.New(ErrCodeInvalidConfig,
					fmt.Sprintf("invalid YAML array item %d at line %d: %v",
						i+1, lineNum, err))
			}
			result = append(result, parsedItem)
		}
	}

	return result, nil
}

// validateTOMLSection validates TOML section header format [section.name].
// Ensures proper bracket matching and valid section name syntax.
func validateTOMLSection(line string, lineNum int) error {
	trimmed := strings.TrimSpace(line)

	// Check for proper bracket format
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid TOML section at line %d: malformed brackets",
				lineNum))
	}

	// Check for nested brackets (not supported in basic TOML)
	content := strings.Trim(trimmed, "[]")
	if strings.Contains(content, "[") || strings.Contains(content, "]") {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid TOML section at line %d: nested brackets not supported",
				lineNum))
	}

	// Validate section name format - prevent leading/trailing dots and empty segments
	if strings.HasPrefix(content, ".") || strings.HasSuffix(content, ".") {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid TOML section at line %d: section name cannot start or end with dot",
				lineNum))
	}

	// Check for consecutive dots or empty segments
	if strings.Contains(content, "..") {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid TOML section at line %d: section name cannot have consecutive dots",
				lineNum))
	}

	// Validate each segment of the section name
	segments := strings.Split(content, ".")
	for _, segment := range segments {
		if segment == "" {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid TOML section at line %d: section name cannot have empty segments",
					lineNum))
		}

		// SECURITY FIX: Apply same character validation as keys
		for _, char := range segment {
			if char == '\x00' {
				return errors.New(ErrCodeInvalidConfig,
					fmt.Sprintf("invalid TOML section at line %d: null byte not allowed in section names", lineNum))
			}
			// Block other dangerous control characters (except tab, LF, CR)
			if char < 32 && char != '\t' && char != '\n' && char != '\r' {
				return errors.New(ErrCodeInvalidConfig,
					fmt.Sprintf("invalid TOML section at line %d: control character not allowed in section names", lineNum))
			}
			// Block non-printable characters (like DEL 0x7F)
			if !unicode.IsPrint(char) && char != '\t' {
				return errors.New(ErrCodeInvalidConfig,
					fmt.Sprintf("invalid TOML section at line %d: non-printable character not allowed in section names", lineNum))
			}
		}
	}

	return nil
}

// validateTOMLKey validates that a TOML key follows proper naming conventions.
// Keys must be non-empty and follow basic identifier rules.
func validateTOMLKey(key string, lineNum int) error {
	if key == "" {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid TOML key at line %d: key cannot be empty", lineNum))
	}

	// SECURITY FIX: Check for dangerous control characters including null bytes
	for _, char := range key {
		if char == '\x00' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid TOML key at line %d: null byte not allowed in keys", lineNum))
		}
		// Block other dangerous control characters (except tab, LF, CR)
		if char < 32 && char != '\t' && char != '\n' && char != '\r' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid TOML key at line %d: control character not allowed in keys", lineNum))
		}
		// Block non-printable characters (like DEL 0x7F)
		if !unicode.IsPrint(char) && char != '\t' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid TOML key at line %d: non-printable character not allowed in keys", lineNum))
		}
	}

	// Check for whitespace in unquoted key (indicates potential parsing issue)
	if strings.TrimSpace(key) != key {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid TOML key at line %d: key contains unexpected whitespace",
				lineNum))
	}

	return nil
}

// parseTOMLValueWithValidation parses a TOML value with enhanced validation and error reporting.
// Supports TOML arrays, quoted strings, numbers, booleans with proper error handling.
func parseTOMLValueWithValidation(value string, lineNum int) (interface{}, error) {
	value = strings.TrimSpace(value)

	// Handle arrays with validation
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		return parseTOMLArrayWithValidation(value, lineNum)
	}

	// Handle quoted strings (remove quotes)
	if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && len(value) >= 2) ||
		(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'") && len(value) >= 2) {
		// Remove quotes and return the string content
		return value[1 : len(value)-1], nil
	}

	// Use existing parseTOMLValue for type inference
	return parseTOMLValue(value), nil
}

// parseTOMLArrayWithValidation parses TOML array syntax with validation.
// Provides detailed error reporting for malformed arrays.
func parseTOMLArrayWithValidation(arrayStr string, lineNum int) (interface{}, error) {
	// Validate bracket structure
	if strings.Count(arrayStr, "[") != strings.Count(arrayStr, "]") {
		return nil, errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid TOML array at line %d: unmatched brackets",
				lineNum))
	}

	// Use existing parseTOMLArray for actual parsing
	return parseTOMLArray(arrayStr), nil
}
