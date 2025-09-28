// properties_validation.go: Validation functions for Properties parser
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"strings"

	"github.com/agilira/go-errors"
)

// validatePropertiesKey validates that a Properties key follows Java Properties conventions.
// Keys must be non-empty and properly formatted according to Java Properties spec.
func validatePropertiesKey(key string, lineNum int) error {
	if key == "" {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid Properties key at line %d: key cannot be empty", lineNum))
	}

	// Check for whitespace in key (indicates potential parsing issue)
	if strings.TrimSpace(key) != key {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid Properties key at line %d: key '%s' contains unexpected whitespace",
				lineNum, key))
	}

	// Check for invalid characters that might indicate malformed syntax
	// Note: = and : are handled during parsing, don't need to check here
	if strings.Contains(key, "\n") || strings.Contains(key, "\r") {
		return errors.New(ErrCodeInvalidConfig,
			fmt.Sprintf("invalid Properties key at line %d: key '%s' contains line breaks",
				lineNum, key))
	}

	return nil
}
