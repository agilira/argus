// document_store.go: reading documents off the filesystem, safely
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// A documents directory is somewhere an operator drops files, so this is a
// hostile surface. Three rules carry most of the weight:
//
//   - stat before open. os.ReadFile on a FIFO blocks until somebody writes to
//     the other end, which would hang the reload for ever.
//   - resolve symlinks and require the target to stay inside the directory the
//     source named. Refusing symlinks outright is not an option: a Kubernetes
//     ConfigMap mount is a directory of symlinks into ..data.
//   - stat, read, stat again. An editor that truncates and rewrites can be
//     read exactly in between, and half a system prompt is worse than an old
//     one.

package argus

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agilira/go-errors"
)

// documentLimits bounds what a documents directory can cost.
//
// The caps stop a log file dropped into prompts/, not real use: a system
// prompt is 1-20 KB, a long one 50-100 KB, a skill with examples perhaps
// 100 KB. Both the current snapshot and the candidate are held during a swap,
// so the real peak is twice the total.
type documentLimits struct {
	maxBytes      int64 // per document
	maxTotalBytes int64 // across every document
	maxDocuments  int
}

// defaultDocumentLimits returns the caps an application gets without asking.
func defaultDocumentLimits() documentLimits {
	return documentLimits{
		maxBytes:      1 << 20,  // 1 MB
		maxTotalBytes: 16 << 20, // 16 MB
		maxDocuments:  1000,
	}
}

// documentSource is one declared group of documents: a file, or a directory
// with a glob.
//
// required changes what a problem means. An optional document that is
// oversized, unreadable or moving is skipped and reported, and the rest of the
// cycle proceeds. A required one that cannot be read refuses the whole
// candidate, because an agent running without its system prompt is worse than
// an agent that did not reload.
type documentSource struct {
	group    string
	pattern  string
	required bool
}

// documentSkip records a document that was left out, and why. The reasons are
// what Explain and the audit trail report.
type documentSkip struct {
	ID     DocumentID
	Path   string
	Reason string
}

// documentStore reads the declared sources into one map of documents.
type documentStore struct {
	sources []documentSource
	limits  documentLimits

	// beforeRead and afterRead are test seams for the two races that cannot be
	// provoked from outside: a file that disappears between the scan and the
	// read, and one that is rewritten under the read itself. They are nil in
	// every non-test build.
	beforeRead func(path string)
	afterRead  func(path string)
}

// load reads every source. The error return means the candidate must be
// refused; the skips are documents left out of an otherwise good candidate.
func (s *documentStore) load() (map[DocumentID]Document, []documentSkip, error) {
	docs := make(map[DocumentID]Document)
	var skipped []documentSkip
	var totalBytes int64

	for _, source := range s.sources {
		matches, root, err := source.scan()
		if err != nil {
			return nil, nil, err
		}
		if len(matches) == 0 && source.required {
			return nil, nil, errors.New(ErrCodeInvalidDocument,
				"required documents not found: "+source.pattern)
		}

		for _, match := range matches {
			name, err := source.nameFor(root, match)
			if err != nil {
				if source.required {
					return nil, nil, err
				}
				skipped = append(skipped, documentSkip{
					ID:     DocumentID{Group: source.group, Name: filepath.Base(match)},
					Path:   match,
					Reason: "does not name a document",
				})
				continue
			}
			id := DocumentID{Group: source.group, Name: name}

			if existing, clash := docs[id]; clash {
				return nil, nil, errors.New(ErrCodeInvalidDocument,
					"two files map to the same document "+id.String()+": "+
						existing.Path+" and "+match)
			}

			doc, reason := s.read(source, root, id, match)
			if reason != "" {
				if source.required {
					return nil, nil, errors.New(ErrCodeInvalidDocument,
						"required document "+id.String()+" cannot be used: "+reason)
				}
				skipped = append(skipped, documentSkip{ID: id, Path: match, Reason: reason})
				continue
			}

			totalBytes += doc.Size
			if totalBytes > s.limits.maxTotalBytes {
				return nil, nil, errors.New(ErrCodeInvalidDocument,
					"documents exceed the total size limit")
			}
			docs[id] = doc
			if len(docs) > s.limits.maxDocuments {
				return nil, nil, errors.New(ErrCodeInvalidDocument,
					"too many documents")
			}
		}
	}

	return docs, skipped, nil
}

// read returns the document at path, or the reason it cannot be used.
//
// Every refusal is a reason rather than an error because the caller decides
// what it means: nothing for an optional document, a refused candidate for a
// required one.
func (s *documentStore) read(source documentSource, root string, id DocumentID, path string) (Document, string) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		// A dangling link, a loop, or a file that vanished after the scan.
		return Document{}, "cannot be resolved: " + err.Error()
	}
	if !withinRoot(root, real) {
		return Document{}, "resolves outside the documents directory"
	}

	// Stat before open: this is what keeps a FIFO from blocking the reload.
	before, err := os.Stat(real)
	if err != nil {
		return Document{}, "cannot be read: " + err.Error()
	}
	if !before.Mode().IsRegular() {
		return Document{}, "is not a regular file"
	}
	if before.Size() > s.limits.maxBytes {
		return Document{}, "exceeds the per-document size limit"
	}

	if s.beforeRead != nil {
		s.beforeRead(path)
	}

	data, err := os.ReadFile(real) // #nosec G304 -- resolved, contained and stat-checked above
	if err != nil {
		return Document{}, "cannot be read: " + err.Error()
	}

	if s.afterRead != nil {
		s.afterRead(path)
	}

	// Stat again: if the file moved while it was being read, what is in hand
	// may be half of a save. Discard it; the next cycle finds it at rest.
	after, err := os.Stat(real)
	if err != nil {
		return Document{}, "changed while it was being read: " + err.Error()
	}
	if after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) {
		return Document{}, "changed while it was being read"
	}
	if int64(len(data)) > s.limits.maxBytes {
		return Document{}, "exceeds the per-document size limit"
	}

	return newDocument(id.Group, id.Name, path, string(data), after.ModTime()), ""
}

// scan expands a source's pattern into the paths it names, and the directory
// those paths must stay inside.
func (source documentSource) scan() ([]string, string, error) {
	if root, suffix, recursive := splitRecursive(source.pattern); recursive {
		matches, err := walkRecursive(root, suffix)
		if err != nil {
			return nil, "", err
		}
		return matches, root, nil
	}

	if !hasGlobMeta(source.pattern) {
		root := filepath.Dir(source.pattern)
		if _, err := os.Lstat(source.pattern); err != nil {
			return nil, root, nil // absent: required-ness decides what that means
		}
		return []string{source.pattern}, root, nil
	}

	matches, err := filepath.Glob(source.pattern)
	if err != nil {
		return nil, "", errors.Wrap(err, ErrCodeInvalidDocument,
			"invalid document pattern: "+source.pattern)
	}
	sort.Strings(matches)

	return matches, globRoot(source.pattern), nil
}

// nameFor derives a document's name from its path: the base name without
// extension, or the relative path without extension when the pattern was
// recursive, since two directories may hold the same file name.
func (source documentSource) nameFor(root, path string) (string, error) {
	if _, _, recursive := splitRecursive(source.pattern); !recursive {
		return derivedName(path, filepath.Base(path))
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", errors.Wrap(err, ErrCodeInvalidDocument,
			"document path is not inside its directory: "+path)
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		// Defence in depth: the walk only yields paths under root, so this is
		// unreachable today. If it ever becomes reachable, a document whose
		// name walks upwards is a document addressed as another group's.
		return "", errors.New(ErrCodeInvalidDocument,
			"document path escapes its directory: "+path)
	}
	dir, base := "", rel
	if cut := strings.LastIndex(rel, "/"); cut >= 0 {
		dir, base = rel[:cut+1], rel[cut+1:]
	}
	name, err := derivedName(path, base)
	if err != nil {
		return "", err
	}
	// The directory part travels into the name too, so it is checked as well:
	// only the last element went through derivedName.
	if strings.ContainsRune(dir, 0) {
		return "", errors.New(ErrCodeInvalidDocument,
			"path does not name a document: "+path)
	}

	return dir + name, nil
}

// derivedName turns a file's base name into a document name, and refuses the
// ones that are not names at all. Stripping the extension is what makes this
// necessary: "..." is a legal file name whose extension is ".", and the ".."
// left behind would address another group's directory.
func derivedName(path, base string) (string, error) {
	if !isNameable(base) {
		return "", errors.New(ErrCodeInvalidDocument,
			"path does not name a document: "+path)
	}

	name := trimDocumentExt(base)
	if !isNameable(name) {
		return "", errors.New(ErrCodeInvalidDocument,
			"path does not name a document: "+path)
	}

	return name, nil
}

// isNameable reports whether a base name can become a document name. A root,
// a dot and a parent reference are paths, not names.
func isNameable(base string) bool {
	switch base {
	case "", ".", "..", "/", "\\":
		return false
	}
	return !strings.ContainsRune(base, 0)
}

// trimDocumentExt strips a file's extension to get its name, except when that
// would leave nothing: a dotfile is all extension as far as filepath.Ext is
// concerned, and Doc("prompts", "") is not a document anyone can ask for.
func trimDocumentExt(base string) string {
	if name := strings.TrimSuffix(base, filepath.Ext(base)); name != "" {
		return name
	}
	return base
}

// splitRecursive recognises a pattern with a ** element, and returns the
// directory to walk and the pattern the file names must match.
func splitRecursive(pattern string) (root, suffix string, recursive bool) {
	parts := strings.Split(filepath.ToSlash(pattern), "/")
	for i, part := range parts {
		if part == "**" {
			root = filepath.FromSlash(strings.Join(parts[:i], "/"))
			suffix = filepath.Base(filepath.FromSlash(strings.Join(parts[i+1:], "/")))
			if root == "" {
				// As in globRoot: an absolute pattern whose ** is the first
				// element is rooted at /, and defaulting it to "." would walk
				// the working directory instead — a different filesystem.
				root = "."
				if strings.HasPrefix(filepath.ToSlash(pattern), "/") {
					root = string(filepath.Separator)
				}
			}
			if suffix == "" || suffix == "." {
				suffix = "*"
			}
			return root, suffix, true
		}
	}
	return "", "", false
}

// walkRecursive lists the files under root whose base name matches suffix.
func walkRecursive(root, suffix string) ([]string, error) {
	var matches []string

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			// An unreadable subdirectory is not a reason to lose the rest.
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if ok, _ := filepath.Match(suffix, entry.Name()); ok {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, errors.Wrap(err, ErrCodeInvalidDocument,
			"cannot walk the documents directory: "+root)
	}
	sort.Strings(matches)

	return matches, nil
}

// globRoot returns the directory part of a glob: everything up to the first
// element that holds a wildcard.
func globRoot(pattern string) string {
	parts := strings.Split(filepath.ToSlash(pattern), "/")
	for i, part := range parts {
		if hasGlobMeta(part) {
			root := filepath.FromSlash(strings.Join(parts[:i], "/"))
			if root == "" {
				// The wildcard is the first element: either the pattern is
				// relative, and the root is here, or it is rooted at /.
				if strings.HasPrefix(filepath.ToSlash(pattern), "/") {
					return string(filepath.Separator)
				}
				return "."
			}
			return root
		}
	}
	return filepath.Dir(pattern)
}

// hasGlobMeta reports whether a path element is a pattern rather than a name.
func hasGlobMeta(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

// withinRoot reports whether a resolved path stays inside a resolved root.
//
// Both sides are resolved before they are compared, because a prefix test on
// unresolved paths is exactly the check a symlink defeats.
func withinRoot(root, path string) bool {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(realRoot, path)
	if err != nil {
		return false
	}

	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
