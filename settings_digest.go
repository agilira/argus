// settings_digest.go: the content identity of a revision
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// Revision is a counter, and a counter is local to one process: two instances
// running the same configuration have unrelated numbers, and a restart puts
// the count back to one. For an application that has to be able to say, later,
// which configuration and which prompts produced a given answer, that is not
// enough.
//
// The digest is the other half: a hash of what the revision holds, so that
// "this run used configuration 3f9c…" means the same thing on another machine
// and after a restart, and two instances can be compared by it.

package argus

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	flashflags "github.com/agilira/flash-flags"
)

// Digest returns the content identity of the revision in force.
//
// Two instances that resolve every key to the same value and hold the same
// documents have the same digest, whatever supplied those values: the same
// port from a file and from an environment variable is the same
// configuration.
//
// Two caveats, both deliberate. Values are compared as they render, so two
// spellings of one number are two values. And a key that exists only in the
// environment, named by no file, no flag and no default, cannot be enumerated
// and is therefore not part of the digest — Explain has the same blind spot,
// for the same reason.
func (s *Settings) Digest() string { return s.core.res.view().digest }

// keyUniverse is every key a layer can name, sorted.
//
// The environment is not in it: a variable is only visible once something asks
// for that key, so an env-only key nobody declares is invisible to both this
// and Explain.
func (r *resolver) keyUniverse(view *snapshot) []string {
	seen := make(map[string]struct{})

	flat := make(map[string]interface{})
	flattenInto(flat, "", view.file)
	flattenInto(flat, "", view.remote)
	for key := range flat {
		seen[key] = struct{}{}
	}

	r.mu.RLock()
	for key := range r.overrides {
		seen[key] = struct{}{}
	}
	for key := range r.defaults {
		seen[key] = struct{}{}
	}
	flags := r.flags
	r.mu.RUnlock()

	if flags != nil {
		flags.VisitAll(func(flag *flashflags.Flag) { seen[flag.Name()] = struct{}{} })
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

// digestOf hashes what a revision resolves to: every key a layer names,
// through the whole precedence, and every document by name and hash.
func (r *resolver) digestOf(view *snapshot) string {
	digest := sha256.New()

	for _, key := range r.keyUniverse(view) {
		got := r.resolveIn(view, key)
		if !got.found {
			continue
		}
		writeDigestField(digest, key, r.renderValue(key, got))
	}

	ids := make([]DocumentID, 0, len(view.docs))
	for id := range view.docs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i].Group != ids[j].Group {
			return ids[i].Group < ids[j].Group
		}
		return ids[i].Name < ids[j].Name
	})
	for _, id := range ids {
		writeDigestField(digest, "doc:"+id.String(), view.docs[id].Hash)
	}

	return hex.EncodeToString(digest.Sum(nil))
}

// renderValue turns a resolved value into the text the digest compares.
func (r *resolver) renderValue(key string, got resolution) string {
	if got.fromFlag {
		r.mu.RLock()
		flags := r.flags
		r.mu.RUnlock()
		if flags != nil {
			if flag := flags.Lookup(key); flag != nil {
				return renderDigestValue(flag.Value())
			}
		}
		return ""
	}

	return renderDigestValue(got.value)
}

// renderDigestValue renders one value. A list renders as its elements, so that
// a list of one and the element itself are not the same thing.
func renderDigestValue(value interface{}) string {
	switch typed := value.(type) {
	case []string:
		return "[" + strings.Join(typed, "\x1f") + "]"
	case []interface{}:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, convertToString(item))
		}
		return "[" + strings.Join(parts, "\x1f") + "]"
	default:
		return convertToString(value)
	}
}

// writeDigestField adds one name and value to the hash, with separators that
// cannot appear in either.
func writeDigestField(digest interface{ Write([]byte) (int, error) }, name, value string) {
	_, _ = digest.Write([]byte(name))
	_, _ = digest.Write([]byte{0x1f})
	_, _ = digest.Write([]byte(value))
	_, _ = digest.Write([]byte{0x1e})
}
