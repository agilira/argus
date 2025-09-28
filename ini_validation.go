// ini_validation.go: Validation functions for INI parser
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira System Libraries
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"strings"

	"github.com/agilira/go-errors"
)

// validateINISection validates INI section header format [section].
// Ensures proper bracket matching and valid section name syntax.
func validateINISection(line string, lineNum int) error {
	trimmed := strings.TrimSpace(line)

	// Check for proper bracket format
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid INI section at line %d: malformed brackets in '%s'",
				lineNum, trimmed))
	}

	// Check for nested brackets
	content := strings.Trim(trimmed, "[]")
	if strings.Contains(content, "[") || strings.Contains(content, "]") {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid INI section at line %d: nested brackets not supported in '%s'",
				lineNum, trimmed))
	}

	return nil
}

// validateINIKey validates that an INI key follows proper naming conventions.
// Keys must be non-empty and properly formatted.
func validateINIKey(key string, lineNum int) error {
	if key == "" {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid INI key at line %d: key cannot be empty", lineNum))
	}

	// Check for whitespace in key (indicates potential parsing issue)
	if strings.TrimSpace(key) != key {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid INI key at line %d: key '%s' contains unexpected whitespace",
				lineNum, key))
	}

	return nil
}
