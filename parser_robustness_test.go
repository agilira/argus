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
