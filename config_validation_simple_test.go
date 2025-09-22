// config_validation_simple_test.go - Simple validation tests in main package
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"errors"
	"testing"
	"time"
)

// Test basic validation functionality
func TestConfig_BasicValidation(t *testing.T) {
	t.Parallel()

	// Test valid config
	validConfig := Config{
		PollInterval:    5 * time.Second,
		CacheTTL:        2 * time.Second,
		MaxWatchedFiles: 100,
	}

	if err := validConfig.Validate(); err != nil {
		t.Errorf("Valid config failed validation: %v", err)
	}

	// Test invalid poll interval
	invalidConfig := Config{
		PollInterval:    0, // Invalid
		CacheTTL:        2 * time.Second,
		MaxWatchedFiles: 100,
	}

	err := invalidConfig.Validate()
	if err == nil {
		t.Error("Invalid config should have failed validation")
	}

	// Check error code
	code := GetValidationErrorCode(err)
	if code != "ARGUS_INVALID_POLL_INTERVAL" {
		t.Errorf("Expected ARGUS_INVALID_POLL_INTERVAL, got %s", code)
	}
}

// Test validation error detection
func TestValidationErrorDetection(t *testing.T) {
	t.Parallel()

	// Test with validation error
	if !IsValidationError(ErrInvalidPollInterval) {
		t.Error("Should detect validation error")
	}

	// Test with non-validation error
	err := errors.New("some other error")
	if IsValidationError(err) {
		t.Error("Should not detect validation error")
	}

	// Test with nil
	if IsValidationError(nil) {
		t.Error("Nil should not be validation error")
	}
}
