// config_writer_test.go: Tests for ConfigWriter functionality
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigWriterBasicOperations tests core ConfigWriter functionality
func TestConfigWriterBasicOperations(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_config.json")

	// Create writer with empty config
	writer, err := NewConfigWriter(configPath, FormatJSON, nil)
	if err != nil {
		t.Fatalf("Failed to create ConfigWriter: %v", err)
	}

	// Test setting simple value
	err = writer.SetValue("app.name", "test-app")
	if err != nil {
		t.Fatalf("Failed to set simple value: %v", err)
	}

	// Test setting nested value
	err = writer.SetValue("database.host", "localhost")
	if err != nil {
		t.Fatalf("Failed to set nested value: %v", err)
	}

	err = writer.SetValue("database.port", 5432)
	if err != nil {
		t.Fatalf("Failed to set nested value with number: %v", err)
	}

	// Write to file
	err = writer.WriteConfig()
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read written config: %v", err)
	}

	var parsed map[string]interface{}
	err = json.Unmarshal(content, &parsed)
	if err != nil {
		t.Fatalf("Written config is not valid JSON: %v", err)
	}

	// Verify values
	if app, ok := parsed["app"].(map[string]interface{}); ok {
		if name, ok := app["name"].(string); !ok || name != "test-app" {
			t.Errorf("Expected app.name='test-app', got %v", name)
		}
	} else {
		t.Error("app section not found or invalid")
	}

	if db, ok := parsed["database"].(map[string]interface{}); ok {
		if host, ok := db["host"].(string); !ok || host != "localhost" {
			t.Errorf("Expected database.host='localhost', got %v", host)
		}
		if port, ok := db["port"].(float64); !ok || port != 5432 {
			t.Errorf("Expected database.port=5432, got %v", port)
		}
	} else {
		t.Error("database section not found or invalid")
	}
}

// TestConfigWriterAtomicWrite tests that writes are atomic
func TestConfigWriterAtomicWrite(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "atomic_test.json")

	// Create initial config
	writer, err := NewConfigWriter(configPath, FormatJSON, nil)
	if err != nil {
		t.Fatalf("Failed to create ConfigWriter: %v", err)
	}

	if err := writer.SetValue("initial", "value"); err != nil {
		t.Logf("Failed to set value: %v", err)
	}
	err = writer.WriteConfig()
	if err != nil {
		t.Fatalf("Failed to write initial config: %v", err)
	}

	// Verify initial file exists
	initialContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read initial config: %v", err)
	}

	// Update existing writer instead of creating new one
	if err := writer.SetValue("updated", "value"); err != nil {
		t.Logf("Failed to set value: %v", err)
	}
	err = writer.WriteConfig()
	if err != nil {
		t.Fatalf("Failed to write updated config: %v", err)
	}

	// Verify content was updated
	updatedContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read updated config: %v", err)
	}

	if string(initialContent) == string(updatedContent) {
		t.Error("Config content was not updated")
	}

	var parsed map[string]interface{}
	err = json.Unmarshal(updatedContent, &parsed)
	if err != nil {
		t.Fatalf("Updated config is not valid JSON: %v", err)
	}

	// Should contain both initial and updated values
	if initial, ok := parsed["initial"].(string); !ok || initial != "value" {
		t.Errorf("Expected initial='value', got %v", initial)
	}
	if updated, ok := parsed["updated"].(string); !ok || updated != "value" {
		t.Errorf("Expected updated='value', got %v", updated)
	}
}

// TestConfigWriterErrorHandling tests error conditions
func TestConfigWriterErrorHandling(t *testing.T) {
	// Test invalid file path
	_, err := NewConfigWriter("", FormatJSON, nil)
	if err == nil {
		t.Error("Expected error for empty file path")
	}

	// Test supported YAML format
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test.yaml")

	writer, err := NewConfigWriter(configPath, FormatYAML, nil)
	if err != nil {
		t.Fatalf("Failed to create ConfigWriter: %v", err)
	}

	if err := writer.SetValue("test", "value"); err != nil {
		t.Logf("Failed to set value: %v", err)
	}
	err = writer.WriteConfig()
	if err != nil {
		t.Errorf("YAML format should be supported: %v", err)
	}

	// Test unsupported format by using FormatUnknown
	// First, make sure there are changes to force serialization
	if err := writer.SetValue("force_change", "trigger_serialization"); err != nil {
		t.Logf("Failed to set value: %v", err)
	}
	writer.format = FormatUnknown
	err = writer.WriteConfig()
	if err == nil {
		t.Error("Expected error for unsupported format")
	}
}

// TestConfigWriterBufferReuse tests that buffers are reused correctly
func TestConfigWriterBufferReuse(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "buffer_test.json")

	writer, err := NewConfigWriter(configPath, FormatJSON, nil)
	if err != nil {
		t.Fatalf("Failed to create ConfigWriter: %v", err)
	}

	// Check initial buffer capacity
	initialCap := cap(writer.valueBuffer)

	// Set a value that should fit in existing buffer
	if err := writer.SetValue("small", "value"); err != nil {
		t.Logf("Failed to set value: %v", err)
	}
	err = writer.WriteConfig()
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Buffer capacity should not have changed for small config
	if cap(writer.valueBuffer) != initialCap {
		t.Errorf("Buffer capacity changed unexpectedly: %d -> %d", initialCap, cap(writer.valueBuffer))
	}
}

// BenchmarkConfigWriterSetValue benchmarks SetValue performance
func BenchmarkConfigWriterSetValue(b *testing.B) {
	tempDir := b.TempDir()
	configPath := filepath.Join(tempDir, "bench_config.json")

	writer, err := NewConfigWriter(configPath, FormatJSON, nil)
	if err != nil {
		b.Fatalf("Failed to create ConfigWriter: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := writer.SetValue("benchmark.key", "value")
		if err != nil {
			b.Fatalf("SetValue failed: %v", err)
		}
	}
}

// BenchmarkConfigWriterWriteConfig benchmarks WriteConfig performance
func BenchmarkConfigWriterWriteConfig(b *testing.B) {
	tempDir := b.TempDir()

	// Pre-create config with some data
	config := map[string]interface{}{
		"app": map[string]interface{}{
			"name":    "benchmark-app",
			"version": "1.0.0",
		},
		"database": map[string]interface{}{
			"host": "localhost",
			"port": 5432,
		},
	}

	// Use same config file for all iterations
	configPath := filepath.Join(tempDir, "bench_config.json")
	writer, err := NewConfigWriter(configPath, FormatJSON, config)
	if err != nil {
		b.Fatalf("Failed to create ConfigWriter: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err = writer.WriteConfig()
		if err != nil {
			b.Fatalf("WriteConfig failed: %v", err)
		}
	}
}

func TestConfigWriter_WriteConfig_INI(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.ini")

	writer, err := NewConfigWriter(testFile, FormatINI, make(map[string]interface{}))
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Set values for INI format
	if err := writer.SetValue("host", "localhost"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}
	if err := writer.SetValue("database.host", "localhost"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}
	if err := writer.SetValue("database.port", 5432); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	// Write to file
	if err := writer.WriteConfig(); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	// Verify file exists and has correct content
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	content := string(data)

	// Check for flat key
	if !strings.Contains(content, "host=localhost") {
		t.Error("Expected flat key 'host=localhost' not found")
	}

	// Check for section
	if !strings.Contains(content, "[database]") {
		t.Error("Expected section '[database]' not found")
	}

	// Check for section key
	if !strings.Contains(content, "port=5432") {
		t.Error("Expected section key 'port=5432' not found")
	}
}

func TestConfigWriter_WriteConfig_Properties(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.properties")

	writer, err := NewConfigWriter(testFile, FormatProperties, make(map[string]interface{}))
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Set nested values for Properties format (should be flattened)
	if err := writer.SetValue("database.host", "localhost"); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}
	if err := writer.SetValue("database.port", 5432); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}
	if err := writer.SetValue("app.debug", true); err != nil {
		t.Fatalf("SetValue failed: %v", err)
	}

	// Write to file
	if err := writer.WriteConfig(); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	// Verify file exists and has correct content
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read config file: %v", err)
	}

	content := string(data)

	// Check for flattened keys
	if !strings.Contains(content, "database.host=localhost") {
		t.Error("Expected flattened key 'database.host=localhost' not found")
	}

	if !strings.Contains(content, "database.port=5432") {
		t.Error("Expected flattened key 'database.port=5432' not found")
	}

	if !strings.Contains(content, "app.debug=true") {
		t.Error("Expected flattened key 'app.debug=true' not found")
	}
}

// TestConfigWriter_MissingFunctions tests previously uncovered functions
func TestConfigWriter_MissingFunctions(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("WriteConfigAs", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "source.json")
		targetPath := filepath.Join(tempDir, "target.json")

		writer, err := NewConfigWriter(configPath, FormatJSON, nil)
		if err != nil {
			t.Fatalf("Failed to create ConfigWriter: %v", err)
		}

		// Set some test data
		err = writer.SetValue("app.name", "test-app")
		if err != nil {
			t.Fatalf("Failed to set value: %v", err)
		}

		// Test WriteConfigAs with empty path (should fail)
		err = writer.WriteConfigAs("")
		if err == nil {
			t.Error("WriteConfigAs with empty path should fail")
		}

		// Test successful WriteConfigAs
		err = writer.WriteConfigAs(targetPath)
		if err != nil {
			t.Fatalf("WriteConfigAs failed: %v", err)
		}

		// Verify target file exists and has correct content
		if _, err := os.Stat(targetPath); err != nil {
			t.Errorf("Target file was not created: %v", err)
		}

		// Read and verify content
		data, err := os.ReadFile(targetPath)
		if err != nil {
			t.Fatalf("Failed to read target file: %v", err)
		}

		var config map[string]interface{}
		err = json.Unmarshal(data, &config)
		if err != nil {
			t.Fatalf("Failed to parse target config: %v", err)
		}

		if appData, ok := config["app"].(map[string]interface{}); !ok {
			t.Error("Expected app object in target config")
		} else if name, ok := appData["name"].(string); !ok || name != "test-app" {
			t.Errorf("Expected app.name=test-app, got %v", name)
		}
	})

	t.Run("GetConfig", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "getconfig.json")

		writer, err := NewConfigWriter(configPath, FormatJSON, nil)
		if err != nil {
			t.Fatalf("Failed to create ConfigWriter: %v", err)
		}

		// Set some test data including nested structures
		err = writer.SetValue("app.name", "test-app")
		if err != nil {
			t.Fatalf("Failed to set app.name: %v", err)
		}

		err = writer.SetValue("database.host", "localhost")
		if err != nil {
			t.Fatalf("Failed to set database.host: %v", err)
		}

		// Get config copy
		configCopy := writer.GetConfig()

		// Verify copy has correct content
		if appData, ok := configCopy["app"].(map[string]interface{}); !ok {
			t.Error("Expected app object in config copy")
		} else if name, ok := appData["name"].(string); !ok || name != "test-app" {
			t.Errorf("Expected app.name=test-app in copy, got %v", name)
		}

		// Verify it's a deep copy by modifying the copy and checking original
		if appData, ok := configCopy["app"].(map[string]interface{}); ok {
			appData["name"] = "modified-app"
		}

		// Original should be unchanged
		originalValue := writer.GetValue("app.name")
		if originalValue != "test-app" {
			t.Errorf("Original config was modified when copy was changed: %v", originalValue)
		}
	})

	t.Run("Reset", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "reset.json")

		// Create initial config file
		initialConfig := map[string]interface{}{
			"app": map[string]interface{}{
				"name":    "initial-app",
				"version": "1.0.0",
			},
			"database": map[string]interface{}{
				"host": "initial-host",
				"port": 5432,
			},
		}

		data, err := json.Marshal(initialConfig)
		if err != nil {
			t.Fatalf("Failed to marshal initial config: %v", err)
		}

		err = os.WriteFile(configPath, data, 0600)
		if err != nil {
			t.Fatalf("Failed to write initial config file: %v", err)
		}

		// Create writer and modify it
		writer, err := NewConfigWriter(configPath, FormatJSON, nil)
		if err != nil {
			t.Fatalf("Failed to create ConfigWriter: %v", err)
		}

		// Modify the config
		err = writer.SetValue("app.name", "modified-app")
		if err != nil {
			t.Fatalf("Failed to modify config: %v", err)
		}

		// Verify modification
		if writer.GetValue("app.name") != "modified-app" {
			t.Error("Config was not modified as expected")
		}

		// Reset should restore original values
		err = writer.Reset()
		if err != nil {
			t.Fatalf("Reset failed: %v", err)
		}

		// Verify reset worked
		if writer.GetValue("app.name") != "initial-app" {
			t.Errorf("Reset did not restore original value: got %v, expected initial-app", writer.GetValue("app.name"))
		}

		if writer.GetValue("app.version") != "1.0.0" {
			t.Errorf("Reset did not restore version: got %v, expected 1.0.0", writer.GetValue("app.version"))
		}
	})

	t.Run("Reset_NonExistentFile", func(t *testing.T) {
		nonExistentPath := filepath.Join(tempDir, "nonexistent.json")

		writer, err := NewConfigWriter(nonExistentPath, FormatJSON, nil)
		if err != nil {
			t.Fatalf("Failed to create ConfigWriter: %v", err)
		}

		// Set some data
		err = writer.SetValue("test.key", "test-value")
		if err != nil {
			t.Fatalf("Failed to set test value: %v", err)
		}

		// Reset should succeed and clear config when file doesn't exist
		err = writer.Reset()
		if err == nil {
			// Success case - file doesn't exist, config should be empty
			if len(writer.GetConfig()) != 0 {
				t.Errorf("Config should be empty after reset of nonexistent file, got %v", writer.GetConfig())
			}
		} else {
			// Error case - should still result in empty config according to Reset implementation
			if len(writer.GetConfig()) != 0 {
				t.Errorf("Config should be empty after failed reset, got %v", writer.GetConfig())
			}
			t.Logf("Reset returned expected error for nonexistent file: %v", err)
		}
	})
}

// TestDeepCopySlice tests the deepCopySlice function specifically
func TestDeepCopySlice(t *testing.T) {
	t.Run("nil_slice", func(t *testing.T) {
		result := deepCopySlice(nil)
		if result != nil {
			t.Errorf("Expected nil for nil input, got %v", result)
		}
	})

	t.Run("empty_slice", func(t *testing.T) {
		input := []interface{}{}
		result := deepCopySlice(input)
		if len(result) != 0 {
			t.Errorf("Expected empty slice, got length %d", len(result))
		}
	})

	t.Run("simple_values", func(t *testing.T) {
		input := []interface{}{"string", 42, true, 3.14}
		result := deepCopySlice(input)

		if len(result) != len(input) {
			t.Errorf("Expected length %d, got %d", len(input), len(result))
		}

		for i, v := range input {
			if result[i] != v {
				t.Errorf("Expected %v at index %d, got %v", v, i, result[i])
			}
		}
	})

	t.Run("nested_maps", func(t *testing.T) {
		nestedMap := map[string]interface{}{
			"inner":  "value",
			"number": 123,
		}
		input := []interface{}{
			"string",
			nestedMap,
			42,
		}

		result := deepCopySlice(input)

		// Verify structure
		if len(result) != 3 {
			t.Fatalf("Expected length 3, got %d", len(result))
		}

		// Check deep copy by modifying original map
		nestedMap["inner"] = "modified"

		// Result should not be affected
		if resultMap, ok := result[1].(map[string]interface{}); !ok {
			t.Error("Expected map at index 1")
		} else if inner, ok := resultMap["inner"].(string); !ok || inner != "value" {
			t.Errorf("Deep copy failed: expected 'value', got %v", inner)
		}
	})

	t.Run("nested_slices", func(t *testing.T) {
		nestedSlice := []interface{}{"nested", 456}
		input := []interface{}{
			"string",
			nestedSlice,
			42,
		}

		result := deepCopySlice(input)

		// Verify structure
		if len(result) != 3 {
			t.Fatalf("Expected length 3, got %d", len(result))
		}

		// Check deep copy by modifying original slice
		nestedSlice[0] = "modified"

		// Result should not be affected
		if resultSlice, ok := result[1].([]interface{}); !ok {
			t.Error("Expected slice at index 1")
		} else if first, ok := resultSlice[0].(string); !ok || first != "nested" {
			t.Errorf("Deep copy failed: expected 'nested', got %v", first)
		}
	})

	t.Run("complex_nested_structure", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": []interface{}{
						"deep_value",
						map[string]interface{}{
							"level3": "deepest",
						},
					},
				},
			},
		}

		result := deepCopySlice(input)

		// Verify deep copy worked
		if len(result) != 1 {
			t.Fatalf("Expected length 1, got %d", len(result))
		}

		// Navigate to deepest level and verify
		if topMap, ok := result[0].(map[string]interface{}); !ok {
			t.Error("Expected map at top level")
		} else if level1, ok := topMap["level1"].(map[string]interface{}); !ok {
			t.Error("Expected level1 map")
		} else if level2, ok := level1["level2"].([]interface{}); !ok {
			t.Error("Expected level2 slice")
		} else if level3Map, ok := level2[1].(map[string]interface{}); !ok {
			t.Error("Expected map at level2[1]")
		} else if deepest, ok := level3Map["level3"].(string); !ok || deepest != "deepest" {
			t.Errorf("Expected 'deepest', got %v", deepest)
		}
	})
}

// Test Reset method functionality
func TestConfigWriterReset(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "reset_test.json")

	// Create writer with some data
	config := map[string]interface{}{
		"app": map[string]interface{}{
			"name": "test",
		},
	}

	writer, err := NewConfigWriter(configPath, FormatJSON, config)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Reset should work without error
	writer.Reset()

	// After reset, config should be empty
	keys := writer.ListKeys("")
	if len(keys) != 0 {
		t.Errorf("Expected empty config after reset, got %d keys", len(keys))
	}
}

// Test atomicWrite method functionality (simple case)
func TestConfigWriterAtomicWriteSimple(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "atomic_test.json")

	writer, err := NewConfigWriter(configPath, FormatJSON, nil)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Set a simple value
	err = writer.SetValue("test", "value")
	if err != nil {
		t.Fatalf("Failed to set value: %v", err)
	}

	// WriteConfig uses atomicWrite internally
	err = writer.WriteConfig()
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("File was not created by atomic write")
	}
}

// Test setNestedValue function (simple case)
func TestConfigWriterSetNestedValue(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "nested_test.json")

	writer, err := NewConfigWriter(configPath, FormatJSON, nil)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Test simple nested value
	err = writer.SetValue("app.name", "test-app")
	if err != nil {
		t.Fatalf("Failed to set nested value: %v", err)
	}

	// Verify value was set
	value := writer.GetValue("app.name")
	if value != "test-app" {
		t.Errorf("Expected 'test-app', got %v", value)
	}
}

// Test deleteNestedValue function
func TestConfigWriterDeleteNestedValue(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "delete_nested_test.json")

	// Create writer with initial data
	config := map[string]interface{}{
		"app": map[string]interface{}{
			"name":    "test",
			"version": "1.0.0",
		},
	}

	writer, err := NewConfigWriter(configPath, FormatJSON, config)
	if err != nil {
		t.Fatalf("Failed to create writer: %v", err)
	}

	// Delete nested value
	deleted := writer.DeleteValue("app.version")
	if !deleted {
		t.Error("Expected deletion to succeed")
	}

	// Verify value was deleted
	value := writer.GetValue("app.version")
	if value != nil {
		t.Errorf("Expected nil after deletion, got %v", value)
	}

	// Verify other value still exists
	name := writer.GetValue("app.name")
	if name != "test" {
		t.Errorf("Expected 'test', got %v", name)
	}
}
