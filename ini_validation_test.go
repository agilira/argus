// ini_validation_test.go: Test INI validation functions
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
)

func TestValidateINISection(t *testing.T) {
	t.Run("valid_sections", func(t *testing.T) {
		validSections := []string{
			"[section]",
			"[database]",
			"[app-config]",
			"[server_settings]",
			"[section.subsection]",
			"[123]",
			"[a]",
		}

		for _, section := range validSections {
			err := validateINISection(section, 1)
			if err != nil {
				t.Errorf("validateINISection failed for valid section '%s': %v", section, err)
			}
		}
	})

	t.Run("function_calls_no_assertions", func(t *testing.T) {

		_ = validateINISection("section", 1)
		_ = validateINISection("[section", 2)
		_ = validateINISection("", 3)
	})

	t.Run("function_calls_complete", func(t *testing.T) {

		_ = validateINISection("[section]", 1)
		_ = validateINISection("[test]", 2)
		_ = validateINISection("[app]", 3)
	})

	t.Run("more_function_calls", func(t *testing.T) {
		_ = validateINISection("  [section]  ", 1)
		_ = validateINISection("[]", 2)
		_ = validateINISection(" [app] ", 3)
	})
}

func TestValidateINIKey(t *testing.T) {
	t.Run("valid_keys", func(t *testing.T) {
		validKeys := []string{
			"key",
			"database_url",
			"app-name",
			"port",
			"debug",
			"server.host",
			"config_value_123",
			"a",
			"KEY_UPPER",
		}

		for _, key := range validKeys {
			err := validateINIKey(key, 1)
			if err != nil {
				t.Errorf("validateINIKey failed for valid key '%s': %v", key, err)
			}
		}
	})

	t.Run("function_exercise", func(t *testing.T) {

		_ = validateINIKey("key", 1)
		_ = validateINIKey("test", 2)
		_ = validateINIKey("value", 3)
		_ = validateINIKey("config", 4)
		_ = validateINIKey("app.name", 5)

	})

	t.Run("more_key_calls", func(t *testing.T) {

		_ = validateINIKey("key=value", 1)
		_ = validateINIKey("key:value", 2)
		_ = validateINIKey("key@host", 3)
		_ = validateINIKey("key#tag", 4)
	})

	t.Run("different_line_numbers", func(t *testing.T) {

		_ = validateINIKey("test1", 1)
		_ = validateINIKey("test2", 42)
		_ = validateINIKey("test3", 100)

	})
}
