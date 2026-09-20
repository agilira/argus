// main_test.go: the CLI entry point
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRun_ConfigGet exercises the command the README documents, end to end
// through the real entry point. No package main existed at all before this,
// so the published "CLI binaries" were library archives.
func TestRun_ConfigGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("server:\n  port: 8080\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if code := run([]string{"config", "get", path, "server.port"}, os.Stderr); code != 0 {
		t.Errorf("run(config get) = %d, want 0", code)
	}
}

// TestRun_UnknownCommandFails: a failure has to reach the exit status, or a
// script driving the CLI cannot tell success from failure.
func TestRun_UnknownCommandFails(t *testing.T) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = devNull.Close() }()

	if code := run([]string{"no-such-command"}, devNull); code == 0 {
		t.Error("run(no-such-command) = 0, want a non-zero exit status")
	}
}

// TestRun_HelpSucceeds: asking for usage is not a failure. Orpheus reports it
// as an error, so the entry point has to tell the two apart.
func TestRun_HelpSucceeds(t *testing.T) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer func() { _ = devNull.Close() }()

	if code := run([]string{"--help"}, devNull); code != 0 {
		t.Errorf("run(--help) = %d, want 0", code)
	}
}
