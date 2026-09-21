// setup_shape6_embedded_test.go - shape 6: Argus inside somebody else's library
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus_test

import (
	"path/filepath"
	"testing"

	"github.com/agilira/argus"
)

// TestShape6_Embedded is a constraint check rather than a feature: a library
// that uses Argus internally must not take over the process. No flags, no
// os.Exit, no global singleton — two independent handles in one binary, each
// closing without touching the other.
func TestShape6_Embedded(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.json")
	second := filepath.Join(dir, "second.json")
	writeFile(t, first, `{"endpoint": "https://first.example"}`)
	writeFile(t, second, `{"endpoint": "https://second.example"}`)

	// Two libraries in the same process, neither aware of the other.
	a, err := argus.Setup("library-a").File(first).Start()
	if err != nil {
		t.Fatalf("Start a: %v", err)
	}
	b, err := argus.Setup("library-b").File(second).Start()
	if err != nil {
		t.Fatalf("Start b: %v", err)
	}

	if got := a.GetString("endpoint"); got != "https://first.example" {
		t.Errorf("a endpoint = %q", got)
	}
	if got := b.GetString("endpoint"); got != "https://second.example" {
		t.Errorf("b endpoint = %q", got)
	}

	// Closing one leaves the other running: no shared state to tear down.
	if err := a.Close(); err != nil {
		t.Fatalf("Close a: %v", err)
	}
	if got := b.GetString("endpoint"); got != "https://second.example" {
		t.Errorf("b endpoint after closing a = %q", got)
	}
	if err := b.Close(); err != nil {
		t.Fatalf("Close b: %v", err)
	}

	// Close is idempotent: a library's Close may be called twice by its own
	// caller, and that is not an error worth propagating.
	if err := b.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
