// properties_validation.go: Validation functions for Properties parser
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/agilira/go-errors"
)

// validatePropertiesKey validates that a Properties key follows Java Properties conventions.
// Keys must be non-empty and properly formatted according to Java Properties spec.
func validatePropertiesKey(key string, lineNum int) error {
	if key == "" {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid Properties key at line %d: key cannot be empty", lineNum))
	}

	// SECURITY FIX: Check for dangerous control characters including null bytes
	for _, char := range key {
		if char == '\x00' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid Properties key at line %d: null byte not allowed in keys", lineNum))
		}
		// Block other dangerous control characters (except tab, LF, CR)
		if char < 32 && char != '\t' && char != '\n' && char != '\r' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid Properties key at line %d: control character not allowed in keys", lineNum))
		}
		// Block non-printable characters (like DEL 0x7F)
		if !unicode.IsPrint(char) && char != '\t' {
			return errors.New(ErrCodeInvalidConfig,
				fmt.Sprintf("invalid Properties key at line %d: non-printable character not allowed in keys", lineNum))
		}
	}

	// Check for whitespace in key (indicates potential parsing issue)
	if strings.TrimSpace(key) != key {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid Properties key at line %d: key contains unexpected whitespace",
				lineNum))
	}

	// Check for invalid characters that might indicate malformed syntax
	// Note: = and : are handled during parsing, don't need to check here
	if strings.Contains(key, "\n") || strings.Contains(key, "\r") {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid Properties key at line %d: key contains line breaks",
				lineNum))
	}

	return nil
}
