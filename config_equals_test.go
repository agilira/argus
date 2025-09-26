// config_equals_test.go: Tests for ConfigEquals utility function
//
// This file tests the public ConfigEquals function that provides basic
// configuration comparison capabilities for users and providers.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import "testing"

// TestConfigEquals tests the public ConfigEquals function
func TestConfigEquals(t *testing.T) {
	tests := []struct {
		name     string
		config1  map[string]interface{}
		config2  map[string]interface{}
		expected bool
	}{
		{
			name:     "Empty configs should be equal",
			config1:  map[string]interface{}{},
			config2:  map[string]interface{}{},
			expected: true,
		},
		{
			name: "Identical configs should be equal",
			config1: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
				"key3": true,
			},
			config2: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
				"key3": true,
			},
			expected: true,
		},
		{
			name: "Different values should not be equal",
			config1: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
			config2: map[string]interface{}{
				"key1": "value1",
				"key2": 43,
			},
			expected: false,
		},
		{
			name: "Different keys should not be equal",
			config1: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
			config2: map[string]interface{}{
				"key1": "value1",
				"key3": 42,
			},
			expected: false,
		},
		{
			name: "Different lengths should not be equal",
			config1: map[string]interface{}{
				"key1": "value1",
				"key2": 42,
			},
			config2: map[string]interface{}{
				"key1": "value1",
			},
			expected: false,
		},
		{
			name:     "Nil vs empty should not be equal",
			config1:  nil,
			config2:  map[string]interface{}{},
			expected: false,
		},
		{
			name:     "Both nil should be equal",
			config1:  nil,
			config2:  nil,
			expected: true,
		},
		{
			name: "Mixed types with same string representation",
			config1: map[string]interface{}{
				"number": 42,
				"string": "hello",
				"bool":   true,
			},
			config2: map[string]interface{}{
				"number": 42,
				"string": "hello",
				"bool":   true,
			},
			expected: true,
		},
		{
			name: "Float vs int with same value should be equal (string comparison)",
			config1: map[string]interface{}{
				"number": 42,
			},
			config2: map[string]interface{}{
				"number": 42.0,
			},
			expected: true, // Both convert to "42" in string representation
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConfigEquals(tt.config1, tt.config2)
			if result != tt.expected {
				t.Errorf("ConfigEquals() = %v, expected %v", result, tt.expected)
				t.Errorf("Config1: %+v", tt.config1)
				t.Errorf("Config2: %+v", tt.config2)
			}
		})
	}
}

// TestConfigEqualsEdgeCases tests edge cases for ConfigEquals
func TestConfigEqualsEdgeCases(t *testing.T) {
	t.Run("Nested structures are compared as strings", func(t *testing.T) {
		config1 := map[string]interface{}{
			"nested": map[string]interface{}{
				"inner": "value",
			},
		}
		config2 := map[string]interface{}{
			"nested": map[string]interface{}{
				"inner": "value",
			},
		}

		// This should be true because string representations are the same
		result := ConfigEquals(config1, config2)
		if !result {
			t.Errorf("Expected nested structures to be equal via string comparison")
		}
	})

	t.Run("Array/slice comparison via string representation", func(t *testing.T) {
		config1 := map[string]interface{}{
			"array": []int{1, 2, 3},
		}
		config2 := map[string]interface{}{
			"array": []int{1, 2, 3},
		}

		result := ConfigEquals(config1, config2)
		if !result {
			t.Errorf("Expected arrays to be equal via string comparison")
		}
	})
}

// BenchmarkConfigEquals benchmarks the ConfigEquals function
func BenchmarkConfigEquals(b *testing.B) {
	config1 := map[string]interface{}{
		"string_key": "test_value",
		"int_key":    42,
		"float_key":  3.14,
		"bool_key":   true,
		"array_key":  []string{"a", "b", "c"},
	}

	config2 := map[string]interface{}{
		"string_key": "test_value",
		"int_key":    42,
		"float_key":  3.14,
		"bool_key":   true,
		"array_key":  []string{"a", "b", "c"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ConfigEquals(config1, config2)
	}
}
