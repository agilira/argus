// document_store_test.go - tests for reading documents off the filesystem
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// The document store takes paths and file contents from outside the process,
// so most of what is here is the hostile half: a FIFO in the prompts
// directory, a symlink out of it, a file that moves under the read.

package argus

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func writeDoc(t *testing.T, path, contents string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// store builds a document store over one group, with the default limits.
func store(group, pattern string, required bool) *documentStore {
	return &documentStore{
		sources: []documentSource{{group: group, pattern: pattern, required: required}},
		limits:  defaultDocumentLimits(),
	}
}

// loadOK loads and fails the test on error.
func loadOK(t *testing.T, s *documentStore) (map[DocumentID]Document, []documentSkip) {
	t.Helper()
	docs, skipped, err := s.load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return docs, skipped
}

func TestDocumentStore_Glob(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "prompts", "system.md"), "be careful")
	writeDoc(t, filepath.Join(dir, "prompts", "greeting.md"), "hello")
	writeDoc(t, filepath.Join(dir, "prompts", "notes.txt"), "not matched")

	docs, _ := loadOK(t, store("prompts", filepath.Join(dir, "prompts", "*.md"), false))

	if len(docs) != 2 {
		t.Fatalf("loaded %d documents, want 2: %v", len(docs), docs)
	}
	system := docs[DocumentID{Group: "prompts", Name: "system"}]
	if system.String() != "be careful" {
		t.Errorf("system = %q", system.String())
	}
	if system.Size != int64(len("be careful")) {
		t.Errorf("Size = %d", system.Size)
	}
	if system.Hash == "" || system.ModTime.IsZero() {
		t.Errorf("document metadata is incomplete: %+v", system)
	}
}

func TestDocumentStore_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := writeDoc(t, filepath.Join(dir, "system.md"), "be careful")

	docs, _ := loadOK(t, store("prompts", path, true))

	if _, ok := docs[DocumentID{Group: "prompts", Name: "system"}]; !ok {
		t.Errorf("a plain file path did not produce prompts/system: %v", docs)
	}
}

func TestDocumentStore_Recursive(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "skills", "text", "summarize.md"), "short")
	writeDoc(t, filepath.Join(dir, "skills", "summarize.md"), "top level")

	docs, _ := loadOK(t, store("skills", filepath.Join(dir, "skills", "**", "*.md"), false))

	// A recursive pattern keys by relative path, so the two summarize.md files
	// do not collide.
	if _, ok := docs[DocumentID{Group: "skills", Name: "text/summarize"}]; !ok {
		t.Errorf("nested document missing: %v", keysOf(docs))
	}
	if _, ok := docs[DocumentID{Group: "skills", Name: "summarize"}]; !ok {
		t.Errorf("top level document missing: %v", keysOf(docs))
	}
}

func TestDocumentStore_AppearsAndDisappears(t *testing.T) {
	dir := t.TempDir()
	prompts := filepath.Join(dir, "prompts")
	writeDoc(t, filepath.Join(prompts, "system.md"), "be careful")
	s := store("prompts", filepath.Join(prompts, "*.md"), false)

	if docs, _ := loadOK(t, s); len(docs) != 1 {
		t.Fatalf("first load has %d documents", len(docs))
	}

	writeDoc(t, filepath.Join(prompts, "tone.md"), "warm")
	if docs, _ := loadOK(t, s); len(docs) != 2 {
		t.Fatalf("a new file was not picked up: %v", keysOf(docs))
	}

	if err := os.Remove(filepath.Join(prompts, "system.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	docs, _ := loadOK(t, s)
	if _, ok := docs[DocumentID{Group: "prompts", Name: "system"}]; ok {
		t.Error("a deleted document is still loaded")
	}
}

func TestDocumentStore_OversizedOptionalIsSkipped(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "small.md"), "fine")
	writeDoc(t, filepath.Join(dir, "huge.md"), strings.Repeat("x", 4096))

	s := store("prompts", filepath.Join(dir, "*.md"), false)
	s.limits.maxBytes = 1024

	docs, skipped := loadOK(t, s)

	if len(docs) != 1 {
		t.Errorf("loaded %v, want only the small document", keysOf(docs))
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "size") {
		t.Errorf("skipped = %+v, want one entry explaining the size", skipped)
	}
}

func TestDocumentStore_OversizedRequiredRefusesTheCandidate(t *testing.T) {
	dir := t.TempDir()
	path := writeDoc(t, filepath.Join(dir, "system.md"), strings.Repeat("x", 4096))

	s := store("prompts", path, true)
	s.limits.maxBytes = 1024

	if _, _, err := s.load(); err == nil {
		t.Error("an oversized required document was accepted")
	}
}

func TestDocumentStore_TooManyDocuments(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 5; i++ {
		writeDoc(t, filepath.Join(dir, "doc"+string(rune('a'+i))+".md"), "x")
	}

	s := store("prompts", filepath.Join(dir, "*.md"), false)
	s.limits.maxDocuments = 3

	if _, _, err := s.load(); err == nil {
		t.Error("the document count limit was not enforced")
	}
}

func TestDocumentStore_TotalSizeCap(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 4; i++ {
		writeDoc(t, filepath.Join(dir, "doc"+string(rune('a'+i))+".md"), strings.Repeat("x", 400))
	}

	s := store("prompts", filepath.Join(dir, "*.md"), false)
	s.limits.maxTotalBytes = 1000

	if _, _, err := s.load(); err == nil {
		t.Error("the total size cap was not enforced")
	}
}

func TestDocumentStore_NameCollision(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "summarize.md"), "one")
	writeDoc(t, filepath.Join(dir, "summarize.txt"), "two")

	s := store("skills", filepath.Join(dir, "*"), false)

	// Two files that map to the same name is ambiguous, and picking one
	// silently would mean a deploy changes behaviour by file listing order.
	if _, _, err := s.load(); err == nil {
		t.Error("a name collision was resolved silently")
	}
}

func TestDocumentStore_SkipsNonRegularFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no FIFOs on Windows")
	}
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "system.md"), "be careful")
	fifo := filepath.Join(dir, "trap.md")
	mkfifo(t, fifo)

	// os.ReadFile on a FIFO blocks until somebody writes, which would hang the
	// reload for ever. Nothing opens it: the store stats first.
	done := make(chan struct{})
	var docs map[DocumentID]Document
	var skipped []documentSkip
	go func() {
		defer close(done)
		docs, skipped, _ = store("prompts", filepath.Join(dir, "*.md"), false).load()
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("load blocked on a FIFO")
	}

	if _, ok := docs[DocumentID{Group: "prompts", Name: "trap"}]; ok {
		t.Error("a FIFO was loaded as a document")
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "regular") {
		t.Errorf("skipped = %+v, want the FIFO reported as not a regular file", skipped)
	}
}

func TestDocumentStore_SymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	secret := writeDoc(t, filepath.Join(dir, "outside", "secret.md"), "not yours")
	prompts := filepath.Join(dir, "prompts")
	writeDoc(t, filepath.Join(prompts, "system.md"), "be careful")
	if err := os.Symlink(secret, filepath.Join(prompts, "leak.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	docs, skipped := loadOK(t, store("prompts", filepath.Join(prompts, "*.md"), false))

	if _, ok := docs[DocumentID{Group: "prompts", Name: "leak"}]; ok {
		t.Error("a symlink pointing outside the documents directory was followed")
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "outside") {
		t.Errorf("skipped = %+v, want the escape reported", skipped)
	}
}

// TestDocumentStore_SymlinkInsideIsFollowed is the Kubernetes shape: a
// ConfigMap mount is a directory of symlinks into a ..data directory that
// kubelet swaps. Refusing symlinks outright would make Argus useless there.
func TestDocumentStore_SymlinkInsideIsFollowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	mount := filepath.Join(dir, "prompts")
	data := filepath.Join(mount, "..data")
	writeDoc(t, filepath.Join(data, "system.md"), "be careful")
	if err := os.Symlink(filepath.Join(data, "system.md"), filepath.Join(mount, "system.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	docs, skipped := loadOK(t, store("prompts", filepath.Join(mount, "*.md"), false))

	doc, ok := docs[DocumentID{Group: "prompts", Name: "system"}]
	if !ok {
		t.Fatalf("the ConfigMap symlink was not followed: %v, skipped %+v", keysOf(docs), skipped)
	}
	if doc.String() != "be careful" {
		t.Errorf("content = %q", doc.String())
	}
}

func TestDocumentStore_SymlinkLoop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "b.md")
	if err := os.Symlink(b, a); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	docs, skipped := loadOK(t, store("prompts", filepath.Join(dir, "*.md"), false))

	if len(docs) != 0 {
		t.Errorf("a symlink loop produced documents: %v", keysOf(docs))
	}
	if len(skipped) != 2 {
		t.Errorf("skipped = %+v, want both ends of the loop reported", skipped)
	}
}

func TestDocumentStore_RequiredMissing(t *testing.T) {
	dir := t.TempDir()

	if _, _, err := store("prompts", filepath.Join(dir, "system.md"), true).load(); err == nil {
		t.Error("a missing required document was accepted")
	}
	// The same group, not required, is simply empty.
	docs, _ := loadOK(t, store("prompts", filepath.Join(dir, "system.md"), false))
	if len(docs) != 0 {
		t.Errorf("optional missing document produced %v", keysOf(docs))
	}
}

func TestDocumentStore_RequiredUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file modes do not stop this user")
	}
	dir := t.TempDir()
	path := writeDoc(t, filepath.Join(dir, "system.md"), "be careful")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if _, _, err := store("prompts", path, true).load(); err == nil {
		t.Error("an unreadable required document was accepted")
	}
}

// TestDocumentStore_StableRead: an editor that truncates and rewrites can be
// read exactly between the two, which yields half a prompt. The store stats
// before and after and discards the read when the file moved under it.
func TestDocumentStore_StableRead(t *testing.T) {
	dir := t.TempDir()
	path := writeDoc(t, filepath.Join(dir, "system.md"), "first version")

	s := store("prompts", path, false)
	s.afterRead = func(read string) {
		if read == path {
			writeDoc(t, path, "second version, longer than the first")
		}
	}

	docs, skipped := loadOK(t, s)

	if len(docs) != 0 {
		t.Errorf("a document that changed under the read was accepted: %v", keysOf(docs))
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0].Reason, "changed") {
		t.Errorf("skipped = %+v, want the unstable read reported", skipped)
	}

	// The next cycle, with the file at rest, loads it.
	s.afterRead = nil
	docs, _ = loadOK(t, s)
	if docs[DocumentID{Group: "prompts", Name: "system"}].String() != "second version, longer than the first" {
		t.Error("the next load did not pick the settled file up")
	}
}

// TestDocumentStore_DisappearsBetweenScanAndRead: a file listed by the scan
// can be gone by the time it is read. Optional, so the cycle proceeds.
func TestDocumentStore_DisappearsBetweenScanAndRead(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "system.md"), "be careful")
	writeDoc(t, filepath.Join(dir, "tone.md"), "warm")

	s := store("prompts", filepath.Join(dir, "*.md"), false)
	s.beforeRead = func(path string) {
		if strings.HasSuffix(path, "tone.md") {
			_ = os.Remove(path)
		}
	}

	docs, skipped := loadOK(t, s)

	if _, ok := docs[DocumentID{Group: "prompts", Name: "system"}]; !ok {
		t.Errorf("the surviving document was lost: %v", keysOf(docs))
	}
	if len(skipped) != 1 {
		t.Errorf("skipped = %+v, want the vanished document reported", skipped)
	}
}

// keysOf makes a failure message readable.
func keysOf(docs map[DocumentID]Document) []string {
	names := make([]string, 0, len(docs))
	for id := range docs {
		names = append(names, id.String())
	}
	return names
}

// TestDocumentStore_DotfileKeepsItsName: filepath.Ext calls a whole dotfile an
// extension, so stripping it would leave a document addressable only as
// Doc("prompts", "").
func TestDocumentStore_DotfileKeepsItsName(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, ".hidden"), "quiet")

	docs, _ := loadOK(t, store("prompts", filepath.Join(dir, "*"), false))

	if _, ok := docs[DocumentID{Group: "prompts", Name: ".hidden"}]; !ok {
		t.Errorf("dotfile lost its name: %v", keysOf(docs))
	}
	if _, empty := docs[DocumentID{Group: "prompts", Name: ""}]; empty {
		t.Error("a document was loaded under the empty name")
	}
}

// TestDocumentStore_PathsThatAreNotNames: a root, a dot or a parent reference
// reaches nameFor only through a bug, and it must come out as an error rather
// than as a document called "/" or "..".
func TestDocumentStore_PathsThatAreNotNames(t *testing.T) {
	for _, path := range []string{"/", ".", "..", "prompts/.."} {
		source := documentSource{group: "prompts", pattern: "prompts/*.md"}
		if name, err := source.nameFor("prompts", path); err == nil {
			t.Errorf("nameFor(%q) = %q, want an error", path, name)
		}
	}
}

// TestDocumentStore_GlobRootOfARootedPattern: a wildcard in the first element
// of an absolute pattern is still rooted at /, and a root of "" would make the
// containment check pass anything.
func TestDocumentStore_GlobRootOfARootedPattern(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path roots differ on Windows")
	}
	if got := globRoot("/*"); got != "/" {
		t.Errorf("globRoot(/*) = %q, want /", got)
	}
	if got := globRoot("*.md"); got != "." {
		t.Errorf("globRoot(*.md) = %q, want .", got)
	}
}

// TestDocumentStore_UnnameableFileIsSkipped: "..." is a legal file name whose
// extension, as far as filepath is concerned, is "." — stripping it leaves
// "..". Such a file is left out with a reason, and its neighbours still load.
func TestDocumentStore_UnnameableFileIsSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows reads "..." as a path, not as a file name, so the case
		// cannot be set up there. The derivation it guards is the same on
		// every platform and is covered by FuzzDocumentName.
		t.Skip("a file named \"...\" cannot be created on Windows")
	}
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "system.md"), "be careful")
	writeDoc(t, filepath.Join(dir, "..."), "nonsense")

	docs, skipped := loadOK(t, store("prompts", filepath.Join(dir, "*"), false))

	if _, ok := docs[DocumentID{Group: "prompts", Name: ".."}]; ok {
		t.Error("a document was loaded under the name \"..\"")
	}
	if _, ok := docs[DocumentID{Group: "prompts", Name: "system"}]; !ok {
		t.Errorf("the good document was lost: %v", keysOf(docs))
	}
	if len(skipped) != 1 {
		t.Errorf("skipped = %+v, want the unnameable file reported", skipped)
	}
}

// TestDocumentStore_RecursiveRootOfARootedPattern: a ** in the first element
// of an absolute pattern must walk /, not the working directory.
func TestDocumentStore_RecursiveRootOfARootedPattern(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("path roots differ on Windows")
	}
	root, suffix, recursive := splitRecursive("/**/*.md")
	if !recursive || root != "/" || suffix != "*.md" {
		t.Errorf("splitRecursive(/**/*.md) = %q, %q, %v", root, suffix, recursive)
	}
	if root, _, _ := splitRecursive("**/*.md"); root != "." {
		t.Errorf("splitRecursive(**/*.md) root = %q, want .", root)
	}
}
