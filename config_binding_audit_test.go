// config_binding_audit_test.go: the configuration-file binding, the deep copy
// and the ConfigManager accessors added during the audit
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// TestBindConfigMap_EveryDocumentedKey: every key bindConfigMap claims to
// support must reach its field. A key that silently does nothing is the defect
// this whole layer was fixed for.
func TestBindConfigMap_EveryDocumentedKey(t *testing.T) {
	configMap := map[string]interface{}{
		"poll_interval":         "3s",
		"cache_ttl":             "1s",
		"max_watched_files":     42,
		"boreas_capacity":       256,
		"disable_audit":         true,
		"optimization_strategy": "light",
		"audit": map[string]interface{}{
			"enabled":        true,
			"output_file":    "/var/log/argus/audit.db",
			"min_level":      "security",
			"buffer_size":    77,
			"flush_interval": "9s",
			"include_stack":  true,
		},
		"remote": map[string]interface{}{
			"enabled":       true,
			"primary_url":   "consul://localhost:8500/config/app",
			"fallback_url":  "consul://backup:8500/config/app",
			"fallback_path": "/etc/app/fallback.json",
			"sync_interval": "45s",
			"timeout":       "11s",
			"max_retries":   4,
			"retry_delay":   "2s",
		},
	}

	var config Config
	if err := bindConfigMap(configMap, &config); err != nil {
		t.Fatalf("bindConfigMap: %v", err)
	}

	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"PollInterval", config.PollInterval, 3 * time.Second},
		{"CacheTTL", config.CacheTTL, time.Second},
		{"MaxWatchedFiles", config.MaxWatchedFiles, 42},
		{"BoreasLiteCapacity", config.BoreasLiteCapacity, int64(256)},
		{"DisableAudit", config.DisableAudit, true},
		{"OptimizationStrategy", config.OptimizationStrategy, OptimizationLight},
		{"Audit.Enabled", config.Audit.Enabled, true},
		{"Audit.OutputFile", config.Audit.OutputFile, "/var/log/argus/audit.db"},
		{"Audit.MinLevel", config.Audit.MinLevel, AuditSecurity},
		{"Audit.BufferSize", config.Audit.BufferSize, 77},
		{"Audit.FlushInterval", config.Audit.FlushInterval, 9 * time.Second},
		{"Audit.IncludeStack", config.Audit.IncludeStack, true},
		{"Remote.Enabled", config.Remote.Enabled, true},
		{"Remote.PrimaryURL", config.Remote.PrimaryURL, "consul://localhost:8500/config/app"},
		{"Remote.FallbackURL", config.Remote.FallbackURL, "consul://backup:8500/config/app"},
		{"Remote.FallbackPath", config.Remote.FallbackPath, "/etc/app/fallback.json"},
		{"Remote.SyncInterval", config.Remote.SyncInterval, 45 * time.Second},
		{"Remote.Timeout", config.Remote.Timeout, 11 * time.Second},
		{"Remote.MaxRetries", config.Remote.MaxRetries, 4},
		{"Remote.RetryDelay", config.Remote.RetryDelay, 2 * time.Second},
	}

	for _, check := range checks {
		if !reflect.DeepEqual(check.got, check.want) {
			t.Errorf("%s = %v, want %v", check.name, check.got, check.want)
		}
	}
}

// TestBindConfigMap_RejectsWrongTypes: a value of the wrong shape is reported,
// not quietly ignored and not coerced into a surprise.
func TestBindConfigMap_RejectsWrongTypes(t *testing.T) {
	cases := []struct {
		name      string
		configMap map[string]interface{}
	}{
		{"duration", map[string]interface{}{"poll_interval": "not-a-duration"}},
		{"int", map[string]interface{}{"max_watched_files": "not-a-number"}},
		{"int64", map[string]interface{}{"boreas_capacity": "not-a-number"}},
		{"bool", map[string]interface{}{"disable_audit": "not-a-bool"}},
		{"strategy", map[string]interface{}{"optimization_strategy": "turbo"}},
		{"audit level", map[string]interface{}{"audit.min_level": "shouty"}},
		{"nested duration", map[string]interface{}{"remote.timeout": "soon"}},
		{"nested int", map[string]interface{}{"remote.max_retries": "many"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var config Config
			if err := bindConfigMap(tc.configMap, &config); err == nil {
				t.Errorf("bindConfigMap(%v) returned nil, want an error", tc.configMap)
			}
		})
	}
}

// TestBindConfigMap_AbsentKeysLeaveZeroValues: an absent key must stay at the
// zero value so WithDefaults and the environment layer can still fill it.
// Writing a default here would make the file look like it set everything.
func TestBindConfigMap_AbsentKeysLeaveZeroValues(t *testing.T) {
	var config Config
	if err := bindConfigMap(map[string]interface{}{"max_watched_files": 5}, &config); err != nil {
		t.Fatalf("bindConfigMap: %v", err)
	}

	if config.PollInterval != 0 {
		t.Errorf("PollInterval = %v, want the zero value for a key the file did not set", config.PollInterval)
	}
	if config.Audit.BufferSize != 0 {
		t.Errorf("Audit.BufferSize = %d, want the zero value", config.Audit.BufferSize)
	}
}

// TestCopyMap_IsDeep: the audit trail compares a before snapshot with an after
// one. A one-level copy shares every nested container, so the snapshot changes
// under it and the record compares a value against itself.
func TestCopyMap_IsDeep(t *testing.T) {
	original := map[string]interface{}{
		"scalar": 1,
		"nested": map[string]interface{}{
			"level": "info",
			"deep":  map[string]interface{}{"key": "value"},
		},
		"list":    []interface{}{"a", map[string]interface{}{"k": "v"}},
		"strings": []string{"x", "y"},
	}

	snapshot := copyMap(original)

	// Mutate every container in the original.
	original["nested"].(map[string]interface{})["level"] = "debug"
	original["nested"].(map[string]interface{})["deep"].(map[string]interface{})["key"] = "changed"
	original["list"].([]interface{})[0] = "z"
	original["list"].([]interface{})[1].(map[string]interface{})["k"] = "changed"
	original["strings"].([]string)[0] = "changed"

	nested := snapshot["nested"].(map[string]interface{})
	if nested["level"] != "info" {
		t.Errorf("nested.level = %v, want the snapshot to be independent", nested["level"])
	}
	if nested["deep"].(map[string]interface{})["key"] != "value" {
		t.Error("a map nested two levels down is shared with the original")
	}
	list := snapshot["list"].([]interface{})
	if list[0] != "a" {
		t.Errorf("list[0] = %v, want %q", list[0], "a")
	}
	if list[1].(map[string]interface{})["k"] != "v" {
		t.Error("a map inside a slice is shared with the original")
	}
	if snapshot["strings"].([]string)[0] != "x" {
		t.Error("a []string is shared with the original")
	}
}

// TestCopyMap_Nil keeps the nil contract.
func TestCopyMap_Nil(t *testing.T) {
	if copyMap(nil) != nil {
		t.Error("copyMap(nil) should stay nil")
	}
}

// TestConfigManager_AllAccessorsFollowPrecedence exercises every typed getter
// against every layer, which is the part of the fix most likely to rot.
func TestConfigManager_AllAccessorsFollowPrecedence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{
		"host": "from-file",
		"port": 2222,
		"debug": true,
		"timeout": "2s",
		"ratio": 2.5,
		"peers": ["a", "b"]
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cm := NewConfigManager("myapp").
		StringFlag("host", "default-host", "Host").
		IntFlag("port", 1111, "Port").
		BoolFlag("debug", false, "Debug").
		DurationFlag("timeout", time.Second, "Timeout").
		Float64Flag("ratio", 1.5, "Ratio").
		StringSliceFlag("peers", []string{"z"}, "Peers")

	if err := cm.Parse([]string{}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Layer: flag defaults only.
	if got := cm.GetString("host"); got != "default-host" {
		t.Errorf("GetString(host) = %q, want the flag default", got)
	}

	// Layer: configuration file.
	if err := cm.LoadConfigFile(path); err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}
	if got := cm.GetString("host"); got != "from-file" {
		t.Errorf("GetString(host) = %q, want %q", got, "from-file")
	}
	if got := cm.GetInt("port"); got != 2222 {
		t.Errorf("GetInt(port) = %d, want 2222", got)
	}
	if !cm.GetBool("debug") {
		t.Error("GetBool(debug) = false, want true from the file")
	}
	if got := cm.GetDuration("timeout"); got != 2*time.Second {
		t.Errorf("GetDuration(timeout) = %v, want 2s", got)
	}
	if got := cm.GetFloat64("ratio"); got != 2.5 {
		t.Errorf("GetFloat64(ratio) = %v, want 2.5", got)
	}
	if got := cm.GetStringSlice("peers"); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("GetStringSlice(peers) = %v, want [a b]", got)
	}

	// Layer: explicit Set outranks everything.
	cm.Set("host", "from-set")
	cm.Set("port", 3333)
	cm.Set("ratio", 9.5)
	cm.Set("peers", []string{"q"})
	if got := cm.GetString("host"); got != "from-set" {
		t.Errorf("GetString(host) = %q, want %q", got, "from-set")
	}
	if got := cm.GetInt("port"); got != 3333 {
		t.Errorf("GetInt(port) = %d, want 3333", got)
	}
	if got := cm.GetFloat64("ratio"); got != 9.5 {
		t.Errorf("GetFloat64(ratio) = %v, want 9.5", got)
	}
	if got := cm.GetStringSlice("peers"); !reflect.DeepEqual(got, []string{"q"}) {
		t.Errorf("GetStringSlice(peers) = %v, want [q]", got)
	}
}

// TestConfigManager_CommandLineOutranksFile pins the middle of the order.
func TestConfigManager_CommandLineOutranksFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"port": 2222}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cm := NewConfigManager("myapp").IntFlag("port", 1111, "Port")
	if err := cm.Parse([]string{"--port", "4444"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := cm.LoadConfigFile(path); err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}

	if got := cm.GetInt("port"); got != 4444 {
		t.Errorf("GetInt(port) = %d, want 4444 from the command line", got)
	}
}

// TestConfigManager_ConcurrentAccess: WatchConfigFile reloads from the
// watcher's goroutine while the application reads. Run with -race.
func TestConfigManager_ConcurrentAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"port": 2222}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cm := NewConfigManager("myapp").IntFlag("port", 1111, "Port")
	if err := cm.Parse([]string{}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = cm.LoadConfigFile(path)
			cm.Set("other", i)
			cm.SetDefault("fallback", i)
		}
	}()

	for i := 0; i < 200; i++ {
		_ = cm.GetInt("port")
		_ = cm.GetString("host")
		_ = cm.GetInt("other")
		_ = cm.GetInt("fallback")
	}
	<-done
}
