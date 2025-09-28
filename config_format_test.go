// config_format_test.go - Unit tests for configuration format functions
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"strings"
	"testing"
)

func TestFormatFunctions_ComprehensiveCoverage(t *testing.T) {
	// Test formatYAMLValue with all types and edge cases
	t.Run("formatYAMLValue_comprehensive", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    interface{}
			expected string
		}{
			// Basic types
			{"nil", nil, "null"},
			{"bool_true", true, "true"},
			{"bool_false", false, "false"},
			{"string_simple", "hello", "hello"},
			{"string_empty", "", `""`},
			{"int", 42, "42"},
			{"int64", int64(9223372036854775807), "9223372036854775807"},
			{"float32", float32(3.14), "3.14"},
			{"float64", 2.71828, "2.71828"},

			// Strings requiring quoting
			{"string_reserved_true", "true", `"true"`},
			{"string_reserved_false", "FALSE", `"FALSE"`},
			{"string_reserved_null", "null", `"null"`},
			{"string_reserved_yes", "YES", `"YES"`},
			{"string_reserved_no", "no", `"no"`},
			{"string_reserved_on", "ON", `"ON"`},
			{"string_reserved_off", "off", `"off"`},

			// Strings with special characters
			{"string_with_colon", "key:value", `"key:value"`},
			{"string_with_space", "hello world", `"hello world"`},
			{"string_with_tab", "hello\tworld", "\"hello\tworld\""},
			{"string_with_newline", "hello\nworld", "\"hello\nworld\""},
			{"string_with_brackets", "[array]", `"[array]"`},
			{"string_with_braces", "{object}", `"{object}"`},
			{"string_with_pipe", "value|other", `"value|other"`},
			{"string_with_greater", "value>other", `"value>other"`},
			{"string_with_dash", "-value", `"-value"`},
			{"string_with_hash", "#comment", `"#comment"`},
			{"string_with_ampersand", "a&b", `"a&b"`},
			{"string_with_asterisk", "*wildcard", `"*wildcard"`},
			{"string_with_exclamation", "!important", `"!important"`},
			{"string_with_percent", "100%", `"100%"`},
			{"string_with_at", "@symbol", `"@symbol"`},
			{"string_with_backtick", "`code`", "\"`code`\""},
			{"string_with_quotes", `"quoted"`, `"\"quoted\""`},
			{"string_with_apostrophe", "it's", `"it's"`},
			{"string_with_backslash", `path\to\file`, `"path\\to\\file"`},

			// Strings starting with special characters
			{"string_start_dash", "-123", `"-123"`},
			{"string_start_question", "?query", `"?query"`},
			{"string_start_colon", ":key", `":key"`},
			{"string_start_bracket", "[item]", `"[item]"`},
			{"string_start_brace", "{key}", `"{key}"`},
			{"string_start_exclamation", "!tag", `"!tag"`},

			// Numeric-looking strings
			{"string_number_int", "123", `"123"`},
			{"string_number_float", "3.14", `"3.14"`},
			{"string_number_zero", "0", "0"}, // Special case: "0" doesn't get quoted

			// Arrays
			{"array_empty", []interface{}{}, "[]"},
			{"array_simple", []interface{}{"a", "b", "c"}, "[a, b, c]"},
			{"array_mixed", []interface{}{1, "hello", true}, `[1, hello, true]`},
			{"array_nested", []interface{}{[]interface{}{1, 2}, "test"}, "[[1, 2], test]"},

			// Objects/Maps
			{"map_empty", map[string]interface{}{}, "{}"},
			{"map_simple", map[string]interface{}{"key": "value"}, "{key: value}"},
			{"map_two_items", map[string]interface{}{"a": 1, "b": 2}, "{a: 1, b: 2}"},           // Order may vary but should contain both
			{"map_complex", map[string]interface{}{"a": 1, "b": 2, "c": 3}, "map[a:1 b:2 c:3]"}, // Fallback for >2 items

			// Custom types
			{"custom_type", struct{ Name string }{"test"}, `"{test}"`},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := formatYAMLValue(tc.input)

				// For maps with 2 items, order might vary - check both possibilities
				if strings.HasPrefix(tc.name, "map_two_items") {
					if !strings.Contains(result, "a: 1") || !strings.Contains(result, "b: 2") {
						t.Errorf("formatYAMLValue(%v) = %s, expected to contain both 'a: 1' and 'b: 2'", tc.input, result)
					}
				} else {
					if result != tc.expected {
						t.Errorf("formatYAMLValue(%v) = %s, expected %s", tc.input, result, tc.expected)
					}
				}
			})
		}
	})

	// Test formatTOMLValue with all types and edge cases
	t.Run("formatTOMLValue_comprehensive", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    interface{}
			expected string
		}{
			// Basic types
			{"nil", nil, `""`},
			{"bool_true", true, "true"},
			{"bool_false", false, "false"},
			{"string_simple", "hello", `"hello"`},
			{"string_empty", "", `""`},
			{"string_with_quotes", `say "hello"`, `"say \"hello\""`},
			{"int", 42, "42"},
			{"int64", int64(123456789), "123456789"},
			{"float64", 3.14159, "3.14159"},
			{"float64_scientific", 1e10, "1e+10"},

			// Arrays
			{"array_empty", []interface{}{}, "[]"},
			{"array_strings", []interface{}{"a", "b", "c"}, `["a", "b", "c"]`},
			{"array_numbers", []interface{}{1, 2, 3}, "[1, 2, 3]"},
			{"array_mixed", []interface{}{1, "hello", true}, `[1, "hello", true]`},
			{"array_nested", []interface{}{[]interface{}{1, 2}, "test"}, `[[1, 2], "test"]`},

			// Custom types
			{"custom_type", struct{ Name string }{"test"}, `"{test}"`},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := formatTOMLValue(tc.input)
				if result != tc.expected {
					t.Errorf("formatTOMLValue(%v) = %s, expected %s", tc.input, result, tc.expected)
				}
			})
		}
	})

	// Test formatHCLValue with all types and edge cases
	t.Run("formatHCLValue_comprehensive", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    interface{}
			expected string
		}{
			// Basic types
			{"nil", nil, "null"},
			{"bool_true", true, "true"},
			{"bool_false", false, "false"},
			{"string_simple", "hello", `"hello"`},
			{"string_empty", "", `""`},
			{"string_with_quotes", `say "hello"`, `"say \"hello\""`},
			{"int", 42, "42"},
			{"int64", int64(987654321), "987654321"},
			{"float64", 2.71828, "2.71828"},
			{"float64_zero", 0.0, "0"},

			// Arrays/Lists
			{"list_empty", []interface{}{}, "[]"},
			{"list_strings", []interface{}{"a", "b", "c"}, `["a", "b", "c"]`},
			{"list_numbers", []interface{}{10, 20, 30}, "[10, 20, 30]"},
			{"list_mixed", []interface{}{42, "test", false}, `[42, "test", false]`},
			{"list_nested", []interface{}{[]interface{}{"x", "y"}, 1}, `[["x", "y"], 1]`},

			// Custom types
			{"custom_type", struct{ Value int }{42}, `"{42}"`},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := formatHCLValue(tc.input)
				if result != tc.expected {
					t.Errorf("formatHCLValue(%v) = %s, expected %s", tc.input, result, tc.expected)
				}
			})
		}
	})

	// Test formatYAMLString helper function specifically
	t.Run("formatYAMLString_edge_cases", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    string
			expected string
		}{
			{"empty_string", "", `""`},
			{"simple_string", "hello", "hello"},
			{"reserved_case_insensitive", "True", `"True"`},
			{"mixed_case_reserved", "NuLL", `"NuLL"`},
			{"number_like", "42", `"42"`},
			{"float_like", "3.14", `"3.14"`},
			{"zero_special", "0", "0"}, // Zero is not quoted as per implementation
			{"complex_escaping", `line1\nline2"quoted"`, `"line1\\nline2\"quoted\""`},
			{"backslash_only", `\`, `"\\"`},
			{"quote_only", `"`, `"\""`},
			{"mixed_special", `path\to"file":value`, `"path\\to\"file\":value"`},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := formatYAMLString(tc.input)
				if result != tc.expected {
					t.Errorf("formatYAMLString(%s) = %s, expected %s", tc.input, result, tc.expected)
				}
			})
		}
	})
}
