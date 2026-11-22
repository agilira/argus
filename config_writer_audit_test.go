// config_writer_audit_test.go: what the audit of v1.10.0 found, kept from
// coming back
//
// Copyright (c) 2026 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func yamlWriter(t *testing.T, body string) (*ConfigWriter, string) {
	t.Helper()
	path := writeFile(t, "c.yaml", body)
	w, err := NewConfigWriter(path, FormatYAML, nil)
	if err != nil {
		t.Fatal(err)
	}
	return w, path
}

// An anchored value is not replaced or deleted (its aliases would point
// at nothing, the file would not parse), and no path goes through an alias
// or a merged mapping: each is refused, and nothing changes.
func TestYAMLAnchorsAliasesAndMerges(t *testing.T) {
	const doc = "base: &b {port: 1, host: h}\nnum: &n 5\nx: *n\ndev: *b\nstage:\n  <<: *b\n  name: s\n"
	refusals := []func(w *ConfigWriter) error{
		func(w *ConfigWriter) error { return w.SetValue("num", 6) },
		func(w *ConfigWriter) error { return w.SetValue("base", map[string]interface{}{"port": 2}) },
		func(w *ConfigWriter) error { return w.SetValue("dev.port", 2) },
		func(w *ConfigWriter) error { return w.SetValue("stage.host.x", 1) },
	}
	for i, refuse := range refusals {
		w, path := yamlWriter(t, doc)
		before := w.GetConfig()
		if err := refuse(w); err == nil {
			t.Errorf("%d: accepted", i)
		}
		if w.DeleteValue("num") || w.DeleteValue("dev.port") || w.DeleteValue("stage.port") {
			t.Errorf("%d: deleted an anchor, through an alias or a merged key", i)
		}
		if !reflect.DeepEqual(w.GetConfig(), before) {
			t.Errorf("%d: values changed", i)
		}
		if err := w.SetValue("unrelated", 1); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteConfig(); err != nil {
			t.Fatal(err)
		}
		back, err := ParseConfig([]byte(read(t, path)), FormatYAML)
		if err != nil || !reflect.DeepEqual(back, w.GetConfig()) {
			t.Errorf("%d: the file and the values differ: %v\n%s", i, err, read(t, path))
		}
	}
}

// Inside an anchored mapping a value may change: its aliases follow, in
// the file and in the values alike.
func TestYAMLEditInsideAnAnchor(t *testing.T) {
	w, path := yamlWriter(t, "base: &b {port: 1}\ndev: *b\n")
	if err := w.SetValue("base.port", 2); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteConfig(); err != nil {
		t.Fatal(err)
	}
	back, _ := ParseConfig([]byte(read(t, path)), FormatYAML)
	if !reflect.DeepEqual(back, w.GetConfig()) || w.GetValue("dev.port") != 2 {
		t.Fatalf("%v / %v", back, w.GetConfig())
	}
}

// A replaced string keeps its quoting; a file of "~" is an empty one.
func TestYAMLQuotingAndNullRoot(t *testing.T) {
	w, path := yamlWriter(t, "name: \"x\"\n")
	if err := w.SetValue("name", "y"); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteConfig(); err != nil || !strings.Contains(read(t, path), `name: "y"`) {
		t.Fatalf("%v %q", err, read(t, path))
	}
	if _, err := NewConfigWriter(writeFile(t, "n.yaml", "~\n"), FormatYAML, nil); err != nil {
		t.Fatal(err)
	}
}

// Properties and INI keys are nested on load: an edit of db.host changes
// db.host, once, every time (it was written old or new by chance).
func TestFlatFormatsAreAddressed(t *testing.T) {
	for format, body := range map[ConfigFormat]string{FormatProperties: "db.host=old\nother=1\n", FormatINI: "[db]\nhost=old\n"} {
		for range 10 {
			path := writeFile(t, "c", body)
			w, err := NewConfigWriter(path, format, nil)
			if err != nil {
				t.Fatal(err)
			}
			if w.GetValue("db.host") != "old" {
				t.Fatalf("%v: get %v", format, w.GetValue("db.host"))
			}
			if err := w.SetValue("db.host", "new"); err != nil {
				t.Fatal(err)
			}
			if err := w.WriteConfig(); err != nil {
				t.Fatal(err)
			}
			if out := read(t, path); !strings.Contains(out, "new") || strings.Contains(out, "old") {
				t.Fatalf("%v: %q", format, out)
			}
		}
	}
	if _, err := NewConfigWriter(writeFile(t, "c.properties", "a=1\na.b=2\n"), FormatProperties, nil); err == nil {
		t.Fatal("a key both a value and a group was accepted")
	}
}

// What a text format cannot hold is an error naming the key, not a file
// that reads back otherwise.
func TestTextFormatsRefuseWhatTheyCannotHold(t *testing.T) {
	cases := map[ConfigFormat][]map[string]interface{}{
		FormatTOML:       {{"lom": []interface{}{map[string]interface{}{"n": 1}}}, {"nested": []interface{}{[]interface{}{1}}}, {"comma": []interface{}{"a,b"}}, {"q": `say "hi"`}, {"nil": nil}},
		FormatHCL:        {{"lom": []interface{}{map[string]interface{}{"n": 1}}}, {"empty": map[string]interface{}{}}, {"nil": nil}},
		FormatINI:        {{"list": []interface{}{1, "x"}}, {"s": "true"}, {"lead": "  x"}},
		FormatProperties: {{"list": []interface{}{1}}, {"s": "42"}},
	}
	for format, configs := range cases {
		for _, config := range configs {
			w, err := NewConfigWriter(filepath.Join(t.TempDir(), "c"), format, config)
			if err != nil {
				t.Fatal(err)
			}
			err = w.WriteConfig()
			var key string
			for k := range config {
				key = k
			}
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Errorf("%v %v: %v", format, config, err)
			}
		}
	}
}

// Reset after a failed reload drops the document too: kept, it was
// written back over what GetConfig said.
func TestResetDropsTheDocument(t *testing.T) {
	w, path := yamlWriter(t, "a: 1\nkeep: me\n")
	if err := os.WriteFile(path, []byte("a: [broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := w.Reset(); err == nil {
		t.Fatal("a broken file reset without error")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := w.SetValue("new", 1); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteConfig(); err != nil {
		t.Fatal(err)
	}
	if out := read(t, path); strings.Contains(out, "keep") {
		t.Fatalf("the old document came back: %q", out)
	}
}

// Writes and reads at once do not race (run with -race).
func TestWriterConcurrentUse(t *testing.T) {
	w, _ := yamlWriter(t, "a: 1\nb:\n  c: 2\n")
	dir := t.TempDir()
	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = w.WriteConfigAs(filepath.Join(dir, "out"+string(rune('a'+i))+".yaml"))
		}()
		go func() {
			defer wg.Done()
			_ = w.GetValue("b.c")
		}()
	}
	wg.Wait()
}

// An audit file .DB is a database as .db is.
func TestAuditDatabaseExtensionAnyCase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.DB")
	if err := os.WriteFile(path, []byte("this is not a database, but a file of text long enough to be read"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAuditLogger(AuditConfig{Enabled: true, OutputFile: path, MinLevel: AuditInfo}); err == nil {
		t.Fatal("a .DB SQLite cannot open was accepted")
	}
}

// The refusals' other paths: a group given twice, a nested key that is a
// value too, a value that would not read back as YAML, keys a flat format
// reads otherwise, a nested difference.
func TestWriterRefusalPaths(t *testing.T) {
	for _, config := range []map[string]interface{}{
		{"a": 1, "a.b": 2},
		{"a": map[string]interface{}{"b": 1}, "a.b": 2},
		{"g": map[string]interface{}{"x": 1, "x.y": 2}},
	} {
		if _, err := NewConfigWriter(filepath.Join(t.TempDir(), "c.ini"), FormatINI, config); err == nil {
			t.Errorf("%v accepted", config)
		}
	}
	w, _ := yamlWriter(t, "a: 1\n")
	if err := w.SetValue("x", map[string]interface{}{"": 1}); err == nil || w.GetValue("x") != nil {
		t.Fatal("an empty YAML key set")
	}
	for format, config := range map[ConfigFormat]map[string]interface{}{
		FormatProperties: {"a=b": "c"},
		FormatINI:        {"n": map[string]interface{}{"s": "true"}},
	} {
		w, err := NewConfigWriter(filepath.Join(t.TempDir(), "c"), format, config)
		if err != nil {
			continue // refused already: fine
		}
		if err := w.WriteConfig(); err == nil {
			t.Errorf("%v %v written", format, config)
		}
		if err := w.WriteConfigAs(filepath.Join(t.TempDir(), "d")); err == nil {
			t.Errorf("%v %v exported", format, config)
		}
	}
}

// The file's header comment, hung on the first key, stays when that key
// is deleted; an alias as the key deleted is refused.
func TestYAMLDeleteKeepsTheHeader(t *testing.T) {
	w, path := yamlWriter(t, "# the file's header\nfirst: 1\nn: &n 2\nx: *n\n")
	if !w.DeleteValue("first") || w.DeleteValue("x") {
		t.Fatal("first kept, or the alias deleted")
	}
	if err := w.WriteConfig(); err != nil || !strings.Contains(read(t, path), "# the file's header") {
		t.Fatalf("%v %q", err, read(t, path))
	}
}

// Second audit of v1.10.1: deleting the last key of a group left an empty
// group, which INI, Properties and HCL cannot hold: nothing could be
// written after. The group goes with its last key.
func TestDeletingTheLastKeyOfAGroup(t *testing.T) {
	for format, body := range map[ConfigFormat]string{FormatProperties: "db.host=h\napp.x=1\n", FormatINI: "[db]\nhost=h\n[app]\nx=1\n", FormatHCL: "db {\n  host = \"h\"\n}\nx = 1\n"} {
		path := writeFile(t, "c", body)
		w, err := NewConfigWriter(path, format, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !w.DeleteValue("db.host") {
			t.Fatalf("%v: not deleted", format)
		}
		if err := w.WriteConfig(); err != nil {
			t.Fatalf("%v: %v", format, err)
		}
		if out := read(t, path); strings.Contains(out, "db") {
			t.Errorf("%v: the group stayed: %q", format, out)
		}
	}
}

// A YAML file without anchors is edited in place: a thousand edits of a
// file of 500 keys take a fraction of a second (5.8 s read back whole).
func TestYAMLEditsAreFast(t *testing.T) {
	var b strings.Builder
	for i := range 500 {
		b.WriteString("# key " + strconv.Itoa(i) + "\nk" + strconv.Itoa(i) + ": " + strconv.Itoa(i) + "\n")
	}
	w, path := yamlWriter(t, b.String())
	start := time.Now()
	for i := range 1000 {
		if err := w.SetValue("k"+strconv.Itoa(i%500), i); err != nil {
			t.Fatal(err)
		}
	}
	if took := time.Since(start); took > 500*time.Millisecond {
		t.Fatalf("1000 edits took %v", took)
	}
	if err := w.WriteConfig(); err != nil {
		t.Fatal(err)
	}
	back, err := ParseConfig([]byte(read(t, path)), FormatYAML)
	if err != nil || !reflect.DeepEqual(back, w.GetConfig()) || !strings.Contains(read(t, path), "# key 499") {
		t.Fatalf("the file and the values differ: %v", err)
	}
}

// A Go value of its own types is written by every format, and read as the
// file would give it.
func TestTypedGoValues(t *testing.T) {
	for _, format := range []ConfigFormat{FormatYAML, FormatJSON, FormatTOML, FormatHCL} {
		path := filepath.Join(t.TempDir(), "c")
		w, _ := NewConfigWriter(path, format, map[string]interface{}{})
		for key, value := range map[string]interface{}{"tags": []string{"a", "b"}, "ports": []int{1, 2}, "labels": map[string]string{"k": "v"}} {
			if err := w.SetValue(key, value); err != nil {
				t.Fatalf("%v %s: %v", format, key, err)
			}
		}
		if err := w.WriteConfig(); err != nil {
			t.Fatalf("%v: %v", format, err)
		}
		if tags, ok := w.GetValue("tags").([]interface{}); !ok || len(tags) != 2 {
			t.Errorf("%v: tags %#v", format, w.GetValue("tags"))
		}
	}
}

// A refusal says why; a merged value is not "deleted" when the override
// goes and the merged one would read back.
func TestRefusalsSayWhy(t *testing.T) {
	w, _ := yamlWriter(t, "base: &b {x: 1}\nsvc:\n  <<: *b\n  x: 9\nn: &n 2\n")
	if err := w.SetValue("n", 3); err == nil || !strings.Contains(err.Error(), "edit the file by hand") {
		t.Fatalf("%v", err)
	}
	if w.DeleteValue("svc.x") || w.GetValue("svc.x") != 9 {
		t.Fatalf("override: deleted, or changed: %v", w.GetValue("svc.x"))
	}
	if _, err := NewConfigWriter(writeFile(t, "b.yaml", "a: [x\n"), FormatYAML, nil); err == nil || !strings.Contains(err.Error(), "line") {
		t.Fatalf("broken file: %v", err)
	}
}

// A file of comments only keeps them; a lost key is named, not its group;
// GetValue gives a copy.
func TestCommentsOnlyNamesAndCopies(t *testing.T) {
	w, path := yamlWriter(t, "# only a comment\n")
	if err := w.SetValue("k", 1); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteConfig(); err != nil || !strings.Contains(read(t, path), "# only a comment") {
		t.Fatalf("%v %q", err, read(t, path))
	}
	if d := firstDifference(map[string]interface{}{"sec": map[string]interface{}{"empty": map[string]interface{}{}, "k": 1}}, map[string]interface{}{}, ""); d != "sec.empty" {
		t.Fatalf("named %q", d)
	}
	j, _ := NewConfigWriter(filepath.Join(t.TempDir(), "c.json"), FormatJSON, map[string]interface{}{"b": map[string]interface{}{"k": 1}})
	got := j.GetValue("b").(map[string]interface{})
	got["k"] = 2
	if j.GetValue("b.k") != 1 {
		t.Fatal("GetValue gave the writer's own map")
	}
}

// The other paths of the second round: pruning through several levels and
// stopping at a group still holding keys, a shared document refusing an
// edit that would not read back, nested and non-string-keyed Go values.
func TestSecondRoundPaths(t *testing.T) {
	path := writeFile(t, "c.properties", "a.b.c=1\na.d=2\n")
	w, err := NewConfigWriter(path, FormatProperties, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !w.DeleteValue("a.b.c") || w.GetValue("a.d") != 2 || w.GetValue("a.b") != nil {
		t.Fatalf("pruning: %v", w.GetConfig())
	}
	if !w.DeleteValue("a.d") || len(w.GetConfig()) != 0 {
		t.Fatalf("pruning to the top: %v", w.GetConfig())
	}
	shared, _ := yamlWriter(t, "n: &n 1\nx: *n\n")
	if err := shared.SetValue("y", map[string]interface{}{"": 1}); err == nil || shared.GetValue("y") != nil {
		t.Fatal("a shared document took a value that does not read back")
	}
	plain, _ := yamlWriter(t, "a: 1\n")
	if err := plain.SetValue("y", map[interface{}]interface{}{nil: 1}); err == nil {
		t.Fatal("a null key set")
	}
	j, _ := NewConfigWriter(filepath.Join(t.TempDir(), "c.json"), FormatJSON, map[string]interface{}{})
	if err := j.SetValue("deep", map[string]interface{}{"l": []interface{}{[]string{"x"}}, "m": map[int]string{1: "one"}}); err != nil {
		t.Fatal(err)
	}
	l := j.GetValue("deep.l").([]interface{})
	if _, ok := l[0].([]interface{}); !ok {
		t.Fatalf("nested []string not held as []interface{}: %#v", l[0])
	}
	if _, ok := j.GetValue("deep.m").(map[int]string); !ok {
		t.Fatalf("a map with int keys changed: %#v", j.GetValue("deep.m"))
	}
}
