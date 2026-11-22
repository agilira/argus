// config_writer_edges_test.go: the writer's refusals and rare values
//
// Copyright (c) 2026 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	stderrors "errors"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

// Every number kind is written exactly, the odd floats by name.
func TestNumbersOfEveryKind(t *testing.T) {
	for v, want := range map[interface{}]string{
		int8(-3): "-3", uint16(7): "7", float32(0.5): "0.5", 2.0: "2.0", 1e21: "1e+21",
		math.Inf(1): "inf", math.Inf(-1): "-inf", math.NaN(): "nan",
	} {
		if got, ok := formatNumber(v); !ok || got != want {
			t.Errorf("%T %v: %q", v, v, got)
		}
	}
	if _, ok := formatNumber("1"); ok {
		t.Error("a string taken as a number")
	}
	if formatPlainValue(nil) != "" || formatTOMLValue(nil) != `""` || formatHCLValue(struct{}{}) != `"{}"` || formatTOMLValue(struct{}{}) != `"{}"` {
		t.Error("plain values")
	}
}

// INI has one level: a mapping inside a section is [section.sub].
func TestININestedSections(t *testing.T) {
	out, err := serializeINI(map[string]interface{}{"top": 1, "a": map[string]interface{}{"k": "v", "b": map[string]interface{}{"x": nil}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(out); got != "top=1\n\n[a]\nk=v\n\n[a.b]\nx=\n" {
		t.Fatalf("%q", got)
	}
}

// Values of any type hash, typed: nil and a struct are not the same.
func TestHashOfOddValues(t *testing.T) {
	a := hashConfig(map[string]interface{}{"k": nil})
	b := hashConfig(map[string]interface{}{"k": struct{ X int }{1}})
	if a == b || hashConfig(nil) != 0 {
		t.Fatal("hash")
	}
}

// The YAML helpers refuse what they cannot hold.
func TestYAMLDocumentRefusals(t *testing.T) {
	if _, err := loadYAMLDocument([]byte("a: [unclosed")); err == nil {
		t.Error("broken YAML loaded")
	}
	if _, err := loadYAMLDocument([]byte("- 1")); err == nil {
		t.Error("a list as the root loaded")
	}
	if _, err := yamlDocumentOf(map[string]interface{}{"f": func() {}}); err == nil {
		t.Error("a func encoded")
	}
	if _, err := encodeYAMLDocument(&yaml.Node{Kind: yaml.Kind(99)}); err == nil {
		t.Error("an invalid node encoded")
	}
	doc, _ := loadYAMLDocument([]byte("a: 1\n"))
	if err := setYAMLPath(doc, []string{"b"}, func() {}); err == nil {
		t.Error("a func set")
	}
	del := func(d *yaml.Node, path ...string) bool { ok, _ := deleteYAMLPath(d, path); return ok }
	if err := setYAMLPath(doc, nil, 1); err == nil || del(doc) || del(nil, "a") {
		t.Error("an empty path taken")
	}
	if del(doc, "a", "x") || del(doc, "none") || del(doc, "none", "x") {
		t.Error("deleted what is not there")
	}
}

// A value the document cannot hold is refused; and an unencodable value
// is a writer's error, not a file half written.
func TestYAMLWriterRefusesWhatItCannotWrite(t *testing.T) {
	path := writeFile(t, "c.yaml", "a: 1\n")
	w, err := NewConfigWriter(path, FormatYAML, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.SetValue("f", func() {}); err == nil || w.GetValue("f") != nil {
		t.Fatal("a func set, or left in the values")
	}
	if _, err := NewConfigWriter(filepath.Join(t.TempDir(), "new.yaml"), FormatYAML, map[string]interface{}{"f": func() {}}); err == nil {
		t.Fatal("a func taken") // refused at once: a YAML writer holds its document from the start
	}
	w.doc = &yaml.Node{Kind: yaml.Kind(99)}
	w.config["changed"] = true
	if err := w.WriteConfig(); err == nil || !strings.Contains(err.Error(), "serialization") {
		t.Fatalf("an invalid document written: %v", err)
	}
}

// A backend that fails to close is reported.
func TestCloseReportsTheBackend(t *testing.T) {
	l, err := NewAuditLogger(AuditConfig{Enabled: true, OutputFile: filepath.Join(t.TempDir(), "audit.db"), MinLevel: AuditInfo})
	if err != nil {
		t.Fatal(err)
	}
	inner := l.backend
	l.backend = closeFails{inner}
	if err := l.Close(); err == nil || !strings.Contains(err.Error(), "close audit backend") {
		t.Fatalf("%v", err)
	}
	_ = inner.Close()
}

type closeFails struct{ auditBackend }

func (closeFails) Close() error { return stderrors.New("disk gone") }
