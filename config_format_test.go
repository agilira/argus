// config_format_test.go - Unit tests for configuration format functions
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
)

func TestFormatFunctions_ComprehensiveCoverage(t *testing.T) {
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
			{"float64_zero", 0.0, "0.0"}, // still a float when read back

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

}
