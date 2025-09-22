// simple_set_test.go: Testing Argus Simple Set Operations
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
)

func TestSimpleSet(t *testing.T) {
	config := NewConfigManager("test").
		StringFlag("host", "default", "Host")

	// Do not parse any flags
	err := config.Parse([]string{})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Default value
	if config.GetString("host") != "default" {
		t.Errorf("Expected default 'default', got %q", config.GetString("host"))
	}

	// Set explicit
	config.Set("host", "explicit")

	// Should take precedence
	if config.GetString("host") != "explicit" {
		t.Errorf("Expected explicit 'explicit', got %q", config.GetString("host"))
	}
}

func TestSetWithFlag(t *testing.T) {
	config := NewConfigManager("test").
		StringFlag("host", "default", "Host")

	// Parse with flag
	err := config.Parse([]string{"--host=from-flag"})
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Flag's value should take precedence
	if config.GetString("host") != "from-flag" {
		t.Errorf("Expected from flag 'from-flag', got %q", config.GetString("host"))
	}

	// Set explicit should take precedence
	config.Set("host", "explicit")

	if config.GetString("host") != "explicit" {
		t.Errorf("Expected explicit 'explicit', got %q", config.GetString("host"))
	}
}
