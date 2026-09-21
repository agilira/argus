// setup_shape2_server_test.go - shape 2: a server configured by file and environment
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

// TestShape2_Server is the twelve-factor shape: the image ships a config file
// with sane values and the deployment overrides a few of them from the
// environment. It declares no flags, which today means it sees no environment
// at all — that hole is the reason this shape is in the coverage test.
func TestShape2_Server(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.yaml")
	writeFile(t, config, "port: 8080\ndatabase_url: postgres://localhost/dev\n")

	t.Setenv("SERVER_PORT", "9090")
	t.Setenv("SERVER_DATABASE_URL", "postgres://prod/app")

	// The one line.
	settings, err := argus.Setup("server").File(config).Env("SERVER_").Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	// Environment outranks the file.
	if got := settings.GetInt("port"); got != 9090 {
		t.Errorf("port = %d, want 9090 from the environment", got)
	}
	if got := settings.GetString("database_url"); got != "postgres://prod/app" {
		t.Errorf("database_url = %q, want the environment value", got)
	}

	// And Explain says so, rather than leaving the operator guessing which of
	// the two won.
	if src := settings.Explain().KeySource("port"); src != argus.SourceEnv {
		t.Errorf("KeySource(port) = %v, want %v", src, argus.SourceEnv)
	}
}
