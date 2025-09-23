package argus

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfigWriterAllFormats verifies that all supported formats
// have consistent behavior for basic operations.
func TestConfigWriterAllFormats(t *testing.T) {
	// Test data covering different value types
	testConfig := map[string]interface{}{
		"string_value": "hello world",
		"int_value":    42,
		"bool_value":   true,
		"float_value":  3.14,
		"nested": map[string]interface{}{
			"key1": "value1",
			"key2": 100,
		},
	}

	// All supported formats with correct file extensions
	formats := []struct {
		name      string
		format    ConfigFormat
		extension string
	}{
		{"JSON", FormatJSON, "json"},
		{"YAML", FormatYAML, "yaml"},
		{"TOML", FormatTOML, "toml"},
		{"HCL", FormatHCL, "hcl"},
		{"INI", FormatINI, "ini"},
		{"Properties", FormatProperties, "properties"},
	}

	for _, fmt := range formats {
		t.Run(fmt.name, func(t *testing.T) {
			// Create temporary file for this format
			tempDir := t.TempDir()
			configFile := filepath.Join(tempDir, "test."+fmt.extension)

			// Test writer creation
			writer, err := NewConfigWriter(configFile, fmt.format, testConfig)
			if err != nil {
				t.Fatalf("Failed to create writer for %s: %v", fmt.name, err)
			}

			// ConfigWriter only writes if there are changes, so make a change first
			if err := writer.SetValue("initialized", true); err != nil {
				t.Fatalf("Failed to set initial marker for %s: %v", fmt.name, err)
			}

			// Test initial write
			if err := writer.WriteConfig(); err != nil {
				t.Fatalf("Failed to write initial config for %s: %v", fmt.name, err)
			}

			// Verify file was created
			if _, err := os.Stat(configFile); os.IsNotExist(err) {
				t.Fatalf("Config file not created for %s", fmt.name)
			}

			// Test value setting
			if err := writer.SetValue("new_key", "new_value"); err != nil {
				t.Errorf("SetValue failed for %s: %v", fmt.name, err)
			}

			// Test nested value setting
			if err := writer.SetValue("nested.new_nested", "nested_value"); err != nil {
				t.Errorf("Nested SetValue failed for %s: %v", fmt.name, err)
			}

			// Test value retrieval
			if got := writer.GetValue("string_value"); got != "hello world" {
				t.Errorf("GetValue failed for %s: got %v, want %q", fmt.name, got, "hello world")
			}

			// Test changes detection
			if !writer.HasChanges() {
				t.Errorf("HasChanges should be true after modifications for %s", fmt.name)
			}

			// Test write with changes
			if err := writer.WriteConfig(); err != nil {
				t.Errorf("Failed to write modified config for %s: %v", fmt.name, err)
			}

			// Verify the new values are accessible after write
			if got := writer.GetValue("new_key"); got != "new_value" {
				t.Errorf("New value not preserved after write for %s: got %v, want %q",
					fmt.name, got, "new_value")
			}

			if got := writer.GetValue("nested.new_nested"); got != "nested_value" {
				t.Errorf("New nested value not preserved after write for %s: got %v, want %q",
					fmt.name, got, "nested_value")
			}
		})
	}
}

// TestConfigWriterRoundTrip verifies that all formats can be written and read back correctly.
func TestConfigWriterRoundTrip(t *testing.T) {
	// Full-featured test data for formats that support complex structures
	complexTestData := map[string]interface{}{
		"app": map[string]interface{}{
			"name":    "test-app",
			"version": "1.0.0",
			"debug":   true,
		},
		"server": map[string]interface{}{
			"port":    8080,
			"timeout": 30,
		},
	}

	// Simple test data for formats with limitations (INI, Properties)
	simpleTestData := map[string]interface{}{
		"app_name":    "test-app",
		"app_version": "1.0.0",
		"app_debug":   true,
		"server_port": 8080,
	}

	// Test complex formats that support nested structures
	complexFormats := []struct {
		format ConfigFormat
		ext    string
	}{
		{FormatJSON, "json"},
		{FormatYAML, "yaml"},
		{FormatTOML, "toml"},
		{FormatHCL, "hcl"},
	}

	for _, fmt := range complexFormats {
		t.Run(fmt.format.String()+"_complex", func(t *testing.T) {
			tempDir := t.TempDir()
			configFile := filepath.Join(tempDir, "complex."+fmt.ext)

			writer, err := NewConfigWriter(configFile, fmt.format, complexTestData)
			if err != nil {
				t.Fatalf("Failed to create writer: %v", err)
			}

			// Force write by making a change (ConfigWriter optimizes away unchanged configs)
			if err := writer.SetValue("test_marker", true); err != nil {
				t.Fatalf("Failed to set test marker: %v", err)
			}

			if err := writer.WriteConfig(); err != nil {
				t.Fatalf("Failed to write config: %v", err)
			}

			data, err := os.ReadFile(configFile)
			if err != nil {
				t.Fatalf("Failed to read written file: %v", err)
			}

			parsed, err := ParseConfig(data, fmt.format)
			if err != nil {
				t.Fatalf("Failed to parse written config: %v", err)
			}

			if _, ok := parsed["app"]; !ok {
				t.Errorf("Key 'app' missing after round trip")
			}
			if _, ok := parsed["server"]; !ok {
				t.Errorf("Key 'server' missing after round trip")
			}
		})
	}

	// Test simple formats with flat structures
	simpleFormats := []struct {
		format ConfigFormat
		ext    string
	}{
		{FormatINI, "ini"},
		{FormatProperties, "properties"},
	}

	for _, fmt := range simpleFormats {
		t.Run(fmt.format.String()+"_simple", func(t *testing.T) {
			tempDir := t.TempDir()
			configFile := filepath.Join(tempDir, "simple."+fmt.ext)

			writer, err := NewConfigWriter(configFile, fmt.format, simpleTestData)
			if err != nil {
				t.Fatalf("Failed to create writer: %v", err)
			}

			// Force write by making a change (ConfigWriter optimizes away unchanged configs)
			if err := writer.SetValue("test_marker", true); err != nil {
				t.Fatalf("Failed to set test marker: %v", err)
			}

			if err := writer.WriteConfig(); err != nil {
				t.Fatalf("Failed to write config: %v", err)
			}

			data, err := os.ReadFile(configFile)
			if err != nil {
				t.Fatalf("Failed to read written file: %v", err)
			}

			if len(data) == 0 {
				t.Fatalf("Written file is empty")
			}

			parsed, err := ParseConfig(data, fmt.format)
			if err != nil {
				t.Fatalf("Failed to parse written config: %v", err)
			}

			// Check that flat keys exist
			if _, ok := parsed["app_name"]; !ok {
				t.Errorf("Key 'app_name' missing after round trip")
			}
		})
	}
}

// TestConfigWriterOperations verifies advanced operations work across all formats.
func TestConfigWriterOperations(t *testing.T) {
	baseConfig := map[string]interface{}{
		"section1": map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
		"section2": map[string]interface{}{
			"key3": 42,
			"key4": true,
		},
	}

	formats := []ConfigFormat{FormatJSON, FormatYAML, FormatTOML}

	for _, format := range formats {
		t.Run(format.String()+"_operations", func(t *testing.T) {
			tempDir := t.TempDir()
			configFile := filepath.Join(tempDir, "ops."+format.String())

			writer, err := NewConfigWriter(configFile, format, baseConfig)
			if err != nil {
				t.Fatalf("Failed to create writer: %v", err)
			}

			// Test key listing
			keys := writer.ListKeys("")
			if len(keys) < 2 {
				t.Errorf("ListKeys should return at least 2 top-level keys, got %d", len(keys))
			}

			// Test prefix filtering
			section1Keys := writer.ListKeys("section1")
			if len(section1Keys) != 2 {
				t.Errorf("ListKeys('section1') should return 2 keys, got %d", len(section1Keys))
			}

			// Test deletion
			if !writer.DeleteValue("section1.key1") {
				t.Error("DeleteValue should return true for existing key")
			}

			if writer.DeleteValue("nonexistent.key") {
				t.Error("DeleteValue should return false for non-existent key")
			}

			// Verify deletion worked
			if val := writer.GetValue("section1.key1"); val != nil {
				t.Errorf("Deleted key should return nil, got %v", val)
			}

			// Test that remaining keys still exist
			if val := writer.GetValue("section1.key2"); val != "value2" {
				t.Errorf("Non-deleted key should remain, got %v", val)
			}
		})
	}
}
