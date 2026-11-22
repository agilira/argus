// config_writer_preserve_test.go: the writer changes what it is told, and
// nothing else
//
// Copyright (c) 2026 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const handWritten = `# The service, as the team wrote it.
name: billing   # never rename: the dashboards use it
url: https://example.com/a/very/long/path/that/goes/on/and/on/well/past/eighty/columns/of/text
database:
  # the primary
  host: db1
  port: 5432
profiles:
  - name: a
    level: 1
`

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// 22/11: a YAML file is edited, not rewritten: its comments, its order,
// its other entries and its long lines stay; what is set changes, what is
// new goes at the end of its mapping.
func TestYAMLWriterKeepsTheFile(t *testing.T) {
	path := writeFile(t, "c.yaml", handWritten)
	w, err := NewConfigWriter(path, FormatYAML, nil) // the file's, not an empty one
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]interface{}{"database.port": 6543, "database.user": "app", "new.deep.key": true} {
		if err := w.SetValue(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if !w.DeleteValue("profiles") {
		t.Fatal("profiles not deleted")
	}
	if err := w.WriteConfig(); err != nil {
		t.Fatal(err)
	}
	out := read(t, path)
	for _, want := range []string{"# The service, as the team wrote it.", "# never rename: the dashboards use it", "  # the primary",
		"https://example.com/a/very/long/path/that/goes/on/and/on/well/past/eighty/columns/of/text\n", "port: 6543", "  user: app", "new:\n  deep:\n    key: true"} {
		if !strings.Contains(out, want) {
			t.Errorf("lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "profiles") || strings.Index(out, "name:") > strings.Index(out, "database:") {
		t.Errorf("order or deletion:\n%s", out)
	}
	cfg, err := ParseConfig([]byte(out), FormatYAML)
	if err != nil || cfg["name"] != "billing" {
		t.Fatalf("%v %v", cfg, err)
	}
}

// Numbers are written exactly, in every format (%.6g made 1234567.5 into
// 1.23457e+06); keys a hand-written writer left bare are quoted in YAML.
func TestWriterValuesComeBackTheSame(t *testing.T) {
	config := map[string]interface{}{"ratio": 0.1234567, "big": 1234567.5, "count": 42, "on": true, "text": "a: b # c",
		"nested": map[string]interface{}{"deeper": map[string]interface{}{"x": 2.5, "y": "z"}}}
	for _, format := range []ConfigFormat{FormatYAML, FormatTOML, FormatHCL} {
		path := filepath.Join(t.TempDir(), "c")
		w, err := NewConfigWriter(path, format, config)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.WriteConfig(); err != nil {
			t.Fatal(err)
		}
		got, err := ParseConfig([]byte(read(t, path)), format)
		if err != nil {
			t.Fatalf("%v: %v\n%s", format, err, read(t, path))
		}
		if got["big"] != 1234567.5 || got["ratio"] != 0.1234567 {
			t.Errorf("%v: numbers %v %v\n%s", format, got["big"], got["ratio"], read(t, path))
		}
		deeper, _ := got["nested"].(map[string]interface{})["deeper"].(map[string]interface{})
		if deeper["x"] != 2.5 || deeper["y"] != "z" {
			t.Errorf("%v: nesting lost: %v\n%s", format, got["nested"], read(t, path))
		}
	}
	yamlPath := filepath.Join(t.TempDir(), "k.yaml")
	w, _ := NewConfigWriter(yamlPath, FormatYAML, map[string]interface{}{"a:b": 1, "#c": "d"})
	if err := w.WriteConfig(); err != nil {
		t.Fatal(err)
	}
	if got, err := ParseConfig([]byte(read(t, yamlPath)), FormatYAML); err != nil || got["a:b"] != 1 || got["#c"] != "d" {
		t.Fatalf("keys: %v %v\n%s", got, err, read(t, yamlPath))
	}
}

// The same configuration is written the same way every time.
func TestWriterOutputIsStable(t *testing.T) {
	config := map[string]interface{}{"b": 1, "a": 2, "c": map[string]interface{}{"z": 1, "y": 2}, "d": "x"}
	for _, format := range []ConfigFormat{FormatYAML, FormatTOML, FormatHCL, FormatINI, FormatProperties, FormatJSON} {
		w, _ := NewConfigWriter(filepath.Join(t.TempDir(), "c"), format, config)
		first, err := w.serializeConfig(w.config, nil)
		if err != nil {
			t.Fatal(err)
		}
		for range 20 {
			if again, _ := w.serializeConfig(w.config, nil); string(again) != string(first) {
				t.Fatalf("%v: output changed:\n%s\n---\n%s", format, first, again)
			}
		}
	}
}

// 22/11: the change hash walked a map in Go's random order, so the same
// configuration hashed differently, and missed small or typed changes.
func TestChangesAreSeen(t *testing.T) {
	path := writeFile(t, "c.json", `{"a": 1, "b": {"c": 1e-7, "d": "x"}, "e": [1, 2]}`)
	w, err := NewConfigWriter(path, FormatJSON, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if w.HasChanges() {
			t.Fatal("unchanged configuration seen as changed")
		}
	}
	for key, value := range map[string]interface{}{"b.c": 0.0, "a": "1"} {
		if err := w.SetValue(key, value); err != nil {
			t.Fatal(err)
		}
		if !w.HasChanges() {
			t.Errorf("%s = %#v not seen", key, value)
		}
		if err := w.Reset(); err != nil {
			t.Fatal(err)
		}
	}
}

// 22/11: a writer made without a configuration started empty, and its
// first write replaced the file with only the keys it was given.
func TestWriterOfAnExistingFileKeepsTheRest(t *testing.T) {
	path := writeFile(t, "c.json", `{"keep": "me", "port": 1}`)
	w, err := NewConfigWriter(path, FormatJSON, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.SetValue("port", 2); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteConfig(); err != nil {
		t.Fatal(err)
	}
	if got, _ := ParseConfig([]byte(read(t, path)), FormatJSON); got["keep"] != "me" {
		t.Fatalf("the rest was lost: %v", got)
	}
	if _, err := NewConfigWriter(writeFile(t, "bad.json", "{not json"), FormatJSON, nil); err == nil {
		t.Fatal("a file that does not parse was taken as empty")
	}
}

// 22/11: GetValue compared each key with the last by name, so a.b.a
// stopped at the first a; a refused SetValue left the maps it made.
func TestPathsByPosition(t *testing.T) {
	w, _ := NewConfigWriter(filepath.Join(t.TempDir(), "c.json"), FormatJSON, map[string]interface{}{
		"a": map[string]interface{}{"b": map[string]interface{}{"a": "deep"}}, "flat": 1})
	if got := w.GetValue("a.b.a"); got != "deep" {
		t.Fatalf("a.b.a = %v", got)
	}
	before := w.GetConfig()
	if err := w.SetValue("new.flat.x", 1); err != nil {
		t.Fatal(err)
	}
	if err := w.DeleteValue("new"); !err {
		t.Fatal("new not deleted")
	}
	if err := w.SetValue("flat.x.y", 1); err == nil {
		t.Fatal("a path through a value was accepted")
	}
	if !reflect.DeepEqual(w.GetConfig(), before) {
		t.Fatalf("a refused path changed the configuration: %v", w.GetConfig())
	}
	w.config["z"], w.config["m"] = 1, 2
	if keys := w.ListKeys(""); !reflect.DeepEqual(keys, []string{"a.b.a", "flat", "m", "z"}) {
		t.Fatalf("keys %v", keys)
	}
}

// The YAML document refuses the same paths the map does.
func TestYAMLDocumentPaths(t *testing.T) {
	path := writeFile(t, "c.yaml", "a: 1\nb:\n  c: 2\n")
	w, err := NewConfigWriter(path, FormatYAML, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.SetValue("a.x", 1); err == nil {
		t.Fatal("a path through a value was accepted")
	}
	if w.DeleteValue("b.none") || w.DeleteValue("none.c") {
		t.Fatal("deleted what is not there")
	}
	if !w.DeleteValue("b.c") {
		t.Fatal("b.c not deleted")
	}
	if _, err := NewConfigWriter(writeFile(t, "list.yaml", "- 1\n- 2\n"), FormatYAML, nil); err == nil {
		t.Fatal("a list as the root was accepted")
	}
	empty := writeFile(t, "empty.yaml", "")
	w, err = NewConfigWriter(empty, FormatYAML, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.SetValue("k", "v"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteConfig(); err != nil || strings.TrimSpace(read(t, empty)) != "k: v" {
		t.Fatalf("%v %q", err, read(t, empty))
	}
}
