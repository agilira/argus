// plugin_system_test.go: Tests for the parser plugin system
//
// Copyright (c) 2025 AGILira
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
)

// testAdvancedYAMLParser is a simple test parser for testing the plugin system
type testAdvancedYAMLParser struct{}

func (p *testAdvancedYAMLParser) Parse(data []byte) (map[string]interface{}, error) {
	return map[string]interface{}{
		"_parser": "test-yaml-plugin",
		"_note":   "This is a test parser",
	}, nil
}

func (p *testAdvancedYAMLParser) Supports(format ConfigFormat) bool {
	return format == FormatYAML
}

func (p *testAdvancedYAMLParser) Name() string {
	return "Test YAML Parser"
}

// Helper function for tests
func registerTestYAMLParser() {
	RegisterParser(&testAdvancedYAMLParser{})
}

// Helper function for tests
func listRegisteredParsers() []string {
	parserMutex.RLock()
	defer parserMutex.RUnlock()

	var names []string
	for _, parser := range customParsers {
		names = append(names, parser.Name())
	}
	return names
}

func TestParserPluginSystem(t *testing.T) {
	// Save original state and ensure complete cleanup
	parserMutex.Lock()
	originalParsers := make([]ConfigParser, len(customParsers))
	copy(originalParsers, customParsers)
	customParsers = nil // Clear for clean test
	parserMutex.Unlock()

	// Restore original state when test completes
	defer func() {
		parserMutex.Lock()
		customParsers = originalParsers
		parserMutex.Unlock()
	}()

	t.Run("register_custom_parser", func(t *testing.T) {
		// Ensure clean state for this subtest
		parserMutex.Lock()
		customParsers = nil
		parserMutex.Unlock()

		// Register the test parser
		registerTestYAMLParser()

		// Check it was registered
		parsers := listRegisteredParsers()
		if len(parsers) != 1 {
			t.Fatalf("Expected 1 registered parser, got %d", len(parsers))
		}

		if parsers[0] != "Test YAML Parser" {
			t.Errorf("Expected 'Test YAML Parser', got %q", parsers[0])
		}
	})

	t.Run("custom_parser_takes_priority", func(t *testing.T) {
		// Ensure clean state for this subtest
		parserMutex.Lock()
		customParsers = nil
		parserMutex.Unlock()

		// Register the test parser
		registerTestYAMLParser()

		yamlData := []byte("key: value\nnested:\n  item: test")

		// Parse YAML - should use custom parser
		result, err := ParseConfig(yamlData, FormatYAML)
		if err != nil {
			t.Fatalf("Failed to parse YAML: %v", err)
		}

		// Check that custom parser was used (it adds special markers)
		if result["_parser"] != "test-yaml-plugin" {
			t.Errorf("Custom parser was not used. Result: %+v", result)
		}
	})

	t.Run("fallback_to_builtin_when_no_custom", func(t *testing.T) {
		// Clear custom parsers for this subtest
		parserMutex.Lock()
		customParsers = nil
		parserMutex.Unlock()

		yamlData := []byte("key: value\nport: 8080")

		// Parse YAML - should use built-in parser
		result, err := ParseConfig(yamlData, FormatYAML)
		if err != nil {
			t.Fatalf("Failed to parse YAML: %v", err)
		}

		// Check that built-in parser was used (no special markers)
		if _, hasMarker := result["_parser"]; hasMarker {
			t.Errorf("Built-in parser should not add _parser marker. Result: %+v", result)
		}

		// Check actual parsing worked
		if result["key"] != "value" {
			t.Errorf("Expected key=value, got key=%v", result["key"])
		}
	})

	t.Run("parser_interface_contract", func(t *testing.T) {
		// This test doesn't modify global state, so no cleanup needed
		parser := &testAdvancedYAMLParser{}

		// Test Supports method
		if !parser.Supports(FormatYAML) {
			t.Error("Parser should support YAML format")
		}

		if parser.Supports(FormatJSON) {
			t.Error("Parser should not support JSON format")
		}

		// Test Name method
		name := parser.Name()
		if name == "" {
			t.Error("Parser name should not be empty")
		}

		// Test Parse method
		data := []byte("test: data")
		result, err := parser.Parse(data)
		if err != nil {
			t.Errorf("Parser.Parse failed: %v", err)
		}

		if result == nil {
			t.Error("Parser.Parse should not return nil result")
		}
	})
}

func TestParserPluginConcurrency(t *testing.T) {
	// Save original state and ensure complete cleanup
	parserMutex.Lock()
	originalParsers := make([]ConfigParser, len(customParsers))
	copy(originalParsers, customParsers)
	customParsers = nil // Clear for clean test
	parserMutex.Unlock()

	// Restore original state when test completes
	defer func() {
		parserMutex.Lock()
		customParsers = originalParsers
		parserMutex.Unlock()
	}()

	// Test that parser registration is thread-safe
	done := make(chan bool, 10)

	// Start multiple goroutines registering parsers
	for i := 0; i < 10; i++ {
		go func() {
			registerTestYAMLParser()
			done <- true
		}()
	}

	// Wait for all to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check that we have the expected number of parsers
	// (Note: We might have duplicates, which is expected behavior)
	parsers := listRegisteredParsers()
	if len(parsers) < 1 {
		t.Errorf("Expected at least 1 parser, got %d", len(parsers))
	}
}
