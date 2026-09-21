// setup_shape1_daemon_test.go - shape 1: a daemon with one configuration file
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

// TestShape1_Daemon is the smallest shape there is: one process, one JSON file,
// reloaded while it runs. Everything Argus needs is in the path.
func TestShape1_Daemon(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	writeFile(t, config, `{"port": 8080, "log_level": "info", "timeout": "30s"}`)

	// The one line.
	settings, err := argus.Setup("daemon").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		if err := settings.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	if got := settings.GetInt("port"); got != 8080 {
		t.Errorf("port = %d, want 8080", got)
	}
	if got := settings.GetString("log_level"); got != "info" {
		t.Errorf("log_level = %q, want \"info\"", got)
	}
	if got := settings.GetDuration("timeout").String(); got != "30s" {
		t.Errorf("timeout = %s, want 30s", got)
	}

	// A key nobody supplied reads as the zero value, not as a panic.
	if got := settings.GetString("nothing_here"); got != "" {
		t.Errorf("missing key = %q, want empty", got)
	}

	// The first successful load is a revision, so an application can log which
	// one it came up on.
	if settings.Revision() == 0 {
		t.Error("Revision = 0 after a successful Start")
	}
}
