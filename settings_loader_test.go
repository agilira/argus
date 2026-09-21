// settings_loader_test.go - tests for building a candidate out of the sources
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSources_MergesDirectoriesAndFiles(t *testing.T) {
	dir := t.TempDir()
	mount := filepath.Join(dir, "conf.d")
	writeDoc(t, filepath.Join(mount, "a.json"), `{"port": 8080, "workers": 4}`)
	writeDoc(t, filepath.Join(mount, "b.json"), `{"tracing": true}`)
	override := writeDoc(t, filepath.Join(dir, "local.json"), `{"workers": 16}`)

	sources := &configSources{dirs: []string{mount}, files: []fileSource{{path: override}}}

	candidate, _, err := sources.build(context.Background(), nil, true)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if got := candidate.file["port"]; got != float64(8080) && got != 8080 {
		t.Errorf("port = %v", got)
	}
	if candidate.file["tracing"] != true {
		t.Errorf("tracing = %v, want the second file's value", candidate.file["tracing"])
	}
	// A file declared after a directory is the more specific source: it wins.
	if got := candidate.file["workers"]; got != float64(16) && got != 16 {
		t.Errorf("workers = %v, want the explicit file's 16", got)
	}
}

func TestSources_BrokenFileRefusesTheCandidate(t *testing.T) {
	dir := t.TempDir()
	path := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080`)

	if _, _, err := (&configSources{files: []fileSource{{path: path}}}).build(context.Background(), nil, true); err == nil {
		t.Error("a file that does not parse was accepted; a typo would empty the configuration")
	}
}

func TestSources_MissingFileRefusesTheCandidate(t *testing.T) {
	dir := t.TempDir()

	sources := &configSources{files: []fileSource{{path: filepath.Join(dir, "gone.json")}}}
	if _, _, err := sources.build(context.Background(), nil, true); err == nil {
		t.Error("a declared file that does not exist was accepted")
	}
}

func TestSources_CarriesDocuments(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "claude-opus-5"}`)
	writeDoc(t, filepath.Join(dir, "prompts", "system.md"), "be careful")

	sources := &configSources{
		files: []fileSource{{path: filepath.Join(dir, "config.json")}},
		docs: &documentStore{
			sources: []documentSource{{group: "prompts", pattern: filepath.Join(dir, "prompts", "*.md")}},
			limits:  defaultDocumentLimits(),
		},
	}

	candidate, skipped, err := sources.build(context.Background(), nil, true)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %+v", skipped)
	}
	if doc := candidate.docs[DocumentID{Group: "prompts", Name: "system"}]; doc.String() != "be careful" {
		t.Errorf("document = %q", doc.String())
	}
}

// TestFingerprint_SeesWhatChanged is what decides whether a cycle rebuilds at
// all: it only stats, so it is cheap enough to run at the poll interval.
func TestFingerprint_SeesWhatChanged(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)
	prompts := filepath.Join(dir, "prompts")
	writeDoc(t, filepath.Join(prompts, "system.md"), "be careful")

	sources := &configSources{
		files: []fileSource{{path: config}},
		docs: &documentStore{
			sources: []documentSource{{group: "prompts", pattern: filepath.Join(prompts, "*.md")}},
			limits:  defaultDocumentLimits(),
		},
	}

	first := sources.fingerprint()
	if first == "" {
		t.Fatal("empty fingerprint")
	}
	if again := sources.fingerprint(); again != first {
		t.Error("the fingerprint moved while nothing did")
	}

	writeDoc(t, config, `{"port": 9090}`)
	if changed := sources.fingerprint(); changed == first {
		t.Error("a rewritten configuration file did not move the fingerprint")
	}

	before := sources.fingerprint()
	writeDoc(t, filepath.Join(prompts, "tone.md"), "warm")
	if changed := sources.fingerprint(); changed == before {
		t.Error("a new document did not move the fingerprint")
	}

	before = sources.fingerprint()
	if err := os.Remove(filepath.Join(prompts, "tone.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if changed := sources.fingerprint(); changed == before {
		t.Error("a deleted document did not move the fingerprint")
	}
}

// TestFingerprint_MissingFileIsStillAFingerprint: a configuration file that is
// deleted must move the fingerprint rather than make it unreadable, so the
// cycle rebuilds, refuses the candidate and keeps the last good revision.
func TestFingerprint_MissingFileIsStillAFingerprint(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)
	sources := &configSources{files: []fileSource{{path: config}}}

	before := sources.fingerprint()
	if err := os.Remove(config); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if after := sources.fingerprint(); after == before {
		t.Error("deleting the configuration file did not move the fingerprint")
	}
}

// TestSources_DirectorySkipsNonRegularFiles: the same FIFO that would hang the
// document store hangs a configuration directory, and the guard has to be on
// both paths. A defence applied on one path only is not a defence.
func TestSources_DirectorySkipsNonRegularFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no FIFOs on Windows")
	}
	dir := t.TempDir()
	mount := filepath.Join(dir, "conf.d")
	writeDoc(t, filepath.Join(mount, "app.json"), `{"port": 8080}`)
	mkfifo(t, filepath.Join(mount, "trap.json"))

	sources := &configSources{dirs: []string{mount}}

	done := make(chan *snapshot, 1)
	go func() {
		candidate, _, err := sources.build(context.Background(), nil, true)
		if err != nil {
			t.Errorf("build: %v", err)
		}
		done <- candidate
	}()

	select {
	case candidate := <-done:
		if got := candidate.file["port"]; got != float64(8080) && got != 8080 {
			t.Errorf("port = %v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("build blocked on a FIFO in the configuration directory")
	}
}

// TestSources_DirectoryRefusesAnEscapingSymlink: a symlink dropped into a
// configuration directory must not pull a file from outside it into the
// application's configuration. The directory watcher has checked this since
// v1.5.1; this path has to check it too.
func TestSources_DirectoryRefusesAnEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	mount := filepath.Join(dir, "conf.d")
	writeDoc(t, filepath.Join(mount, "app.json"), `{"port": 8080}`)
	outside := writeDoc(t, filepath.Join(dir, "outside", "secret.json"), `{"port": 1, "secret": "stolen"}`)
	if err := os.Symlink(outside, filepath.Join(mount, "zz-leak.json")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	candidate, _, err := (&configSources{dirs: []string{mount}}).build(context.Background(), nil, true)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if _, leaked := candidate.file["secret"]; leaked {
		t.Error("a symlink out of the configuration directory was merged in")
	}
	if got := candidate.file["port"]; got != float64(8080) && got != 8080 {
		t.Errorf("port = %v, want the directory's own 8080", got)
	}
}

// TestSources_DirectoryFollowsAConfigMapSymlink: and the Kubernetes shape must
// keep working — a ConfigMap mount is a directory of symlinks into ..data.
func TestSources_DirectoryFollowsAConfigMapSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	mount := filepath.Join(dir, "conf.d")
	data := filepath.Join(mount, "..data")
	writeDoc(t, filepath.Join(data, "app.json"), `{"port": 8080}`)
	if err := os.Symlink(filepath.Join(data, "app.json"), filepath.Join(mount, "app.json")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	candidate, _, err := (&configSources{dirs: []string{mount}}).build(context.Background(), nil, true)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := candidate.file["port"]; got != float64(8080) && got != 8080 {
		t.Errorf("port = %v, want the ConfigMap's 8080", got)
	}
}

// writeBenchFile is writeDoc without a *testing.T, for the benchmarks.
func writeBenchFile(path, contents string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(contents), 0o600)
}

// TestSources_ExplicitFileMustBeARegularFile: a FIFO named as the
// configuration file would block Start for ever with no error and no log line.
// The application declared the path, so it gets an error rather than a skip —
// but it gets an error.
func TestSources_ExplicitFileMustBeARegularFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no FIFOs on Windows")
	}
	dir := t.TempDir()
	fifo := filepath.Join(dir, "config.json")
	mkfifo(t, fifo)

	failed := make(chan error, 1)
	go func() {
		_, _, err := (&configSources{files: []fileSource{{path: fifo}}}).build(context.Background(), nil, true)
		failed <- err
	}()

	select {
	case err := <-failed:
		if err == nil {
			t.Error("a FIFO was accepted as a configuration file")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("build blocked on a FIFO named as the configuration file")
	}
}

// TestSources_DirectorySkipsAreReported: an operator who drops a symlink or a
// pipe into a configuration directory and sees nothing happen deserves to find
// out why from Explain, as they would for a document.
func TestSources_DirectorySkipsAreReported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	dir := t.TempDir()
	mount := filepath.Join(dir, "conf.d")
	writeDoc(t, filepath.Join(mount, "app.json"), `{"port": 8080}`)
	outside := writeDoc(t, filepath.Join(dir, "outside", "secret.json"), `{"secret": 1}`)
	if err := os.Symlink(outside, filepath.Join(mount, "leak.json")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	settings, err := Setup("service").Dir(mount).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	issues := settings.Explain().Issues
	if len(issues) != 1 || !strings.Contains(issues[0], "leak.json") {
		t.Errorf("Issues = %v, want the skipped entry named", issues)
	}
}
