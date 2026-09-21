// document.go: documents — configuration that is text, not keys
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// Everything else in Argus assumes a file parses into a map. A system prompt
// or a skill does not: it is bytes, and it must stay bytes. Reading is not
// parsing, and a document never enters DetectFormat or ParseConfig — the
// attack surface of a parser is absent because there is no parser.

package argus

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// DocumentID addresses a document as a group and a name.
//
// The group is the directory it was declared with, the name its base name
// without extension. Two levels, not one: a flat namespace would collide the
// moment a summarize.md existed in both prompts/ and skills/.
type DocumentID struct {
	Group string `json:"group"`
	Name  string `json:"name"`
}

// String renders a document id as "group/name".
func (id DocumentID) String() string { return id.Group + "/" + id.Name }

// Document is a text file Argus watches and hands over unparsed.
//
// The content is held as a string, which is immutable, so every reader of a
// snapshot shares it safely; Bytes returns a copy for the callers that need
// one.
type Document struct {
	Group   string    `json:"group"`
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`

	// Hash is the SHA-256 of the content, hex encoded. It is what the audit
	// trail records: the name and the hash of a prompt say that it changed
	// without copying the prompt into an audit database.
	Hash string `json:"hash"`

	// Revision is the revision this content arrived in, which is not the
	// current revision — a document that did not change keeps the older one.
	Revision uint64 `json:"revision"`

	// text is unexported so that a shared snapshot cannot be rewritten by one
	// of its readers.
	text string
}

// ID returns the document's address.
func (d Document) ID() DocumentID { return DocumentID{Group: d.Group, Name: d.Name} }

// String returns the document's content. It costs nothing: the content is
// already a string.
func (d Document) String() string { return d.text }

// Bytes returns a copy of the document's content, for callers that need a
// mutable slice. The copy is deliberate; the snapshot is shared.
func (d Document) Bytes() []byte { return []byte(d.text) }

// newDocument builds a document and hashes its content.
func newDocument(group, name, path, text string, modTime time.Time) Document {
	sum := sha256.Sum256([]byte(text))
	return Document{
		Group:   group,
		Name:    name,
		Path:    path,
		Size:    int64(len(text)),
		ModTime: modTime,
		Hash:    hex.EncodeToString(sum[:]),
		text:    text,
	}
}

// Change is what a reload reports: which keys and which documents moved, by
// name. Never the values — a callback argument that carried a prompt's new
// text would end up in somebody's log line.
type Change struct {
	Revision  uint64       `json:"revision"`
	Keys      []string     `json:"keys,omitempty"`
	Documents []DocumentID `json:"documents,omitempty"`
}

// HasKey reports whether the named key changed in this revision.
func (c Change) HasKey(key string) bool {
	for _, changed := range c.Keys {
		if changed == key {
			return true
		}
	}
	return false
}

// HasDocument reports whether the named document changed in this revision.
func (c Change) HasDocument(group, name string) bool {
	for _, changed := range c.Documents {
		if changed.Group == group && changed.Name == name {
			return true
		}
	}
	return false
}
