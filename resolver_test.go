// resolver_test.go - table-driven tests for the six-layer precedence resolver
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"strings"
	"sync"
	"testing"

	flashflags "github.com/agilira/flash-flags"
)

// setFile replaces the file layer, failing the test if the swap is refused.
func setFile(t *testing.T, r *resolver, values map[string]interface{}) {
	t.Helper()
	if err := r.setFile(values); err != nil {
		t.Fatalf("setFile: %v", err)
	}
}

// setRemote replaces the remote layer, failing the test if the swap is
// refused.
func setRemote(t *testing.T, r *resolver, values map[string]interface{}) {
	t.Helper()
	if err := r.setRemote(values); err != nil {
		t.Fatalf("setRemote: %v", err)
	}
}

// newTestFlags returns a parsed FlagSet with one flag of each kind we care
// about: "port" is left alone so it contributes only its declared default,
// "mode" is set on the command line so it contributes a value.
func newTestFlags(t *testing.T, args ...string) *flashflags.FlagSet {
	t.Helper()
	fs := flashflags.New("test")
	fs.Int("port", 1111, "port")
	fs.String("mode", "flag-default", "mode")
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse(%v): %v", args, err)
	}
	return fs
}

// TestResolver_Precedence walks every layer from the top down: each case
// switches on the layers above the one under test and asserts that the value
// comes from the highest one present, and that the resolver says so.
func TestResolver_Precedence(t *testing.T) {
	t.Setenv("RES_KEY", "from-env")

	build := func(t *testing.T, layers ...string) *resolver {
		t.Helper()
		r := newResolver()
		for _, layer := range layers {
			switch layer {
			case "override":
				r.setOverride("key", "from-override")
			case "flag":
				r.useFlags(newTestFlags(t, "--mode", "from-flag"))
			case "flagdefault":
				r.useFlags(newTestFlags(t))
			case "env":
				r.useEnv("RES_")
			case "file":
				setFile(t, r, map[string]interface{}{"key": "from-file"})
			case "remote":
				setRemote(t, r, map[string]interface{}{"key": "from-remote"})
			case "default":
				r.setDefault("key", "from-default")
			}
		}
		return r
	}

	tests := []struct {
		name       string
		layers     []string
		key        string
		wantValue  interface{}
		wantSource Source
		wantFlag   bool
	}{
		{
			name:       "override beats everything",
			layers:     []string{"default", "remote", "file", "env", "flag", "override"},
			key:        "key",
			wantValue:  "from-override",
			wantSource: SourceOverride,
		},
		{
			name:       "a flag that was set beats env, file, remote and defaults",
			layers:     []string{"default", "remote", "file", "env", "flag"},
			key:        "mode",
			wantSource: SourceFlag,
			wantFlag:   true,
		},
		{
			name:       "env beats file, remote and defaults",
			layers:     []string{"default", "remote", "file", "env"},
			key:        "key",
			wantValue:  "from-env",
			wantSource: SourceEnv,
		},
		{
			name:       "env needs no declared flag",
			layers:     []string{"env"},
			key:        "key",
			wantValue:  "from-env",
			wantSource: SourceEnv,
		},
		{
			name:       "file beats remote and defaults",
			layers:     []string{"default", "remote", "file"},
			key:        "key",
			wantValue:  "from-file",
			wantSource: SourceFile,
		},
		{
			name:       "remote beats defaults",
			layers:     []string{"default", "remote"},
			key:        "key",
			wantValue:  "from-remote",
			wantSource: SourceRemote,
		},
		{
			name:       "a declared flag default beats an explicit default",
			layers:     []string{"default", "flagdefault"},
			key:        "port",
			wantSource: SourceFlag,
			wantFlag:   true,
		},
		{
			name:       "a declared flag default sits below the file",
			layers:     []string{"flagdefault", "file"},
			key:        "key",
			wantValue:  "from-file",
			wantSource: SourceFile,
		},
		{
			name:       "defaults are the floor",
			layers:     []string{"default"},
			key:        "key",
			wantValue:  "from-default",
			wantSource: SourceDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := build(t, tt.layers...).resolve(tt.key)
			if !got.found {
				t.Fatalf("resolve(%q) found nothing", tt.key)
			}
			if got.source != tt.wantSource {
				t.Errorf("source = %v, want %v", got.source, tt.wantSource)
			}
			if got.fromFlag != tt.wantFlag {
				t.Errorf("fromFlag = %v, want %v", got.fromFlag, tt.wantFlag)
			}
			if tt.wantValue != nil && got.value != tt.wantValue {
				t.Errorf("value = %v, want %v", got.value, tt.wantValue)
			}
		})
	}
}

// TestResolver_NotFound: a key nobody supplied is reported as absent, not as a
// zero value from some layer that happens to be switched on.
func TestResolver_NotFound(t *testing.T) {
	r := newResolver()
	r.useEnv("RES_")
	setFile(t, r, map[string]interface{}{"other": 1})

	got := r.resolve("key")
	if got.found {
		t.Errorf("resolve found %v from %v, want nothing", got.value, got.source)
	}
	if got.source != SourceNone {
		t.Errorf("source = %v, want SourceNone", got.source)
	}
}

// TestResolver_EnvDisabled: without useEnv the environment is not a source,
// however tempting the variable name looks.
func TestResolver_EnvDisabled(t *testing.T) {
	t.Setenv("RES_KEY", "from-env")

	r := newResolver()
	setFile(t, r, map[string]interface{}{"key": "from-file"})

	if got := r.resolve("key"); got.value != "from-file" {
		t.Errorf("value = %v from %v, want the file value", got.value, got.source)
	}
}

// TestResolver_EnvName covers the mapping from a configuration key to an
// environment variable name, in the one direction the resolver uses it.
func TestResolver_EnvName(t *testing.T) {
	tests := []struct {
		prefix string
		key    string
		want   string
	}{
		{"APP_", "port", "APP_PORT"},
		{"APP", "port", "APP_PORT"}, // a missing underscore is forgiven
		{"", "port", "PORT"},        // no prefix is legal
		{"APP_", "database_url", "APP_DATABASE_URL"},
		{"APP_", "server.port", "APP_SERVER_PORT"}, // dots are path separators
		{"APP_", "log-level", "APP_LOG_LEVEL"},     // so are dashes
		{"app_", "port", "APP_PORT"},               // the prefix is upper-cased too
	}

	for _, tt := range tests {
		if got := envKeyName(tt.prefix, tt.key); got != tt.want {
			t.Errorf("envKeyName(%q, %q) = %q, want %q", tt.prefix, tt.key, got, tt.want)
		}
	}
}

// TestResolver_EnvNestedKey: a dotted key reaches both a nested file value and
// a flat environment variable, and the environment wins.
func TestResolver_EnvNestedKey(t *testing.T) {
	t.Setenv("APP_SERVER_PORT", "9090")

	r := newResolver()
	r.useEnv("APP_")
	setFile(t, r, map[string]interface{}{
		"server": map[string]interface{}{"port": 8080},
	})

	got := r.resolve("server.port")
	if got.source != SourceEnv || got.value != "9090" {
		t.Errorf("resolve(server.port) = %v from %v, want \"9090\" from env", got.value, got.source)
	}

	// With the variable gone the nested file value is still reachable.
	if err := os.Unsetenv("APP_SERVER_PORT"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	if got := r.resolve("server.port"); got.source != SourceFile || got.value != 8080 {
		t.Errorf("resolve(server.port) = %v from %v, want 8080 from the file", got.value, got.source)
	}
}

// TestResolver_EmptyEnvValue: a variable set to the empty string is a value,
// not an absence. Unsetting and blanking are different acts.
func TestResolver_EmptyEnvValue(t *testing.T) {
	t.Setenv("APP_KEY", "")

	r := newResolver()
	r.useEnv("APP_")
	setFile(t, r, map[string]interface{}{"key": "from-file"})

	got := r.resolve("key")
	if got.source != SourceEnv || got.value != "" {
		t.Errorf("resolve(key) = %q from %v, want the empty environment value", got.value, got.source)
	}
}

// TestResolver_Concurrent hammers resolve while the file layer is replaced, the
// way a reload replaces it under a reader. Run with -race.
func TestResolver_Concurrent(t *testing.T) {
	r := newResolver()
	setFile(t, r, map[string]interface{}{"key": "initial"})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if got := r.resolve("key"); got.found {
						if s, ok := got.value.(string); !ok || !strings.HasPrefix(s, "v") && s != "initial" {
							t.Errorf("torn read: %v", got.value)
							return
						}
					}
				}
			}
		}()
	}

	for i := 0; i < 500; i++ {
		setFile(t, r, map[string]interface{}{"key": "v" + strings.Repeat("x", i%7)})
	}
	close(stop)
	wg.Wait()
}
