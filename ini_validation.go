// ini_validation.go: Validation functions for INI parser
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira System Libraries
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/agilira/go-errors"
)

// validateINISection validates INI section header format [section].
// Ensures proper bracket matching and valid section name syntax.
func validateINISection(line string, lineNum int) error {
	trimmed := strings.TrimSpace(line)

	// Check for proper bracket format
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid INI section at line %d: malformed brackets",
				lineNum))
	}

	// Check for nested brackets
	content := strings.Trim(trimmed, "[]")
	if strings.Contains(content, "[") || strings.Contains(content, "]") {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid INI section at line %d: nested brackets not supported",
				lineNum))
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

	// SECURITY FIX: Check for dangerous control characters including null bytes
	for _, char := range key {
		if char == '\x00' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid INI key at line %d: null byte not allowed in keys", lineNum))
		}
		// Block other dangerous control characters (except tab, LF, CR)
		if char < 32 && char != '\t' && char != '\n' && char != '\r' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid INI key at line %d: control character not allowed in keys", lineNum))
		}
		// Block non-printable characters (like DEL 0x7F)
		if !unicode.IsPrint(char) && char != '\t' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid INI key at line %d: non-printable character not allowed in keys", lineNum))
		}
	}

	// Check for whitespace in key (indicates potential parsing issue)
	if strings.TrimSpace(key) != key {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid INI key at line %d: key contains unexpected whitespace",
				lineNum))
	}

	return nil
}
