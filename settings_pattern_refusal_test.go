// settings_pattern_refusal_test.go: malformed globs must be refused, not empty
//
// Two more sites discarded filepath.Match's error, so a glob the standard
// library cannot parse behaved exactly like a pattern that matched nothing:
// Setup().DirPatterns() silently read no configuration file, and a recursive
// Documents() pattern silently found no document.
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSetup_DirPatterns_RejectsMalformedPattern pins that a glob which cannot
// be parsed refuses the load instead of yielding an empty directory.
func TestSetup_DirPatterns_RejectsMalformedPattern(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.json"), []byte(`{"a":1}`), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	s, err := Setup("app").Dir(dir).DirPatterns("*.json", "[a-").Start()
	if err == nil {
		_ = s.Close()
		t.Fatal("a malformed DirPatterns glob must refuse the load")
	}
	if !strings.Contains(err.Error(), "[a-") {
		t.Fatalf("the error must name the offending pattern, got %v", err)
	}
}

// TestSetup_DirPatterns_AcceptsValidPatterns is the companion guard: the
// refusal must not reach the patterns people actually write.
func TestSetup_DirPatterns_AcceptsValidPatterns(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.json"), []byte(`{"a":1}`), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	s, err := Setup("app").Dir(dir).DirPatterns("*.json", "[0-9]*.yaml").Start()
	if err != nil {
		t.Fatalf("valid patterns must be accepted, got %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if got := s.GetInt("a"); got != 1 {
		t.Fatalf("the directory should still have been read: a=%d", got)
	}
}

// TestSetup_Documents_RejectsMalformedRecursivePattern pins the recursive
// document walk. The non-recursive branch goes through filepath.Glob, which
// reports ErrBadPattern; the ** branch used filepath.Match and dropped it, so
// the same typo was an error in one shape and silence in the other.
func TestSetup_Documents_RejectsMalformedRecursivePattern(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "system.md"), []byte("hello"), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	s, err := Setup("app").Documents("prompts", filepath.Join(dir, "**", "[a-")).Start()
	if err == nil {
		_ = s.Close()
		t.Fatal("a malformed recursive document pattern must refuse the load")
	}
	if !strings.Contains(err.Error(), "[a-") {
		t.Fatalf("the error must name the offending pattern, got %v", err)
	}
}

// TestSetup_Documents_AcceptsValidRecursivePattern keeps the working case
// working: a well-formed ** pattern still finds its documents.
func TestSetup_Documents_AcceptsValidRecursivePattern(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested")
	if err := os.Mkdir(sub, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sub, "system.md"), []byte("hello"), 0600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	s, err := Setup("app").Documents("prompts", filepath.Join(dir, "**", "*.md")).Start()
	if err != nil {
		t.Fatalf("valid recursive pattern must be accepted, got %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
}
