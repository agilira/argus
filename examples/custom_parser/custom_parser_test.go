// custom_parser_test.go: Custom Parser Test Suite
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"testing"
	"time"

	"github.com/agilira/argus"
)

func TestAdvancedYAMLParser_Parse(t *testing.T) {
	parser := &AdvancedYAMLParser{}

	// Test with valid YAML content
	yamlContent := `app_name: "test-app"
version: "1.0.0"
debug: true
features:
  - auth
  - logging
`

	result, err := parser.Parse([]byte(yamlContent))
	if err != nil {
		t.Fatalf("Parser failed: %v", err)
	}

	// Verify parser info is added
	parserInfo, exists := result["_parser_info"]
	if !exists {
		t.Fatal("Expected _parser_info in result")
	}

	parserInfoMap, ok := parserInfo.(map[string]interface{})
	if !ok {
		t.Fatal("Expected _parser_info to be a map")
	}

	if parserInfoMap["name"] != "Advanced YAML Parser" {
		t.Errorf("Expected name 'Advanced YAML Parser', got '%v'", parserInfoMap["name"])
	}

	if parserInfoMap["version"] != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%v'", parserInfoMap["version"])
	}

	// Verify raw content is preserved
	rawContent, exists := result["raw_content"]
	if !exists {
		t.Fatal("Expected raw_content in result")
	}

	if rawContent != yamlContent {
		t.Errorf("Raw content mismatch")
	}

	// Verify line count
	lineCount, exists := result["line_count"]
	if !exists {
		t.Fatal("Expected line_count in result")
	}

	expectedLineCount := len([]rune(yamlContent))
	if lineCount != expectedLineCount {
		t.Errorf("Expected line_count %d, got %v", expectedLineCount, lineCount)
	}
}

func TestAdvancedYAMLParser_Supports(t *testing.T) {
	parser := &AdvancedYAMLParser{}

	// Test YAML format support
	if !parser.Supports(argus.FormatYAML) {
		t.Error("Expected parser to support YAML format")
	}

	// Test non-YAML formats
	if parser.Supports(argus.FormatJSON) {
		t.Error("Expected parser to NOT support JSON format")
	}

	if parser.Supports(argus.FormatTOML) {
		t.Error("Expected parser to NOT support TOML format")
	}
}

func TestAdvancedYAMLParser_Name(t *testing.T) {
	parser := &AdvancedYAMLParser{}

	name := parser.Name()
	expectedName := "Advanced YAML Parser (Demo)"

	if name != expectedName {
		t.Errorf("Expected name '%s', got '%s'", expectedName, name)
	}
}

func TestCustomParser_Registration(t *testing.T) {
	// Test parser registration
	parser := &AdvancedYAMLParser{}

	// Register the parser
	argus.RegisterParser(parser)

	// Note: We can't directly test the registration since the function
	// is not exported, but we can test that it doesn't panic
	t.Log("Parser registration completed without errors")
}

func TestCustomParser_Integration(t *testing.T) {
	// Create a temporary YAML config file
	configContent := `app_name: "integration-test"
version: "2.0.0"
debug: false
features:
  - testing
  - integration
environment: test
`

	configFile := "/tmp/test_config.yaml"
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}
	defer func() {
		if err := os.Remove(configFile); err != nil {
			t.Logf("Failed to remove config file: %v", err)
		}
	}()

	// Register custom parser
	argus.RegisterParser(&AdvancedYAMLParser{})

	// Test with UniversalConfigWatcher
	configReceived := false
	var receivedConfig map[string]interface{}

	watcher, err := argus.UniversalConfigWatcher(configFile, func(config map[string]interface{}) {
		configReceived = true
		receivedConfig = config
	})
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer func() {
		if err := watcher.Stop(); err != nil {
			t.Logf("Failed to stop watcher: %v", err)
		}
	}()

	// Wait for initial config load
	time.Sleep(200 * time.Millisecond)

	if !configReceived {
		t.Fatal("Expected config to be received")
	}

	// Verify custom parser features are present
	if receivedConfig == nil {
		t.Fatal("Expected non-nil config")
	}

	// Check for custom parser metadata
	parserInfo, exists := receivedConfig["_parser_info"]
	if !exists {
		t.Fatal("Expected _parser_info from custom parser")
	}

	parserInfoMap, ok := parserInfo.(map[string]interface{})
	if !ok {
		t.Fatal("Expected _parser_info to be a map")
	}

	if parserInfoMap["name"] != "Advanced YAML Parser" {
		t.Errorf("Expected custom parser name, got '%v'", parserInfoMap["name"])
	}
}

func TestCustomParser_ErrorHandling(t *testing.T) {
	parser := &AdvancedYAMLParser{}

	// Test with empty content
	result, err := parser.Parse([]byte(""))
	if err != nil {
		t.Fatalf("Parser should handle empty content: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result for empty content")
	}

	// Test with invalid content (should still work due to simple parsing)
	invalidContent := `invalid: yaml: content: [`
	result, err = parser.Parse([]byte(invalidContent))
	if err != nil {
		t.Fatalf("Parser should handle invalid content gracefully: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result for invalid content")
	}
}

func TestCustomParser_Performance(t *testing.T) {
	parser := &AdvancedYAMLParser{}

	// Test performance with multiple parses
	content := `app_name: "perf-test"
version: "1.0.0"
debug: true
`

	const iterations = 1000
	start := time.Now()

	for i := 0; i < iterations; i++ {
		_, err := parser.Parse([]byte(content))
		if err != nil {
			t.Fatalf("Parser failed at iteration %d: %v", i, err)
		}
	}

	duration := time.Since(start)
	avgTime := duration / iterations

	t.Logf("Performance test: %d iterations in %v", iterations, duration)
	t.Logf("Average time per operation: %v", avgTime)
	t.Logf("Operations per second: %.0f", float64(iterations)/duration.Seconds())

	// Performance should be reasonable (less than 100µs per operation)
	if avgTime > 100*time.Microsecond {
		t.Errorf("Performance too slow: %v per operation (expected < 100µs)", avgTime)
	}
}

func TestCustomParser_EdgeCases(t *testing.T) {
	parser := &AdvancedYAMLParser{}

	t.Run("NilContent", func(t *testing.T) {
		result, err := parser.Parse(nil)
		if err != nil {
			t.Fatalf("Parser should handle nil content: %v", err)
		}

		if result == nil {
			t.Fatal("Expected non-nil result for nil content")
		}
	})

	t.Run("VeryLargeContent", func(t *testing.T) {
		// Create a large YAML content
		largeContent := ""
		for i := 0; i < 1000; i++ {
			largeContent += "key" + string(rune(i%10+'0')) + ": value" + string(rune(i%10+'0')) + "\n"
		}

		result, err := parser.Parse([]byte(largeContent))
		if err != nil {
			t.Fatalf("Parser should handle large content: %v", err)
		}

		if result == nil {
			t.Fatal("Expected non-nil result for large content")
		}

		// Verify line count is correct
		lineCount, exists := result["line_count"]
		if !exists {
			t.Fatal("Expected line_count in result")
		}

		expectedLineCount := len([]rune(largeContent))
		if lineCount != expectedLineCount {
			t.Errorf("Expected line_count %d, got %v", expectedLineCount, lineCount)
		}
	})

	t.Run("UnicodeContent", func(t *testing.T) {
		unicodeContent := `app_name: "测试应用"
version: "1.0.0"
description: "这是一个测试应用"
features:
  - "功能1"
  - "功能2"
`

		result, err := parser.Parse([]byte(unicodeContent))
		if err != nil {
			t.Fatalf("Parser should handle unicode content: %v", err)
		}

		if result == nil {
			t.Fatal("Expected non-nil result for unicode content")
		}

		// Verify raw content is preserved
		rawContent, exists := result["raw_content"]
		if !exists {
			t.Fatal("Expected raw_content in result")
		}

		if rawContent != unicodeContent {
			t.Errorf("Raw content mismatch for unicode content")
		}
	})
}

func TestCustomParser_InterfaceCompliance(t *testing.T) {
	parser := &AdvancedYAMLParser{}

	// Test that the parser implements the required interface
	var _ argus.ConfigParser = parser

	// Test all interface methods
	t.Run("Parse", func(t *testing.T) {
		_, err := parser.Parse([]byte("test: value"))
		if err != nil {
			t.Errorf("Parse method failed: %v", err)
		}
	})

	t.Run("Supports", func(t *testing.T) {
		supports := parser.Supports(argus.FormatYAML)
		if !supports {
			t.Error("Supports method should return true for YAML")
		}
	})

	t.Run("Name", func(t *testing.T) {
		name := parser.Name()
		if name == "" {
			t.Error("Name method should return non-empty string")
		}
	})
}

func TestCustomParser_ConcurrentUsage(t *testing.T) {
	parser := &AdvancedYAMLParser{}

	// Test concurrent parsing
	const goroutines = 10
	const iterationsPerGoroutine = 100

	content := `app_name: "concurrent-test"
version: "1.0.0"
debug: true
`

	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(goroutineID int) {
			defer func() { done <- true }()

			for j := 0; j < iterationsPerGoroutine; j++ {
				result, err := parser.Parse([]byte(content))
				if err != nil {
					t.Errorf("Parser failed in goroutine %d, iteration %d: %v", goroutineID, j, err)
					return
				}

				if result == nil {
					t.Errorf("Expected non-nil result in goroutine %d, iteration %d", goroutineID, j)
					return
				}

				// Verify parser info is present
				_, exists := result["_parser_info"]
				if !exists {
					t.Errorf("Expected _parser_info in goroutine %d, iteration %d", goroutineID, j)
					return
				}
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}
}
