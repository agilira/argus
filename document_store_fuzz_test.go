// document_store_fuzz_test.go - fuzzing document names and patterns
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"path/filepath"
	"strings"
	"testing"
)

// FuzzDocumentName: file names come from whoever can write to the documents
// directory. Whatever they are, the name they resolve to must stay a name —
// never an absolute path, never a walk upwards, never empty, because a
// document's name is the key an application asks for by hand.
func FuzzDocumentName(f *testing.F) {
	f.Add("/srv/prompts", "/srv/prompts/system.md", false)
	f.Add("/srv/prompts", "/srv/prompts/nested/system.md", true)
	f.Add("/srv/prompts", "/srv/prompts/..data/system.md", false)
	f.Add("/srv/prompts", "/srv/prompts/.hidden", false)
	f.Add("/srv/prompts", "/srv/prompts/a.b.c.md", true)
	f.Add(".", "system.md", false)

	f.Fuzz(func(t *testing.T, root, path string, recursive bool) {
		pattern := filepath.Join(root, "*.md")
		if recursive {
			pattern = filepath.Join(root, "**", "*.md")
		}
		source := documentSource{group: "prompts", pattern: pattern}

		name, err := source.nameFor(root, path)
		if err != nil {
			return // an unusable path is refused, which is a fine answer
		}

		if name == "" {
			// An empty name would be addressable as Doc("prompts", ""), which
			// no application would ever ask for on purpose.
			t.Fatalf("nameFor(%q, %q) produced an empty name", root, path)
		}
		if filepath.IsAbs(name) {
			t.Fatalf("nameFor(%q, %q) = %q, which is an absolute path", root, path, name)
		}
		if strings.HasPrefix(name, "../") || strings.Contains(name, "/../") || name == ".." {
			t.Fatalf("nameFor(%q, %q) = %q, which walks out of its group", root, path, name)
		}
		if strings.ContainsRune(name, '\x00') {
			t.Fatalf("nameFor(%q, %q) = %q, which holds a NUL", root, path, name)
		}
	})
}

// FuzzDocumentPattern: the pattern comes from the application, but a typo in
// it must fail as an error, never as a panic or as a walk of the filesystem
// from the root.
func FuzzDocumentPattern(f *testing.F) {
	f.Add("prompts/*.md")
	f.Add("prompts/**/*.md")
	f.Add("[")
	f.Add("**")
	f.Add("/**/*")
	f.Add("")

	f.Fuzz(func(t *testing.T, pattern string) {
		dir := t.TempDir()
		// Keep the fuzzer inside a temporary directory: a pattern rooted at /
		// would otherwise walk the machine.
		source := documentSource{group: "g", pattern: filepath.Join(dir, pattern)}

		if _, _, recursive := splitRecursive(source.pattern); recursive {
			// A recursive pattern that climbed out of the temporary directory
			// would walk the machine, which is minutes of work and no signal.
			if root, _, _ := splitRecursive(source.pattern); !strings.HasPrefix(root, dir) {
				return
			}
		}

		matches, root, err := source.scan()
		if err != nil {
			return
		}
		if root == "" {
			t.Fatalf("scan(%q) returned an empty root", pattern)
		}
		// Every match must sit under the root the scan reports, because that
		// root is what the containment check is then applied against. A
		// pattern that names a directory above the application's own is the
		// application's business; a file escaping the scanned directory is
		// not, and that is what withinRoot refuses.
		for _, match := range matches {
			if !strings.HasPrefix(filepath.Clean(match), filepath.Clean(root)) {
				t.Fatalf("scan(%q) matched %q, outside its own root %q", pattern, match, root)
			}
		}
	})
}
