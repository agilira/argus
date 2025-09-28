// parser_robustness_test.go
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
)

func TestParserRobustness(t *testing.T) {
	t.Run("toml_validation", func(t *testing.T) {
		// Test TOML errors
		malformedTOML := `
[valid_section]
key = value
invalid line without equals
another = valid`

		_, err := parseBuiltin([]byte(malformedTOML), FormatTOML)
		if err == nil {
			t.Error("TOML parser should reject malformed syntax")
		} else {
			t.Logf("TOML correctly caught error: %v", err)
		}
	})

	t.Run("ini_validation", func(t *testing.T) {
		// Test INI errors
		malformedINI := `
[valid_section]
key=value
invalid line without equals
another=valid`

		_, err := parseBuiltin([]byte(malformedINI), FormatINI)
		if err == nil {
			t.Error("INI parser should reject malformed syntax")
		} else {
			t.Logf("INI correctly caught error: %v", err)
		}
	})

	t.Run("properties_validation", func(t *testing.T) {
		// Test Properties errors with empty key
		malformedProperties := `
key=value
=empty_key_value
another=valid`

		_, err := parseBuiltin([]byte(malformedProperties), FormatProperties)
		if err == nil {
			t.Error("Properties parser should reject malformed syntax")
		} else {
			t.Logf("Properties correctly caught error: %v", err)
		}
	})

	t.Run("yaml_validation", func(t *testing.T) {
		// Test YAML errors (already verified previously)
		malformedYAML := `
key: value
invalid line without colon
another: valid`

		_, err := parseBuiltin([]byte(malformedYAML), FormatYAML)
		if err == nil {
			t.Error("YAML parser should reject malformed syntax")
		} else {
			t.Logf("YAML correctly caught error: %v", err)
		}
	})

	t.Run("json_validation", func(t *testing.T) {
		// JSON uses the standard library which already handles errors
		malformedJSON := `{"key": "value", invalid}`

		_, err := parseBuiltin([]byte(malformedJSON), FormatJSON)
		if err == nil {
			t.Error("JSON parser should reject malformed syntax")
		} else {
			t.Logf("JSON correctly caught error: %v", err)
		}
	})

	t.Run("empty_keys_validation", func(t *testing.T) {
		// Test empty keys for all parsers
		testCases := []struct {
			format ConfigFormat
			data   string
			name   string
		}{
			{FormatTOML, "=value", "TOML empty key"},
			{FormatINI, "=value", "INI empty key"},
			{FormatProperties, "=value", "Properties empty key"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := parseBuiltin([]byte(tc.data), tc.format)
				if err == nil {
					t.Errorf("%s should reject empty keys", tc.name)
				} else {
					t.Logf("%s correctly rejected empty key: %v", tc.name, err)
				}
			})
		}
	})

	t.Run("backwards_compatibility", func(t *testing.T) {
		// Verify that parsers still work with valid data
		validConfigs := map[ConfigFormat]string{
			FormatTOML: `
[app]
name = "test"
port = 8080`,
			FormatINI: `
[app]
name=test
port=8080`,
			FormatProperties: `
app.name=test
app.port=8080`,
			FormatYAML: `
app:
  name: test
  port: 8080`,
			FormatJSON: `{"app": {"name": "test", "port": 8080}}`,
		}

		for format, data := range validConfigs {
			t.Run(format.String(), func(t *testing.T) {
				result, err := parseBuiltin([]byte(data), format)
				if err != nil {
					t.Errorf("Valid %s should parse without error: %v", format.String(), err)
				} else if len(result) == 0 {
					t.Errorf("Valid %s should return non-empty result", format.String())
				} else {
					t.Logf("%s parsed successfully with %d keys", format.String(), len(result))
				}
			})
		}
	})
}

// TestParser_MissingCoverage tests previously uncovered parser functions
func TestParser_MissingCoverage(t *testing.T) {
	t.Run("parseTOMLArray", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    string
			expected []interface{}
		}{
			{
				name:     "empty_array",
				input:    "[]",
				expected: []interface{}{},
			},
			{
				name:     "simple_string_array",
				input:    `["one", "two", "three"]`,
				expected: []interface{}{"one", "two", "three"},
			},
			{
				name:     "number_array",
				input:    "[1, 2, 3, 42]",
				expected: []interface{}{1, 2, 3, 42},
			},
			{
				name:     "mixed_types",
				input:    `[123, "text", true, false]`,
				expected: []interface{}{123, "text", true, false},
			},
			{
				name:     "spaces_handling",
				input:    `[ "spaced" , 42 , true ]`,
				expected: []interface{}{"spaced", 42, true},
			},
			{
				name:     "single_item",
				input:    `["single"]`,
				expected: []interface{}{"single"},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := parseTOMLArray(tc.input)

				resultSlice, ok := result.([]interface{})
				if !ok {
					t.Fatalf("Expected []interface{}, got %T", result)
				}

				if len(resultSlice) != len(tc.expected) {
					t.Fatalf("Expected length %d, got %d", len(tc.expected), len(resultSlice))
				}

				for i, expected := range tc.expected {
					if resultSlice[i] != expected {
						t.Errorf("Index %d: expected %v (%T), got %v (%T)",
							i, expected, expected, resultSlice[i], resultSlice[i])
					}
				}
			})
		}
	})

	t.Run("parseTOMLArrayWithValidation", func(t *testing.T) {
		validCases := []struct {
			name     string
			input    string
			expected []interface{}
		}{
			{
				name:     "valid_simple_array",
				input:    `["a", "b", "c"]`,
				expected: []interface{}{"a", "b", "c"},
			},
			{
				name:     "valid_empty_array",
				input:    "[]",
				expected: []interface{}{},
			},
			{
				name:     "valid_mixed_types",
				input:    `[1, "text", true]`,
				expected: []interface{}{1, "text", true},
			},
		}

		for _, tc := range validCases {
			t.Run(tc.name, func(t *testing.T) {
				result, err := parseTOMLArrayWithValidation(tc.input, 1)
				if err != nil {
					t.Errorf("Valid input should not error: %v", err)
				}

				resultSlice, ok := result.([]interface{})
				if !ok {
					t.Fatalf("Expected []interface{}, got %T", result)
				}

				if len(resultSlice) != len(tc.expected) {
					t.Fatalf("Expected length %d, got %d", len(tc.expected), len(resultSlice))
				}

				for i, expected := range tc.expected {
					if resultSlice[i] != expected {
						t.Errorf("Index %d: expected %v, got %v", i, expected, resultSlice[i])
					}
				}
			})
		}

		// Test error cases
		errorCases := []struct {
			name  string
			input string
		}{
			{
				name:  "unmatched_brackets_missing_close",
				input: `["unclosed"`,
			},
			{
				name:  "unmatched_brackets_missing_open",
				input: `"unopened"]`,
			},
			{
				name:  "multiple_unmatched_brackets",
				input: `[["nested", but, "not closed"`,
			},
		}

		for _, tc := range errorCases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := parseTOMLArrayWithValidation(tc.input, 1)
				if err == nil {
					t.Errorf("Invalid input %q should produce error", tc.input)
				} else {
					t.Logf("Correctly caught error for %q: %v", tc.input, err)
				}
			})
		}
	})

	t.Run("parseHCLArray", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    string
			expected []interface{}
		}{
			{
				name:     "empty_hcl_array",
				input:    "[]",
				expected: []interface{}{},
			},
			{
				name:     "hcl_string_array",
				input:    `["hcl1", "hcl2", "hcl3"]`,
				expected: []interface{}{"hcl1", "hcl2", "hcl3"},
			},
			{
				name:     "hcl_number_array",
				input:    "[10, 20, 30]",
				expected: []interface{}{10, 20, 30},
			},
			{
				name:     "hcl_boolean_array",
				input:    "[true, false, true]",
				expected: []interface{}{true, false, true},
			},
			{
				name:     "hcl_mixed_types",
				input:    `[42, "mixed", false]`,
				expected: []interface{}{42, "mixed", false},
			},
			{
				name:     "hcl_with_spaces",
				input:    `[ "spaced" , 99 , true ]`,
				expected: []interface{}{"spaced", 99, true},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := parseHCLArray(tc.input)

				resultSlice, ok := result.([]interface{})
				if !ok {
					t.Fatalf("Expected []interface{}, got %T", result)
				}

				if len(resultSlice) != len(tc.expected) {
					t.Fatalf("Expected length %d, got %d", len(tc.expected), len(resultSlice))
				}

				for i, expected := range tc.expected {
					if resultSlice[i] != expected {
						t.Errorf("Index %d: expected %v (%T), got %v (%T)",
							i, expected, expected, resultSlice[i], resultSlice[i])
					}
				}
			})
		}
	})

	t.Run("parser_array_functions_integration", func(t *testing.T) {
		// Test that all three array parsers handle similar inputs consistently
		testInput := `[1, "test", true]`

		tomlResult := parseTOMLArray(testInput)
		hclResult := parseHCLArray(testInput)

		tomlSlice, tomlOk := tomlResult.([]interface{})
		hclSlice, hclOk := hclResult.([]interface{})

		if !tomlOk || !hclOk {
			t.Fatal("Both parsers should return []interface{}")
		}

		if len(tomlSlice) != len(hclSlice) {
			t.Errorf("Parsers should return same length: TOML=%d, HCL=%d",
				len(tomlSlice), len(hclSlice))
		}

		// Test validation version consistency
		validatedResult, err := parseTOMLArrayWithValidation(testInput, 1)
		if err != nil {
			t.Errorf("Validation version should not error: %v", err)
		}

		validatedSlice, validatedOk := validatedResult.([]interface{})
		if !validatedOk {
			t.Fatal("Validation version should return []interface{}")
		}

		if len(validatedSlice) != len(tomlSlice) {
			t.Errorf("Validation and regular versions should return same length: validated=%d, regular=%d",
				len(validatedSlice), len(tomlSlice))
		}

		t.Logf("All array parsers successfully processed: %v", testInput)
	})
}
