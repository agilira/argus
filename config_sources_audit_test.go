// config_sources_audit_test.go: regression tests for the multi-source, parser
// and binder defects found in audit
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// argusEnvVars is every environment variable the configuration loader reads.
var argusEnvVars = []string{
	"ARGUS_POLL_INTERVAL", "ARGUS_CACHE_TTL", "ARGUS_MAX_WATCHED_FILES",
	"ARGUS_OPTIMIZATION_STRATEGY", "ARGUS_BOREAS_CAPACITY",
	"ARGUS_AUDIT_ENABLED", "ARGUS_ALLOW_AUDIT_DISABLE", "ARGUS_AUDIT_OUTPUT_FILE",
	"ARGUS_AUDIT_MIN_LEVEL", "ARGUS_AUDIT_BUFFER_SIZE", "ARGUS_AUDIT_FLUSH_INTERVAL",
	"ARGUS_AUDIT_DIR",
	"ARGUS_REMOTE_URL", "ARGUS_REMOTE_INTERVAL", "ARGUS_REMOTE_TIMEOUT", "ARGUS_REMOTE_HEADERS",
	"ARGUS_VALIDATION_ENABLED", "ARGUS_VALIDATION_SCHEMA", "ARGUS_VALIDATION_STRICT",
}

// clearArgusEnv unsets every ARGUS_* variable for the duration of the test and
// restores the previous values afterwards.
//
// Tests that assert on values coming from a configuration file need this: the
// environment layer legitimately outranks the file, so a variable leaked by an
// earlier test silently changes the result. t.Setenv gives the restore for
// free, which is why it is used here instead of os.Setenv.
func clearArgusEnv(t *testing.T) {
	t.Helper()
	for _, name := range argusEnvVars {
		if _, present := os.LookupEnv(name); present {
			t.Setenv(name, "")
			if err := os.Unsetenv(name); err != nil {
				t.Fatalf("failed to unset %s: %v", name, err)
			}
		}
	}
}

// TestLoadConfigMultiSource_ReadsTheFile: the documented precedence is
// "environment > configuration file > defaults", but loadConfigFromFile parses
// the file, discards the result and returns bare defaults. The file layer is a
// no-op with a standing TODO, so the middle of that precedence chain is absent.
func TestLoadConfigMultiSource_ReadsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "argus.json")
	body := `{"poll_interval": "3s", "max_watched_files": 42}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := LoadConfigMultiSource(path)
	if err != nil {
		t.Fatalf("LoadConfigMultiSource: %v", err)
	}

	if cfg.MaxWatchedFiles != 42 {
		t.Errorf("MaxWatchedFiles = %d, want 42 from the config file", cfg.MaxWatchedFiles)
	}
	if cfg.PollInterval != 3*time.Second {
		t.Errorf("PollInterval = %v, want 3s from the config file", cfg.PollInterval)
	}
}

// TestLoadConfigMultiSource_EnvBeatsFile locks the precedence once the file is
// actually read.
func TestLoadConfigMultiSource_EnvBeatsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "argus.json")
	if err := os.WriteFile(path, []byte(`{"max_watched_files": 42}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("ARGUS_MAX_WATCHED_FILES", "7")

	cfg, err := LoadConfigMultiSource(path)
	if err != nil {
		t.Fatalf("LoadConfigMultiSource: %v", err)
	}
	if cfg.MaxWatchedFiles != 7 {
		t.Errorf("MaxWatchedFiles = %d, want 7 from the environment", cfg.MaxWatchedFiles)
	}
}

// TestLoadConfigMultiSource_MalformedFileIsReported: a syntax error in the
// config file currently falls through to defaults in silence, so a typo starts
// the application on settings nobody chose.
func TestLoadConfigMultiSource_MalformedFileIsReported(t *testing.T) {
	path := filepath.Join(t.TempDir(), "argus.json")
	if err := os.WriteFile(path, []byte(`{"max_watched_files": `), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := LoadConfigMultiSource(path); err == nil {
		t.Error("LoadConfigMultiSource on a malformed file returned nil error")
	}
}

// TestEnvConfig_LightStrategy: OptimizationLight is a documented public
// strategy, but the environment loader's allow-list and the converter both
// predate it, so it cannot be selected from the environment.
func TestEnvConfig_LightStrategy(t *testing.T) {
	t.Setenv("ARGUS_OPTIMIZATION_STRATEGY", "light")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	if cfg.OptimizationStrategy != OptimizationLight {
		t.Errorf("OptimizationStrategy = %v, want OptimizationLight", cfg.OptimizationStrategy)
	}
}

// TestEnvConfig_InvalidRemoteDurationIsRejected: every other ARGUS_* duration
// returns a typed error on a bad value; the remote ones swallow it.
func TestEnvConfig_InvalidRemoteDurationIsRejected(t *testing.T) {
	t.Setenv("ARGUS_REMOTE_TIMEOUT", "not-a-duration")

	if _, err := LoadConfigFromEnv(); err == nil {
		t.Error("LoadConfigFromEnv accepted ARGUS_REMOTE_TIMEOUT=not-a-duration")
	}
}

// TestConfigBinder_FlatDottedKey: the Properties and INI parsers emit flat keys
// that contain dots ("database.host"). getValue sees the dot, decides the key
// is a nested path, and never tries the direct lookup — so binding a
// .properties file silently yields defaults for every key.
func TestConfigBinder_FlatDottedKey(t *testing.T) {
	cfg, err := ParseConfig([]byte("database.host=db.internal\ndatabase.port=5432\n"), FormatProperties)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}

	var host string
	var port int
	if err := BindFromConfig(cfg).
		BindString(&host, "database.host", "localhost").
		BindInt(&port, "database.port", 1).
		Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if host != "db.internal" {
		t.Errorf("host = %q, want %q", host, "db.internal")
	}
	if port != 5432 {
		t.Errorf("port = %d, want 5432", port)
	}
}

// TestParseConfig_ConcurrentRegisterParser: ParseConfig reads the customParsers
// slice header outside the mutex on its fast path, claiming in a comment that
// append-only makes this safe. Appending rewrites the slice header, so this is
// an unsynchronised read of a word being written. Run with -race.
func TestParseConfig_ConcurrentRegisterParser(t *testing.T) {
	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_, _ = ParseConfig([]byte(`{"a":1}`), FormatJSON)
			}
		}
	}()

	for i := 0; i < 50; i++ {
		RegisterParser(noopParser{})
	}
	close(stop)
	wg.Wait()
}

type noopParser struct{}

func (noopParser) Parse(data []byte) (map[string]interface{}, error) { return nil, nil }
func (noopParser) Supports(format ConfigFormat) bool                 { return false }
func (noopParser) Name() string                                      { return "noop" }

// TestEnvConfig_AuditVarsApplyWithoutEnabledFlag: the audit conversion used to
// be gated on ARGUS_AUDIT_ENABLED, so an operator who set only the buffer size
// or the minimum level had their setting silently dropped.
func TestEnvConfig_AuditVarsApplyWithoutEnabledFlag(t *testing.T) {
	t.Setenv("ARGUS_AUDIT_MIN_LEVEL", "security")
	t.Setenv("ARGUS_AUDIT_BUFFER_SIZE", "250")

	cfg, err := LoadConfigFromEnv()
	if err != nil {
		t.Fatalf("LoadConfigFromEnv: %v", err)
	}
	if cfg.Audit.MinLevel != AuditSecurity {
		t.Errorf("Audit.MinLevel = %v, want AuditSecurity from ARGUS_AUDIT_MIN_LEVEL", cfg.Audit.MinLevel)
	}
	if cfg.Audit.BufferSize != 250 {
		t.Errorf("Audit.BufferSize = %d, want 250 from ARGUS_AUDIT_BUFFER_SIZE", cfg.Audit.BufferSize)
	}
}

// TestLoadConfigMultiSource_NestedAndFlatKeys: a key may be written flat
// ("audit.buffer_size", what Properties and INI produce) or nested (what JSON,
// YAML and TOML produce). Both must reach the same field.
func TestLoadConfigMultiSource_NestedAndFlatKeys(t *testing.T) {
	dir := t.TempDir()

	nested := filepath.Join(dir, "nested.json")
	if err := os.WriteFile(nested, []byte(`{"audit":{"buffer_size":321}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	flat := filepath.Join(dir, "flat.properties")
	if err := os.WriteFile(flat, []byte("audit.buffer_size=321\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	for _, path := range []string{nested, flat} {
		cfg, err := LoadConfigMultiSource(path)
		if err != nil {
			t.Fatalf("LoadConfigMultiSource(%s): %v", filepath.Base(path), err)
		}
		if cfg.Audit.BufferSize != 321 {
			t.Errorf("%s: Audit.BufferSize = %d, want 321", filepath.Base(path), cfg.Audit.BufferSize)
		}
	}
}
