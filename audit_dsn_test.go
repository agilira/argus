// audit_dsn_test.go: the trail goes to the file named, whatever its name
//
// Copyright (c) 2026 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Audit 22/11: a path written raw in the URI went elsewhere: cut at # or
// ? (to another file, readable by all, without WAL), or refused at a %41
// (and the backend fell back to JSONL without its chain).
func TestAuditDatabaseAtAnyName(t *testing.T) {
	names := []string{"run#1", "what?", "a%41b", "with space", "per%cento"}
	if runtime.GOOS == "windows" {
		names = []string{"run#1", "a%41b", "with space"} // ? is not a Windows file name
	}
	for _, name := range names {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "audit.db")
		db, err := openSQLiteDatabase(path)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var seq int
		var schema, file, mode string
		if err := db.QueryRow("PRAGMA database_list").Scan(&seq, &schema, &file); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()
		if !sameFile(t, file, path) || !strings.EqualFold(mode, "wal") {
			t.Errorf("%s: opened %q in %s mode", name, file, mode)
		}
		if entries, _ := os.ReadDir(filepath.Dir(dir)); len(entries) != 1 {
			t.Errorf("%s: files beside the directory: %v", name, entries)
		}
	}
}

func sameFile(t *testing.T, a, b string) bool {
	t.Helper()
	ia, err := os.Stat(a)
	if err != nil {
		return false
	}
	ib, err := os.Stat(b)
	return err == nil && os.SameFile(ia, ib)
}

// A relative path is the process's working directory's, as a plain path.
func TestAuditDSNIsAbsolute(t *testing.T) {
	dsn, err := sqliteDSN("rel/audit.db")
	if err != nil || !strings.HasPrefix(dsn, "file:///") || strings.Contains(dsn, "file:///rel/") {
		t.Fatalf("%q %v", dsn, err)
	}
}

// The URI of each kind of path, on any system: a Windows drive after the
// third slash, and every character a URI would read escaped.
func TestAuditFileURI(t *testing.T) {
	for path, want := range map[string]string{
		"/home/u/audit.db":            "file:///home/u/audit.db",
		"C:/Users/a b/run#1/audit.db": "file:///C:/Users/a%20b/run%231/audit.db",
		"/srv/what?/a%41/audit.db":    "file:///srv/what%3F/a%2541/audit.db",
	} {
		if got := fileURI(path); got != want {
			t.Errorf("%q: %q, want %q", path, got, want)
		}
	}
}

// A relative path whose base cannot be known (the working directory was
// removed) is refused, not opened somewhere else.
func TestAuditDSNWithoutAWorkingDirectory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("a removed working directory is reported on Linux")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PWD", "")
	if _, err := openSQLiteDatabase("audit.db"); err == nil {
		t.Fatal("opened under a removed directory")
	}
}
