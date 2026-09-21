// snapshot.go: one revision of everything a reload can replace
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// The model is sources -> merge -> snapshot -> access -> reload. Everything
// readable at an instant belongs to one revision: a reload builds a candidate,
// validates it, and only then swaps it in. A candidate that fails validation
// is dropped whole — the current snapshot stays and the next cycle tries
// again, so a half-saved prompt never reaches the application.

package argus

import (
	"errors"
	"sync"
	"sync/atomic"
)

// errNilCandidate is returned by apply when there is nothing to publish. It is
// a programming error rather than a configuration one, so it is not wrapped in
// a configuration error code.
var errNilCandidate = errors.New("argus: cannot publish a nil configuration snapshot")

// snapshot holds the layers a reload replaces, at one revision. It is
// immutable once published: readers hold a pointer to it and are never
// interrupted by a swap.
type snapshot struct {
	revision uint64
	file     map[string]interface{}
	remote   map[string]interface{}
	docs     map[DocumentID]Document

	// remoteErr is why the remote layer is missing or stale, carried from one
	// revision to the next until a fetch succeeds.
	remoteErr string

	// digest is the content identity of this revision, worked out before it
	// was published. It is empty on a revision published by ConfigManager,
	// which does not offer one.
	digest string

	// issues are the sources that could not be used while the candidate was
	// still good enough to publish. Explain reports them.
	issues []sourceIssue
}

// document returns a document from this revision.
func (s *snapshot) document(group, name string) (Document, bool) {
	doc, ok := s.docs[DocumentID{Group: group, Name: name}]
	return doc, ok
}

// derive copies the snapshot's layers into a candidate, so that a caller
// replacing one layer keeps the others.
func (s *snapshot) derive() *snapshot {
	return &snapshot{
		file:      s.file,
		remote:    s.remote,
		remoteErr: s.remoteErr,
		docs:      s.docs,
		issues:    s.issues,
	}
}

// swapState is the publishing half of the resolver: the current snapshot and
// the lock that serialises candidates.
//
// Readers never take the lock — they load the pointer. The lock exists so that
// two reloaders cannot hand out the same revision number or interleave the
// stamping of documents.
type swapState struct {
	mu      sync.Mutex
	current atomic.Pointer[snapshot]
}

// view returns the snapshot readable right now.
func (s *swapState) view() *snapshot { return s.current.Load() }

// revision returns the revision readable right now. Zero means nothing has
// been published yet.
func (s *swapState) revision() uint64 { return s.view().revision }

// apply validates a candidate and publishes it only if it passes.
//
// On success it returns the new revision. On failure it returns the revision
// still in force and the validation error, having published nothing: the
// application keeps serving the last good configuration rather than coming up
// on half of a new one.
func (s *swapState) apply(candidate *snapshot, validate func(*snapshot) error) (uint64, error) {
	if candidate == nil {
		return s.revision(), errNilCandidate
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.current.Load()
	if validate != nil {
		if err := validate(candidate); err != nil {
			return current.revision, err
		}
	}

	candidate.revision = current.revision + 1
	stampDocuments(candidate, current)
	s.current.Store(candidate)

	return candidate.revision, nil
}

// stampDocuments gives every document the revision its content arrived in: the
// new one when the content changed, the one it already had when it did not.
//
// Without this a reload of ten prompts because one of them changed would look,
// to anything reading Revision, like ten changed prompts.
//
// The stamps go into a fresh map rather than into the candidate's own. A
// candidate derived from the current snapshot shares its document map with the
// revision that is still being read, and writing through that sharing is a
// data race against every reader holding it.
func stampDocuments(candidate, current *snapshot) {
	if len(candidate.docs) == 0 {
		return
	}

	stamped := make(map[DocumentID]Document, len(candidate.docs))
	for id, doc := range candidate.docs {
		if previous, ok := current.docs[id]; ok && previous.Hash == doc.Hash {
			doc.Revision = previous.Revision
		} else {
			doc.Revision = candidate.revision
		}
		stamped[id] = doc
	}
	candidate.docs = stamped
}

// documentChanges lists the documents that differ between two snapshots, by
// name: added, removed, or rewritten.
func documentChanges(previous, candidate *snapshot) []DocumentID {
	var changed []DocumentID

	for id, doc := range candidate.docs {
		if before, ok := previous.docs[id]; !ok || before.Hash != doc.Hash {
			changed = append(changed, id)
		}
	}
	for id := range previous.docs {
		if _, ok := candidate.docs[id]; !ok {
			changed = append(changed, id)
		}
	}

	return changed
}
