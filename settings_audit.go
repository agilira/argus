// settings_audit.go: the audit trail of a running configuration
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// One rule governs everything here: a change is recorded by name and hash,
// never by content. A system prompt that is copied into an audit database is a
// system prompt that has left the machine it was meant for — and the audit
// trail is the one place where "it is only configuration" stops being true.

package argus

import "strconv"

// auditStart records that an application came up, and on which revision.
func (s *Settings) auditStart() {
	if s.core.auditor == nil {
		return
	}

	view := s.core.res.view()
	s.core.auditor.Log(AuditInfo, "settings_started", "argus.settings", "", nil, nil,
		map[string]interface{}{
			"app":       s.core.app,
			"revision":  view.revision,
			"digest":    view.digest,
			"documents": len(view.docs),
		})
}

// auditChange records a published revision: which keys moved, and which
// documents, by name and hash.
func (s *Settings) auditChange(change Change) {
	if s.core.auditor == nil {
		return
	}

	context := map[string]interface{}{
		"app":      s.core.app,
		"revision": change.Revision,
		"digest":   s.core.res.view().digest,
	}
	if len(change.Keys) > 0 {
		context["keys"] = change.Keys
	}
	if len(change.Documents) > 0 {
		context["documents"] = s.documentFingerprints(change.Documents)
	}

	s.core.auditor.Log(AuditInfo, "settings_reloaded", "argus.settings", "", nil, nil, context)
}

// auditRefusal records a candidate that did not validate, and why. A refusal
// that leaves no trace is a configuration change that silently did not happen.
func (s *Settings) auditRefusal(err error) {
	if s.core.auditor == nil || err == nil {
		return
	}

	s.core.auditor.Log(AuditWarn, "settings_refused", "argus.settings", "", nil, nil,
		map[string]interface{}{
			"app":      s.core.app,
			"revision": s.core.res.revision(),
			"reason":   err.Error(),
		})
}

// documentFingerprints turns changed documents into what may be recorded: the
// group, the name, the hash and the size. A document that was deleted has no
// hash, which is how a deletion is told from a rewrite.
func (s *Settings) documentFingerprints(ids []DocumentID) []map[string]string {
	view := s.core.res.view()

	records := make([]map[string]string, 0, len(ids))
	for _, id := range ids {
		record := map[string]string{"group": id.Group, "name": id.Name}
		if doc, ok := view.docs[id]; ok {
			record["hash"] = doc.Hash
			record["size"] = strconv.FormatInt(doc.Size, 10)
		} else {
			record["state"] = "removed"
		}
		records = append(records, record)
	}

	return records
}

// closeAuditor flushes the audit trail, and closes it only when Argus opened
// it. A logger the application supplied belongs to the application.
//
// The error travels: an audit trail that could not be written is exactly what
// the caller of Close needs to hear about, and swallowing it here would leave
// a gap in the record that nothing else reports.
func (s *Settings) closeAuditor() error {
	if s.core.auditor == nil {
		return nil
	}

	if s.core.ownAuditor {
		return s.core.auditor.Close()
	}

	return s.core.auditor.Flush()
}
