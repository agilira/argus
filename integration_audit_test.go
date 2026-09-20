// integration_audit_test.go: regression tests for ConfigManager defects found in audit
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfigManager_EnvVarIsApplied verifies the documented behaviour
// "port := config.GetInt("port") // Command-line, env var, or default".
//
// ConfigManager.Parse calls flags.Parse() BEFORE loadEnvironmentVariables()
// sets the env prefix, and flash-flags only reads the environment from inside
// Parse. The prefix therefore arrives after the only chance to use it.
func TestConfigManager_EnvVarIsApplied(t *testing.T) {
	t.Setenv("MYAPP_PORT", "9999")

	cm := NewConfigManager("myapp").IntFlag("port", 8080, "Server port")
	if err := cm.Parse([]string{}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := cm.GetInt("port"); got != 9999 {
		t.Errorf("GetInt(port) = %d, want 9999 from MYAPP_PORT", got)
	}
}

// TestConfigManager_CommandLineBeatsEnv guards the precedence order once the
// environment is actually read: an explicit flag must still win.
func TestConfigManager_CommandLineBeatsEnv(t *testing.T) {
	t.Setenv("MYAPP_PORT", "9999")

	cm := NewConfigManager("myapp").IntFlag("port", 8080, "Server port")
	if err := cm.Parse([]string{"--port", "1234"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := cm.GetInt("port"); got != 1234 {
		t.Errorf("GetInt(port) = %d, want 1234 from the command line", got)
	}
}

// TestConfigManager_LoadConfigFile actually loads a file.
// LoadConfigFile is a stub returning nil, so WatchConfigFile reloads nothing.
func TestConfigManager_LoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"port": 7777, "debug": true}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cm := NewConfigManager("myapp").
		IntFlag("port", 8080, "Server port").
		BoolFlag("debug", false, "Debug mode")

	if err := cm.LoadConfigFile(path); err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}

	if got := cm.GetInt("port"); got != 7777 {
		t.Errorf("GetInt(port) = %d, want 7777 from the config file", got)
	}
	if !cm.GetBool("debug") {
		t.Error("GetBool(debug) = false, want true from the config file")
	}
}

// TestConfigManager_LoadConfigFile_Missing: a path that does not exist is an
// error, not silence. The stub swallows every failure.
func TestConfigManager_LoadConfigFile_Missing(t *testing.T) {
	cm := NewConfigManager("myapp").IntFlag("port", 8080, "Server port")
	if err := cm.LoadConfigFile(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Error("LoadConfigFile on a missing path returned nil, want an error")
	}
}

// TestConfigManager_SetDefault honours its own doc comment: "sets a default
// configuration value (lowest precedence)". Today it is an empty body.
func TestConfigManager_SetDefault(t *testing.T) {
	cm := NewConfigManager("myapp")
	cm.SetDefault("timeout", 30)

	if got := cm.GetInt("timeout"); got != 30 {
		t.Errorf("GetInt(timeout) = %d, want 30 from SetDefault", got)
	}
}

// TestConfigManager_SetDefault_LosesToFlag: a registered flag outranks a default.
func TestConfigManager_SetDefault_LosesToFlag(t *testing.T) {
	cm := NewConfigManager("myapp").IntFlag("port", 8080, "Server port")
	cm.SetDefault("port", 30)

	if got := cm.GetInt("port"); got != 8080 {
		t.Errorf("GetInt(port) = %d, want the flag default 8080 to win over SetDefault", got)
	}
}

// TestConfigManager_HelpRequestedIsDetectable: ParseArgsOrExit compares
// err.Error() to the bare string "help requested", but go-errors renders
// "[ARGUS_INVALID_CONFIG]: help requested", so the comparison never matches
// and --help takes the failure branch (stderr + exit 1).
func TestConfigManager_HelpRequestedIsDetectable(t *testing.T) {
	cm := NewConfigManager("myapp").IntFlag("port", 8080, "Server port")
	err := cm.Parse([]string{"--help"})
	if err == nil {
		t.Fatal("Parse(--help) returned nil, want a help sentinel")
	}
	if !IsHelpRequested(err) {
		t.Errorf("IsHelpRequested(%q) = false, want true", err.Error())
	}

	// go-errors matches errors.Is on the code alone, so the sentinel must not
	// share a code with ordinary configuration failures.
	other := cm.Parse([]string{"--port", "not-a-number"})
	if other == nil {
		t.Fatal("Parse(--port not-a-number) returned nil, want a parse error")
	}
	if IsHelpRequested(other) {
		t.Errorf("IsHelpRequested(%q) = true; an ordinary parse error is not a help request", other.Error())
	}
}

// TestConfigManager_ApplicationOwnsHelpFlag: an application that registers its
// own --help must reach it. Parse intercepts the name before any lookup.
func TestConfigManager_ApplicationOwnsHelpFlag(t *testing.T) {
	cm := NewConfigManager("myapp").StringFlag("help", "", "Topic to explain")
	if err := cm.Parse([]string{"--help", "networking"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cm.GetString("help"); got != "networking" {
		t.Errorf("GetString(help) = %q, want %q", got, "networking")
	}
}
