// setup_shape3_cli_test.go - shape 3: a CLI with flags, a file and the environment
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus_test

import (
	"path/filepath"
	"testing"

	"github.com/agilira/argus"
	flashflags "github.com/agilira/flash-flags"
)

// TestShape3_CLI is the shape with the honest "one line" caveat: the program
// declares and parses its own flags, as it already does today, and Argus adds
// one call that puts them on top of the file and the environment.
//
// Flags arrive already parsed, through one adapter. Argus does not grow
// StringFlag/IntFlag/... of its own: that would drag help text, usage printing
// and os.Exit into a configuration facade.
func TestShape3_CLI(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	writeFile(t, config, `{"output": "yaml", "verbose": false, "retries": 3}`)

	t.Setenv("TOOL_OUTPUT", "xml")
	t.Setenv("TOOL_RETRIES", "5")

	flags := flashflags.New("tool")
	flags.String("output", "text", "output format")
	flags.Bool("verbose", false, "verbose output")
	if err := flags.Parse([]string{"--output", "json", "--verbose"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// The one line.
	settings, err := argus.Setup("tool").Flags(flags).File(config).Env("TOOL_").Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	// A flag the user actually set beats both.
	if got := settings.GetString("output"); got != "json" {
		t.Errorf("output = %q, want \"json\" from the command line", got)
	}
	if !settings.GetBool("verbose") {
		t.Error("verbose = false, want true from the command line")
	}
	// A flag the user did not set does not: the environment still outranks the
	// file, and the flag's own default is the floor, not a value.
	if got := settings.GetInt("retries"); got != 5 {
		t.Errorf("retries = %d, want 5 from the environment", got)
	}
}
