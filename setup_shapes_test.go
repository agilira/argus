// setup_shapes_test.go - the coverage test for the Setup API: six app shapes
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// These six files are the design of the Setup API written as code before the
// code exists. Each one is a real application shape, and the claim under test
// is that Argus costs it a single call:
//
//  1. setup_shape1_daemon_test.go      one file
//  2. setup_shape2_server_test.go      file + environment
//  3. setup_shape3_cli_test.go         flags + file + environment
//  4. setup_shape4_kubernetes_test.go  ConfigMap directory + environment
//  5. setup_shape5_agent_test.go       file + prompts/ + skills/, reload, audit
//  6. setup_shape6_embedded_test.go    inside somebody else's library
//
// They live in package argus_test on purpose: an external test package can
// only reach exported API, which is exactly the claim being made. Written
// inside the package they would compile against internals and prove nothing.

package argus_test

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFile creates path, and its parent directories, with the given contents.
func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
