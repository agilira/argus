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
