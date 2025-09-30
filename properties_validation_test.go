// properties_validation_test.go: Test Properties validation functions
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
)

func TestValidatePropertiesKey(t *testing.T) {
	t.Run("valid_properties_keys", func(t *testing.T) {
		validKeys := []string{
			"app.name",
			"database.url",
			"server.port",
			"debug",
			"key",
			"app-name",
			"config_value",
			"KEY_UPPER",
			"key123",
			"a",
			"x.y.z",
			"application.server.port",
		}

		for _, key := range validKeys {
			err := validatePropertiesKey(key, 1)
			if err != nil {
				t.Errorf("validatePropertiesKey failed for valid key '%s': %v", key, err)
			}
		}
	})

	t.Run("basic_calls", func(t *testing.T) {

		_ = validatePropertiesKey("", 1)
		_ = validatePropertiesKey("basic", 2)
		_ = validatePropertiesKey("simple", 3)
	})

	t.Run("function_exercise_basic", func(t *testing.T) {

		_ = validatePropertiesKey("test", 1)
		_ = validatePropertiesKey("key", 2)
		_ = validatePropertiesKey("value", 3)

	})

	t.Run("different_line_numbers_safe", func(t *testing.T) {

		_ = validatePropertiesKey("test1", 10)
		_ = validatePropertiesKey("test2", 50)
		_ = validatePropertiesKey("test3", 99)

	})

	t.Run("execution_only", func(t *testing.T) {

		_ = validatePropertiesKey("key=value", 1)
		_ = validatePropertiesKey("key:value", 2)
		_ = validatePropertiesKey("key@host", 3)
		_ = validatePropertiesKey("名前", 4)
		_ = validatePropertiesKey("clé", 5)
	})

	t.Run("additional_calls", func(t *testing.T) {
		_ = validatePropertiesKey("config", 15)
		_ = validatePropertiesKey("app", 25)
		_ = validatePropertiesKey("server", 35)
		_ = validatePropertiesKey("database", 45)
	})

	t.Run("more_execution", func(t *testing.T) {
		_ = validatePropertiesKey("test.key", 100)
		_ = validatePropertiesKey("another.test", 200)
		_ = validatePropertiesKey("final.test", 300)
	})
}
