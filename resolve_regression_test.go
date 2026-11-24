// resolve_regression_test.go: regressions in value resolution, binding and document globs
//
// Copyright (c) 2026 AGILira - A. Giordano
// Series: an AGILira library
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A: JSON null reads as the literal string "<nil>".
func TestResolve_JSONNullReadsAsEmptyString(t *testing.T) {
	s := startWith(t, "c.json", `{"model": null}`, nil)
	if got := s.GetString("model"); got != "" {
		t.Errorf("GetString(model) = %q, want \"\"", got)
	}
	type C struct {
		Model string `argus:"model"`
	}
	b, err := Bind[C](s)
	if err == nil && b.Value().Model == "<nil>" {
		t.Errorf("Bind Model = %q, want \"\" or an error", b.Value().Model)
	}
}

// B: Bind truncates a fractional / overflowing number into an int silently.
func TestBind_IntRefusesFractionalAndOverflowingNumbers(t *testing.T) {
	s := startWith(t, "c.json", `{"port": 8080.9, "big": 1e20}`, nil)
	type C struct {
		Port int   `argus:"port"`
		Big  int64 `argus:"big"`
	}
	b, err := Bind[C](s)
	if err == nil {
		t.Fatalf("Bind accepted port=8080.9 as %d and big=1e20 as %d", b.Value().Port, b.Value().Big)
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("error does not name the key: %v", err)
	}
	// The getters keep their lenient contract: not an int reads as zero.
	if got := s.GetInt("port"); got != 0 {
		t.Errorf("GetInt(port) = %d, want 0", got)
	}
	whole := startWith(t, "c.json", `{"port": 8080}`, nil)
	if got := whole.GetInt("port"); got != 8080 {
		t.Errorf("GetInt(port) = %d, want 8080", got)
	}
}

func TestBind_IntRefusesNumberOutOfRange(t *testing.T) {
	type C struct {
		Big int64 `argus:"big"`
	}
	s := startWith(t, "c.json", `{"big": 1e20}`, nil)
	if _, err := Bind[C](s); err == nil || !strings.Contains(err.Error(), "big") {
		t.Errorf("Bind(big=1e20) error = %v, want one naming big", err)
	}
}

func TestResolve_NestedOverridesReachableByDottedKey(t *testing.T) {
	s := startWith(t, "c.json", `{"server": {"port": 1}}`, func(b *SetupBuilder) {
		b.Overrides(map[string]interface{}{"server": map[string]interface{}{"port": 2}})
	})
	if got := s.GetInt("server.port"); got != 2 {
		t.Errorf("GetInt(server.port) = %d, want 2 from the nested override", got)
	}
}

// C: a nested Defaults map is unreachable by its dotted key.
func TestResolve_NestedDefaultsReachableByDottedKey(t *testing.T) {
	s := startWith(t, "c.json", `{}`, func(b *SetupBuilder) {
		b.Defaults(map[string]interface{}{"server": map[string]interface{}{"port": 9000}})
	})
	if got := s.GetInt("server.port"); got != 9000 {
		t.Errorf("GetInt(server.port) = %d, want 9000 from nested default", got)
	}
}

// D: a Defaults map shadows a flat file subtree for a composed field.
func TestBind_FlatFileOutranksNestedDefaultMap(t *testing.T) {
	s := startWith(t, "c.properties", "servers.a=from-file\n", func(b *SetupBuilder) {
		b.Defaults(map[string]interface{}{"servers": map[string]interface{}{"a": "from-default"}})
	})
	type C struct {
		Servers map[string]string `argus:"servers"`
	}
	b, err := Bind[C](s)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := b.Value().Servers["a"]; got != "from-file" {
		t.Errorf("servers.a = %q, want from-file (file outranks defaults)", got)
	}
}

// E: required on a nested struct field is ignored.
func TestBind_RequiredNestedStructIsEnforced(t *testing.T) {
	s := startWith(t, "c.json", `{"other": 1}`, nil)
	type C struct {
		Server struct {
			Port int `argus:"port"`
		} `argus:"server,required"`
	}
	if _, err := Bind[C](s); err == nil {
		t.Errorf("Bind succeeded with required section 'server' absent")
	}
	t.Setenv("RQ_SERVER_PORT", "8080")
	viaEnv := startWith(t, "c.json", `{}`, func(b *SetupBuilder) { b.Env("RQ_") })
	if _, err := Bind[C](viaEnv); err != nil {
		t.Errorf("Bind with server.port from env: %v", err)
	}
	flat := startWith(t, "c.properties", "server.port=1\n", nil)
	if _, err := Bind[C](flat); err != nil {
		t.Errorf("Bind with flat server.port: %v", err)
	}
}

// F: the part of a ** pattern after ** but before the file name is ignored.
func TestDocumentStore_RecursiveGlobMatchesMiddleElements(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"en/a.md", "fr/b.md", "x/y/en/c.md", "en/z/d.md"} {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	store := &documentStore{
		sources: []documentSource{{group: "g", pattern: filepath.Join(root, "**", "en", "*.md")}},
		limits:  defaultDocumentLimits(),
	}
	docs, _, err := store.load()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for id := range docs {
		names = append(names, id.Name)
	}
	if len(docs) != 2 || docs[DocumentID{"g", "en/a"}].Name == "" || docs[DocumentID{"g", "x/y/en/c"}].Name == "" {
		t.Errorf("pattern **/en/*.md loaded %v, want en/a and x/y/en/c", names)
	}
}

// startRemoteOnly starts Settings whose only source is a remote serving values.
func startRemoteOnly(t *testing.T, scheme string, values map[string]interface{}) *Settings {
	t.Helper()
	url := freshRemote(t, &settingsRemoteProvider{scheme: scheme, values: values})
	s, err := Setup("t").Remote(url, &RemoteConfigOptions{RetryAttempts: 1}).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// A map field binds from the remote, nested or written flat, when no file
// holds it.
func TestBind_MapFromRemoteNestedOrFlat(t *testing.T) {
	type C struct {
		Servers map[string]string `argus:"servers"`
	}
	for name, values := range map[string]map[string]interface{}{
		"nested": {"servers": map[string]interface{}{"a": "from-remote"}},
		"flat":   {"servers.a": "from-remote"},
	} {
		b, err := Bind[C](startRemoteOnly(t, "regressionmap"+name, values))
		if err != nil {
			t.Fatalf("%s: Bind: %v", name, err)
		}
		if got := b.Value().Servers["a"]; got != "from-remote" {
			t.Errorf("%s: servers.a = %q, want from-remote", name, got)
		}
	}
}

// A required section is present when the remote holds its keys written flat.
func TestBind_RequiredSectionFromFlatRemote(t *testing.T) {
	type C struct {
		Server struct {
			Port int `argus:"port"`
		} `argus:"server,required"`
	}
	b, err := Bind[C](startRemoteOnly(t, "regressionreqflat", map[string]interface{}{"server.port": 8080}))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := b.Value().Server.Port; got != 8080 {
		t.Errorf("server.port = %d, want 8080", got)
	}
}

// A path that cannot be made relative to the root (one absolute, one not)
// matches nothing.
func TestMatchesBelow_UnrelatablePath(t *testing.T) {
	if matchesBelow(string(filepath.Separator)+"root", "relative.md", []string{"*.md"}) {
		t.Error("a path not relative to the root matched")
	}
}

// A required map is satisfied by keys a .properties file writes flat.
func TestBind_RequiredMapFromFlatFile(t *testing.T) {
	type C struct {
		Servers map[string]string `argus:"servers,required"`
	}
	b, err := Bind[C](startWith(t, "c.properties", "servers.a=from-file\n", nil))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := b.Value().Servers["a"]; got != "from-file" {
		t.Errorf("servers.a = %q, want from-file", got)
	}
	if _, err := Bind[C](startWith(t, "c.json", `{"other": 1}`, nil)); err == nil {
		t.Error("Bind succeeded with the required map absent")
	}
}

// A required section written nested is present.
func TestBind_RequiredSectionNested(t *testing.T) {
	type C struct {
		Server struct {
			Port int `argus:"port"`
		} `argus:"server,required"`
	}
	b, err := Bind[C](startWith(t, "c.json", `{"server": {"port": 8080}}`, nil))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := b.Value().Server.Port; got != 8080 {
		t.Errorf("server.port = %d, want 8080", got)
	}
}
