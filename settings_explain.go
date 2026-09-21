// settings_explain.go: what this instance is running, and why
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// Explain returns a struct rather than a string. A string is a debug toy; a
// struct can be logged as JSON or served on a debug endpoint, which for a
// service that reloads its own prompts is the difference between "the config
// is wrong" and "this pod is on revision 7 and its remote has been down since
// the deploy".

package argus

import (
	"os"
	"time"
)

// Refusal is a candidate revision that did not hold together.
//
// An application with no OnError handler would otherwise have no way of
// knowing that what it is serving is no longer what is on disk: a refused
// candidate never becomes a revision, so it appears nowhere else.
type Refusal struct {
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`

	// Cycles is how many consecutive cycles have been refused. A remote that
	// has been failing for an hour and one that failed once read differently.
	Cycles int `json:"cycles"`

	// Revision is the one still in force, which is what the application is
	// actually serving.
	Revision uint64 `json:"revision"`
}

// SourceStatus is one configuration source and how it is doing.
type SourceStatus struct {
	Source Source `json:"source"`
	Detail string `json:"detail,omitempty"`
	State  string `json:"state"` // loaded, failed, absent
}

// DocumentSummary counts what the documents cost, without naming their
// contents.
type DocumentSummary struct {
	Count  int            `json:"count"`
	Bytes  int64          `json:"bytes"`
	Groups map[string]int `json:"groups,omitempty"`
}

// Explanation is a snapshot of an instance's configuration state.
type Explanation struct {
	App      string `json:"app"`
	Revision uint64 `json:"revision"`

	// Digest is the content identity of the revision: the same values and the
	// same documents give the same digest on any machine.
	Digest string `json:"digest"`

	Backend      string            `json:"backend"`
	MaxStaleness time.Duration     `json:"max_staleness"`
	Keys         map[string]Source `json:"keys"`
	Sources      []SourceStatus    `json:"sources"`
	Documents    DocumentSummary   `json:"documents"`
	Issues       []string          `json:"issues,omitempty"`

	// LastRefusal is the candidate that did not become a revision, or nil when
	// the last cycle came to an end.
	LastRefusal *Refusal `json:"last_refusal,omitempty"`
}

// KeySource says which layer supplied a key. A key nobody supplied is
// SourceNone.
func (e Explanation) KeySource(key string) Source {
	if source, ok := e.Keys[key]; ok {
		return source
	}
	return SourceNone
}

// Explain reports the revision in force, where each key came from, how every
// source is doing, and what Setup chose to watch the filesystem with.
func (s *Settings) Explain() Explanation {
	view := s.core.res.view()

	explanation := Explanation{
		App:          s.core.app,
		Revision:     view.revision,
		Digest:       view.digest,
		MaxStaleness: s.core.staleness,
		Keys:         make(map[string]Source),
	}
	if explanation.MaxStaleness <= 0 {
		explanation.MaxStaleness = defaultStaleness
	}
	if s.core.reloader != nil && s.core.reloader.backend != nil {
		explanation.Backend = s.core.reloader.backend.describe()
	}

	// The key universe is every key a layer can enumerate. The environment
	// cannot be enumerated — a variable is only visible once a key asks for it
	// — so an env-only key appears here once some other layer names it, and is
	// readable either way.
	for _, key := range s.core.res.keyUniverse(view) {
		if got := s.core.res.resolveIn(view, key); got.found {
			explanation.Keys[key] = got.source
		}
	}

	if s.core.reloader != nil {
		explanation.LastRefusal = s.core.reloader.refusal.Load()
	}
	explanation.Sources = s.sourceStates(view)
	explanation.Documents = documentSummary(view)
	for _, issue := range view.issues {
		explanation.Issues = append(explanation.Issues, issue.Source.String()+": "+issue.Detail)
	}

	return explanation
}

// sourceStates reports every declared source and how the current revision
// found it.
func (s *Settings) sourceStates(view *snapshot) []SourceStatus {
	var states []SourceStatus

	sources := s.core.reloader.sources
	for _, dir := range sources.dirs {
		states = append(states, SourceStatus{Source: SourceFile, Detail: dir, State: "loaded"})
	}
	for _, file := range sources.files {
		state := "loaded"
		if file.optional {
			if _, err := os.Stat(file.path); err != nil {
				state = "absent"
			}
		}
		states = append(states, SourceStatus{Source: SourceFile, Detail: file.path, State: state})
	}
	if sources.remote != "" {
		state := "loaded"
		switch {
		case view.remoteErr != "" && view.remote != nil:
			// Answered once, quiet now: the values in force are the ones it
			// last gave.
			state = "stale"
		case view.remoteErr != "" || view.remote == nil:
			state = "failed"
		}
		states = append(states, SourceStatus{Source: SourceRemote, Detail: sources.remote, State: state})
	}
	if sources.docs != nil {
		for _, doc := range sources.docs.sources {
			states = append(states, SourceStatus{
				Source: SourceFile,
				Detail: doc.group + ": " + doc.pattern,
				State:  "loaded",
			})
		}
	}

	s.core.res.mu.RLock()
	defer s.core.res.mu.RUnlock()
	if s.core.res.envEnabled {
		states = append(states, SourceStatus{Source: SourceEnv, Detail: s.core.res.envPrefix, State: "loaded"})
	}
	if s.core.res.flags != nil {
		states = append(states, SourceStatus{Source: SourceFlag, State: "loaded"})
	}

	return states
}

// documentSummary counts the documents in a revision, by group.
func documentSummary(view *snapshot) DocumentSummary {
	summary := DocumentSummary{Groups: make(map[string]int)}

	for id, doc := range view.docs {
		summary.Count++
		summary.Bytes += doc.Size
		summary.Groups[id.Group]++
	}

	return summary
}
