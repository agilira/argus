// config_writer_invariant_test.go: what GetConfig says is what the file
// reads back, after any sequence of edits
//
// Copyright (c) 2026 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"encoding/json"
	"math/big"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v4"
)

// The third audit of v1.10.1 fuzzed this and found two ways it failed: a
// timestamp in a flow collection, quoted by the encoder into a string,
// and a writer without a document (a new file, a map given) whose values
// were not the file's. The same property, on fixed seeds.
func TestGetConfigIsTheFile(t *testing.T) {
	starts := []string{
		"",
		"a: {t: 2020-01-01T00:00:00Z}\nb: 1\nl: [2020-01-01T00:00:00Z, 2]\n",
		"# head\nname: \"x\"\nn: 1.5\nnested:\n  k: v\n  z: ~\n",
		"base: &b {port: 1}\ndev: *b\nsvc:\n  <<: *b\n  x: 9\n",
	}
	values := []interface{}{nil, 1, 1.0, 2.5, "yes", "0x10", true, []string{"a"}, map[string]int{"k": 1},
		time.Date(2021, 2, 3, 4, 5, 6, 0, time.UTC), map[string]interface{}{}, []interface{}{}, int8(3), float32(0.1),
		big.NewInt(12345678901234), net.ParseIP("10.0.0.1"), 5 * time.Second,
		map[string]interface{}{"node": yaml.Node{Kind: yaml.ScalarNode, Value: "v"}}, *big.NewInt(7)}
	keys := []string{"a", "b", "a.c", "n", "nested.k", "nested.z.w", "new.deep", "l", "x"}
	for s, start := range starts {
		for seed := range 40 {
			r := rand.New(rand.NewSource(int64(seed*10 + s)))
			path := filepath.Join(t.TempDir(), "c.yaml")
			if start != "" {
				if err := os.WriteFile(path, []byte(start), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			w, err := NewConfigWriter(path, FormatYAML, nil)
			if err != nil {
				t.Fatal(err)
			}
			for step := range 12 {
				switch r.Intn(4) {
				case 0, 1:
					_ = w.SetValue(keys[r.Intn(len(keys))], values[r.Intn(len(values))])
				case 2:
					w.DeleteValue(keys[r.Intn(len(keys))])
				case 3:
					if err := w.WriteConfig(); err != nil {
						t.Fatalf("start %d seed %d step %d: %v", s, seed, step, err)
					}
					back, err := ParseConfig([]byte(read(t, path)), FormatYAML)
					if err != nil || !reflect.DeepEqual(back, w.GetConfig()) {
						t.Fatalf("start %d seed %d step %d: the file and GetConfig differ: %v\nfile:\n%s\nGetConfig: %#v", s, seed, step, err, read(t, path), w.GetConfig())
					}
				}
			}
		}
	}
}

// A map given to a YAML writer is held as the file reads it back, and one
// the file could not hold is refused at once.
func TestYAMLFromAMap(t *testing.T) {
	w, err := NewConfigWriter(filepath.Join(t.TempDir(), "c.yaml"), FormatYAML, map[string]interface{}{"n": nil, "f": 1.0, "d": 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(w.GetValue("n"), map[string]interface{}{}) || w.GetValue("f") != 1.0 || w.GetValue("d") != "5s" {
		t.Fatalf("%#v", w.GetConfig())
	}
	if _, err := NewConfigWriter(filepath.Join(t.TempDir(), "c.yaml"), FormatYAML, map[string]interface{}{"m": map[string]interface{}{"": 1}}); err == nil {
		t.Fatal("a key the file could not hold was accepted")
	}
}

// A float stays a float in YAML; "a: ~" can be filled; no key, a value
// holding itself, a caller's node with an anchor: each handled.
func TestYAMLEdgesOfTheThirdAudit(t *testing.T) {
	w, path := yamlWriter(t, "a: ~\n")
	for _, key := range []string{".", "...", " . "} {
		if err := w.SetValue(key, 1); err == nil || w.DeleteValue(key) {
			t.Errorf("%q accepted", key)
		}
	}
	if err := w.SetValue("x", 1.0); err != nil {
		t.Fatal(err)
	}
	if err := w.SetValue("a.b", 1); err != nil {
		t.Fatalf("a null parent: %v", err)
	}
	if err := w.WriteConfig(); err != nil || !strings.Contains(read(t, path), "x: 1.0") || w.GetValue("x") != 1.0 {
		t.Fatalf("%v %q", err, read(t, path))
	}
	cyclic := map[string]interface{}{}
	cyclic["self"] = cyclic
	list := []interface{}{nil}
	list[0] = list
	for _, v := range []interface{}{cyclic, list} {
		if err := w.SetValue("c", v); err == nil {
			t.Fatal("a value holding itself was taken")
		}
	}
	var anchored yaml.Node
	if err := yaml.Unmarshal([]byte("&a {k: 1}"), &anchored); err != nil {
		t.Fatal(err)
	}
	if err := w.SetValue("node", anchored.Content[0]); err != nil {
		t.Fatal(err)
	}
	if !w.shared {
		t.Fatal("a node with an anchor left the document unshared")
	}
}

// Go numbers of any width, pointers and float32 are held as the parsers
// give them.
func TestGoNumbersAndPointers(t *testing.T) {
	n := 7
	j, _ := NewConfigWriter(filepath.Join(t.TempDir(), "c.toml"), FormatTOML, map[string]interface{}{})
	for key, value := range map[string]interface{}{"i8": int8(3), "u": uint16(9), "f32": float32(0.1), "p": &n, "nilp": (*int)(nil)} {
		if err := j.SetValue(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if j.GetValue("i8") != 3 || j.GetValue("u") != 9 || j.GetValue("f32") != 0.1 || j.GetValue("p") != 7 || j.GetValue("nilp") != nil {
		t.Fatalf("%#v", j.GetConfig())
	}
	if v, err := normalizeGoValue(uint64(1<<63), 0); err != nil || v != uint64(1<<63) {
		t.Fatalf("a uint64 above int: %v %v", v, err)
	}
	_ = strconv.Itoa
}

// Fourth audit of v1.10.1: values with their own encoding were taken
// apart (big.Int written {}, an IP as 16 numbers, a json.RawMessage as
// bytes) and a nested *yaml.Node crashed the encoder; a key with a control
// character made a file never reopened; the initial map skipped all this.
func TestValuesThatWriteThemselves(t *testing.T) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte("k: 1"), &node); err != nil {
		t.Fatal(err)
	}
	for _, format := range []ConfigFormat{FormatYAML, FormatJSON} {
		path := filepath.Join(t.TempDir(), "c")
		w, err := NewConfigWriter(path, format, map[string]interface{}{})
		if err != nil {
			t.Fatal(err)
		}
		for key, value := range map[string]interface{}{"big": big.NewInt(5), "ip": net.ParseIP("1.2.3.4"), "raw": json.RawMessage(`{"a":1}`)} {
			if format == FormatYAML && key == "raw" {
				continue // JSON's own
			}
			if err := w.SetValue(key, value); err != nil {
				t.Fatalf("%v %s: %v", format, key, err)
			}
		}
		if format == FormatYAML {
			if err := w.SetValue("nested", map[string]interface{}{"n": node.Content[0]}); err != nil {
				t.Fatalf("nested node: %v", err)
			}
		}
		if err := w.WriteConfig(); err != nil {
			t.Fatal(err)
		}
		out := read(t, path)
		if !strings.Contains(out, "1.2.3.4") || !strings.Contains(out, "5") || strings.Contains(out, "{}") {
			t.Errorf("%v: %s", format, out)
		}
	}
}

func TestKeysAndInitialValues(t *testing.T) {
	w, _ := yamlWriter(t, "a: 1\n")
	if err := w.SetValue("c\x01", 5); err == nil || strings.Contains(err.Error(), "c\x01") {
		t.Fatalf("a control character in a key: %v", err)
	}
	cyclic := map[string]interface{}{}
	cyclic["self"] = cyclic
	if _, err := NewConfigWriter(filepath.Join(t.TempDir(), "c.json"), FormatJSON, cyclic); err == nil {
		t.Fatal("an initial map holding itself was taken")
	}
	j, err := NewConfigWriter(filepath.Join(t.TempDir(), "c.yaml"), FormatYAML, map[string]interface{}{"f": float32(1), "m": map[string]float64{"x": 1.0}})
	if err != nil {
		t.Fatal(err)
	}
	if j.GetValue("f") != 1.0 || j.GetValue("m.x") != 1.0 {
		t.Fatalf("%#v", j.GetConfig())
	}
}

// A value whose encoding is on its pointer, given by value, is written as
// it says too.
func TestMarshalerOnThePointer(t *testing.T) {
	if _, ok := withOwnEncoding(byPointer{}); !ok {
		t.Fatal("a pointer marshaler missed")
	}
	if _, ok := withOwnEncoding(struct{ X int }{1}); ok {
		t.Fatal("marshalsItself")
	}
}

type byPointer struct{ s string }

func (b *byPointer) MarshalText() ([]byte, error) { return []byte(b.s), nil }

// Fifth audit of v1.10.1: a yaml.Node by value inside a map, a list or
// the initial map crashed the encoder; a key with a C1 or a zero-width
// character, and any unprintable key of the initial map, made a JSON file
// never reopened; a big.Int by value was written {}.
func TestFifthAudit(t *testing.T) {
	node := yaml.Node{Kind: yaml.ScalarNode, Value: "plain"}
	w, path := yamlWriter(t, "a: 1\n")
	for key, value := range map[string]interface{}{"m": map[string]interface{}{"n": node}, "l": []interface{}{node}} {
		if err := w.SetValue(key, value); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
	}
	if _, err := NewConfigWriter(filepath.Join(t.TempDir(), "c.yaml"), FormatYAML, map[string]interface{}{"k": node, "deep": map[string]interface{}{"n": node}}); err != nil {
		t.Fatal(err)
	}
	if err := w.SetValue("b", *big.NewInt(5)); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteConfig(); err != nil || !strings.Contains(read(t, path), `b: "5"`) || !strings.Contains(read(t, path), "- plain") {
		t.Fatalf("%v %q", err, read(t, path))
	}
	j, _ := NewConfigWriter(filepath.Join(t.TempDir(), "c.json"), FormatJSON, map[string]interface{}{})
	for _, key := range []string{"a\u0085b", "a\u200bb", "a\x7fb"} {
		if err := j.SetValue(key, 1); err == nil {
			t.Errorf("%q accepted", key)
		}
	}
	if _, err := NewConfigWriter(filepath.Join(t.TempDir(), "c.json"), FormatJSON, map[string]interface{}{"x": map[string]interface{}{"a\x01": 1}}); err == nil {
		t.Fatal("an unprintable key of the initial map accepted")
	}
}
