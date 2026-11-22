// parser_yaml_keys_test.go: no line of a YAML configuration vanishes
//
// Copyright (c) 2026 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"strings"
	"testing"
)

// 22/11 (yaml v4): a null key was dropped in silence, with v3 too ("~: x",
// "null: x"), so a configuration line vanished; v4 also accepts ": x" and
// put a nested one under a nil key, out of reach of the key policy. Each
// is refused, with its line and without the document's text.
func TestYAMLKeysThatCannotBeHeld(t *testing.T) {
	for doc, line := range map[string]string{
		": secret-value\na: 1":         "line 1",
		"a: 1\n~: secret-value":        "line 2",
		"a: 1\nnull: secret-value":     "line 2",
		"a:\n  b: 1\n  : secret-value": "line 3",
		"? [1, 2]\n: secret-value":     "line 1",
		"? {k: v}\n: secret-value":     "line 1",
		"n: &none ~\n*none : secret":   "line 2", // an alias to a null key is a null key, at the alias's line
		"l: &list [1]\n*list : secret": "line 2", // and an alias to a list, a list
	} {
		_, err := parseYAML([]byte(doc))
		if err == nil || !strings.Contains(err.Error(), line) || strings.Contains(err.Error(), "secret-value") {
			t.Errorf("%q: %v", doc, err)
		}
	}
}

// Keys a map holds as names still parse: numbers and booleans become
// their text, as before.
func TestYAMLScalarKeysStillParse(t *testing.T) {
	cfg, err := parseYAML([]byte("1: one\ntrue: yes\n1.5: x\nname: v\nbase: &b {k: 1}\nmerged:\n  <<: *b\n  j: 2"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg["1"] != "one" || cfg["name"] != "v" {
		t.Fatalf("%v", cfg)
	}
	if m, ok := cfg["merged"].(map[string]interface{}); !ok || m["k"] == nil {
		t.Fatalf("merge key: %v", cfg["merged"])
	}
	// An alias to a name is that name.
	cfg, err = parseYAML([]byte("names: &host db\n*host : 5432"))
	if err != nil || cfg["db"] != 5432 {
		t.Fatalf("alias key: %v %v", cfg, err)
	}
}

// The line is found in v4's position format too, for the fallback that
// keeps the position alone.
func TestYAMLLineOfV4Positions(t *testing.T) {
	for msg, want := range map[string]string{"go-yaml load error in constructor at L2.C7: x": "2", "line 4: did not find expected key": "4"} {
		if m := yamlLinePattern.FindStringSubmatch(msg); m == nil || m[1] != want {
			t.Errorf("%q: %v", msg, m)
		}
	}
}
