// settings.go: Setup and Settings — the front door
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// Argus has about ten entry points, every one of them named after a mechanism:
// New+Watch, UniversalConfigWatcher, SimpleFileWatcher, WatchDirectory,
// WatchRemoteConfig, NewConfigManager, and so on. They all still work. This
// one is named after the job, and it is the only one the README advertises.
//
//	settings, err := argus.Setup("agent").
//	    File("config.json").
//	    Documents("prompts", "prompts/*.md").
//	    Env("AGENT_").
//	    Start()
//
// The model is sources -> merge -> snapshot -> access -> reload, with a fixed
// precedence that cannot be reordered:
//
//	explicit overrides > flags > environment > files > remote > defaults
//
// What watches the filesystem is not part of this API. Setup chooses it and
// Explain reports what it chose.

package argus

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	flashflags "github.com/agilira/flash-flags"
	"github.com/agilira/go-errors"
)

// SetupBuilder collects what an application wants to read its configuration
// from. Nothing happens until Start.
//
// Every method returns the builder, and a method that is never called is a
// source that is switched off: an application with no flags declares none, and
// one with no documents never pays for the document machinery.
type SetupBuilder struct {
	app string

	sources    configSources
	docSources []documentSource
	limits     documentLimits

	flags     *flashflags.FlagSet
	envPrefix string
	envOn     bool

	overrides map[string]interface{}
	defaults  map[string]interface{}

	auditor    *AuditLogger
	ownAuditor bool
	auditErr   error

	staleness time.Duration
	timeout   time.Duration

	onReload func(*Settings, Change)
	onError  func(error)

	backend changeBackend
}

// Setup starts describing where an application's configuration comes from.
// The name identifies the application in audit records and in Explain.
func Setup(app string) *SetupBuilder {
	return &SetupBuilder{
		app:       app,
		limits:    defaultDocumentLimits(),
		overrides: make(map[string]interface{}),
		defaults:  make(map[string]interface{}),
	}
}

// File reads a configuration file. Call it more than once for more than one
// file; a later file wins over an earlier one.
//
// The file must exist and must parse. A declared file that is missing or
// broken refuses the whole load rather than leaving the application running on
// half a configuration.
func (b *SetupBuilder) File(path string) *SetupBuilder {
	b.sources.files = append(b.sources.files, fileSource{path: path})
	return b
}

// FileIfPresent reads a configuration file when it is there, and carries on
// when it is not — the "/etc/myapp/config.json if somebody put one there"
// case, without the application having to stat it first.
//
// Absent is not an error. Present and unparseable still is: a file somebody
// wrote and got wrong is not the same as a file nobody wrote, and silently
// ignoring the first is how a typo becomes an outage nobody can explain.
//
// It sits in the same layer as File, in declaration order, so an optional file
// declared later overrides a required one declared earlier.
func (b *SetupBuilder) FileIfPresent(path string) *SetupBuilder {
	b.sources.files = append(b.sources.files, fileSource{path: path, optional: true})
	return b
}

// Dir reads every configuration file in a directory, merged in name order.
// This is the shape of a Kubernetes ConfigMap mount.
func (b *SetupBuilder) Dir(path string) *SetupBuilder {
	b.sources.dirs = append(b.sources.dirs, path)
	return b
}

// DirPatterns narrows what Dir considers a configuration file. The default is
// *.yaml, *.yml, *.json, *.toml and *.ini.
func (b *SetupBuilder) DirPatterns(patterns ...string) *SetupBuilder {
	b.sources.dirPatterns = patterns
	return b
}

// Env reads the environment, mapping a key to PREFIX_KEY with dots and dashes
// turned into underscores: under "APP_", server.port is APP_SERVER_PORT.
//
// An empty prefix is legal and reads the keys unprefixed. Calling Env at all
// is what switches the layer on.
func (b *SetupBuilder) Env(prefix string) *SetupBuilder {
	b.envPrefix = prefix
	b.envOn = true
	return b
}

// Flags puts an already-parsed flag set on top of the other sources.
//
// The flags stay the application's own: Argus does not declare them, print
// their help, or exit on -h. An application using cobra, pflag or the standard
// library feeds its parsed values through Overrides instead.
func (b *SetupBuilder) Flags(flags *flashflags.FlagSet) *SetupBuilder {
	b.flags = flags
	return b
}

// Overrides sets values that outrank every other source — the layer for an
// application that has already parsed its own command line.
func (b *SetupBuilder) Overrides(values map[string]interface{}) *SetupBuilder {
	for key, value := range values {
		b.overrides[key] = value
	}
	return b
}

// Defaults sets the values used when no source supplies a key.
func (b *SetupBuilder) Defaults(values map[string]interface{}) *SetupBuilder {
	for key, value := range values {
		b.defaults[key] = value
	}
	return b
}

// Remote reads a remote configuration provider, below the local files: the
// local file is the emergency override when the remote is wrong or
// unreachable.
//
// A remote that is down at start is not fatal when a local source is declared;
// it is fatal when the remote is the only source there is.
func (b *SetupBuilder) Remote(url string, opts ...*RemoteConfigOptions) *SetupBuilder {
	b.sources.remote = url
	if len(opts) > 0 {
		b.sources.remoteOpts = opts[0]
	}
	return b
}

// RemoteInterval is how often a remote provider is read again. The default is
// 30 seconds: a network call belongs on a slower clock than a stat of the
// local files.
func (b *SetupBuilder) RemoteInterval(d time.Duration) *SetupBuilder {
	b.sources.remoteInterval = d
	return b
}

// Documents reads text files as themselves: a file, or a directory and a glob.
// System prompts and skills are the reason this exists.
//
// They are never parsed and never enter the key space: Doc("prompts",
// "system") and GetString("system") are different things that cannot collide.
// A pattern holding ** is walked recursively, and those documents are named by
// their path under the directory rather than by their base name.
func (b *SetupBuilder) Documents(group, pattern string) *SetupBuilder {
	b.docSources = append(b.docSources, documentSource{group: group, pattern: pattern})
	return b
}

// RequiredDocuments is Documents for the ones an application cannot run
// without. A required document that is missing, oversized or unreadable
// refuses the whole load; an optional one is skipped and reported.
func (b *SetupBuilder) RequiredDocuments(group, pattern string) *SetupBuilder {
	b.docSources = append(b.docSources, documentSource{group: group, pattern: pattern, required: true})
	return b
}

// DocumentLimits changes the caps on documents. The defaults are 1 MB per
// document, 16 MB in total and 1000 documents — enough for a long prompt and a
// large skills directory, and small enough to stop a log file dropped into
// prompts/.
func (b *SetupBuilder) DocumentLimits(maxBytes, maxTotalBytes int64, maxDocuments int) *SetupBuilder {
	b.limits = documentLimits{maxBytes: maxBytes, maxTotalBytes: maxTotalBytes, maxDocuments: maxDocuments}
	return b
}

// Audit records every configuration change to the default audit trail.
func (b *SetupBuilder) Audit() *SetupBuilder {
	if b.auditor == nil && b.auditErr == nil {
		logger, err := NewAuditLogger(DefaultAuditConfig())
		if err != nil {
			b.auditErr = err
			return b
		}
		b.auditor = logger
		b.ownAuditor = true
	}
	return b
}

// AuditTo records every configuration change to an audit logger the
// application already has. Closing the settings flushes it; it does not close
// a logger it did not create.
func (b *SetupBuilder) AuditTo(logger *AuditLogger) *SetupBuilder {
	b.auditor = logger
	b.ownAuditor = false
	return b
}

// MaxStaleness is how long a change may go unnoticed. It is a budget, not a
// mechanism: how the budget is met — polling today, and something else later —
// is not part of this API, and Explain reports what was chosen.
func (b *SetupBuilder) MaxStaleness(d time.Duration) *SetupBuilder {
	b.staleness = d
	return b
}

// Timeout bounds the first load, so that a remote provider that is slow at
// boot fails with a clear error instead of hanging the application.
func (b *SetupBuilder) Timeout(d time.Duration) *SetupBuilder {
	b.timeout = d
	return b
}

// OnReload is called after a new revision is published, with what changed by
// name — never with the values, so that a prompt cannot travel into a log line
// through a callback argument.
//
// It is not called for the first load, which the application already has from
// Start, nor for a cycle in which nothing changed.
func (b *SetupBuilder) OnReload(callback func(*Settings, Change)) *SetupBuilder {
	b.onReload = callback
	return b
}

// OnError is called when a reload fails: the application keeps the revision it
// has, and this is how it learns that the one on disk is broken.
func (b *SetupBuilder) OnError(callback func(error)) *SetupBuilder {
	b.onError = callback
	return b
}

// Start loads everything once, synchronously, and returns the handle.
//
// It returns an error if the first load does not come together — a missing
// file, a prompt that is required and absent, a remote that is the only source
// and unreachable. An application must not come up on an empty configuration.
func (b *SetupBuilder) Start() (*Settings, error) {
	if b.auditErr != nil {
		return nil, errors.Wrap(b.auditErr, ErrCodeInvalidAuditConfig,
			"cannot open the audit trail")
	}

	res := newResolver()
	if b.flags != nil {
		res.useFlags(b.flags)
	}
	if b.envOn {
		res.useEnv(b.envPrefix)
	}
	for key, value := range b.overrides {
		res.setOverride(key, value)
	}
	for key, value := range b.defaults {
		res.setDefault(key, value)
	}

	sources := b.sources
	if len(b.docSources) > 0 {
		sources.docs = &documentStore{sources: b.docSources, limits: b.limits}
	}
	sources.remoteIsSole = sources.remote != "" && len(sources.files) == 0 && len(sources.dirs) == 0
	if sources.remoteInterval <= 0 {
		sources.remoteInterval = defaultRemoteInterval
	}

	settings := &Settings{core: &settingsCore{
		app:        b.app,
		res:        res,
		flags:      b.flags,
		auditor:    b.auditor,
		ownAuditor: b.ownAuditor,
		staleness:  b.staleness,
		callback:   b.onReload,
	}}

	backend := b.backend
	if backend == nil {
		backend = &pollBackend{interval: b.staleness}
	}

	settings.core.reloader = &reloader{
		sources:     &sources,
		res:         res,
		backend:     backend,
		loadTimeout: b.timeout,
	}
	settings.core.reloader.validate = settings.bindCandidate
	settings.core.reloader.onCommit = settings.commitBindings
	settings.core.reloader.onReload = settings.publish
	settings.core.reloader.onError = func(err error) {
		settings.auditRefusal(err)
		if b.onError != nil {
			b.onError(err)
		}
	}

	if err := settings.core.reloader.start(); err != nil {
		if auditErr := settings.closeAuditor(); auditErr != nil {
			return nil, errors.Wrap(err, ErrCodeInvalidConfig,
				"settings failed to start, and the audit trail did not close cleanly: "+auditErr.Error())
		}
		return nil, err
	}
	settings.auditStart()

	return settings, nil
}

// Settings is a running configuration: one revision of keys and documents,
// replaced whole when the sources change.
//
// It is read-only. An application that could write into it would be fighting
// its own reload; explicit values belong to the builder.
//
// Several handles may exist in one process and none of them is global: a
// library that uses Argus internally does not take the process over, and does
// not interfere with an application that uses Argus too.
type Settings struct {
	// core is shared by every view of these settings. Sub returns a handle
	// over the same core rather than a copy of its fields: a struct rebuilt
	// field by field at a boundary is a struct that silently loses the next
	// field somebody adds.
	core *settingsCore

	// prefix and sub are the only things a view has of its own.
	prefix string
	sub    bool
}

// settingsCore is one running configuration, shared by the handle and by every
// subtree taken from it.
type settingsCore struct {
	app        string
	res        *resolver
	reloader   *reloader
	flags      *flashflags.FlagSet
	auditor    *AuditLogger
	ownAuditor bool
	staleness  time.Duration

	// callback is assigned once, before the backend is started, and only read
	// afterwards. A handle whose reload callback could be swapped while it is
	// running would need a lock, and a lock in here would have to be shared
	// rather than copied — which is what core is for.
	callback func(*Settings, Change)

	// binders are the struct types this configuration is bound to. They are
	// registered after Start, by Bind, and run against every candidate: from
	// the moment an application binds a type, a revision that does not satisfy
	// it is not published.
	bindersMu sync.RWMutex
	binders   []revisionBinder

	closed atomic.Bool
}

// key applies a Sub's prefix.
func (s *Settings) key(name string) string {
	if s.prefix == "" {
		return name
	}
	return s.prefix + name
}

// GetString returns a string value, or "" when no source supplies the key.
func (s *Settings) GetString(key string) string { return s.reader().getString(key) }

// GetInt returns an integer value, or 0 when no source supplies the key.
func (s *Settings) GetInt(key string) int { return s.reader().getInt(key) }

// GetBool returns a boolean value, or false when no source supplies the key.
func (s *Settings) GetBool(key string) bool { return s.reader().getBool(key) }

// GetDuration returns a duration value, or 0 when no source supplies the key.
func (s *Settings) GetDuration(key string) time.Duration { return s.reader().getDuration(key) }

// GetFloat64 returns a floating point value, or 0 when no source supplies the
// key.
func (s *Settings) GetFloat64(key string) float64 { return s.reader().getFloat64(key) }

// GetStringSlice returns a list value, accepting either a real list or a
// comma-separated string, which is all an environment variable can carry.
func (s *Settings) GetStringSlice(key string) []string { return s.reader().getStringSlice(key) }

// Sub returns a view of one subtree: the settings of one agent, or of one
// tenant, read without repeating the prefix at every call.
//
// It shares the revision it is taken from and every later one. Documents are
// not prefixed — a group is already a namespace — and closing a subtree does
// nothing: the handle it came from owns the sources.
func (s *Settings) Sub(prefix string) *Settings {
	return &Settings{core: s.core, prefix: s.key(prefix) + ".", sub: true}
}

// Doc returns a document by group and name.
func (s *Settings) Doc(group, name string) (Document, bool) {
	return s.core.res.document(group, name)
}

// Docs returns every document in a group, in name order.
func (s *Settings) Docs(group string) []Document {
	view := s.core.res.view()

	docs := make([]Document, 0, len(view.docs))
	for id, doc := range view.docs {
		if id.Group == group {
			docs = append(docs, doc)
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Name < docs[j].Name })

	return docs
}

// Revision returns the revision in force. Everything readable at one instant
// belongs to it.
//
// It advances only when something actually changed. A file whose timestamp
// moved but whose content did not — which is every file under a Kubernetes
// ConfigMap mount each time kubelet remounts it — does not spend a revision.
func (s *Settings) Revision() uint64 { return s.core.res.revision() }

// Close stops watching and flushes the audit trail. It is safe to call more
// than once, which matters for a library whose own Close may be called twice
// by its caller. Closing a Sub does nothing.
//
// When it returns, no cycle is running and none will start. A callback already
// running is not waited for: an application that calls Close from inside
// OnReload would be waiting for itself.
func (s *Settings) Close() error {
	if s.sub || s.core.closed.Swap(true) {
		return nil
	}

	err := s.core.reloader.close()
	if auditErr := s.closeAuditor(); auditErr != nil && err == nil {
		err = auditErr
	}

	return err
}

// revisionBinder is a struct type kept in step with the configuration. Bound
// implements it; the cycle drives it in three steps so that a value is only
// published once its revision is.
type revisionBinder interface {
	prepare(view *snapshot) error
	commit(revision uint64)
	discard()
}

// addBinder registers a bound type with this configuration.
func (c *settingsCore) addBinder(binder revisionBinder) {
	c.bindersMu.Lock()
	defer c.bindersMu.Unlock()
	c.binders = append(c.binders, binder)
}

// bindCandidate builds every bound type against a candidate revision. An error
// refuses the candidate, and nothing is published.
func (s *Settings) bindCandidate(view *snapshot) error {
	s.core.bindersMu.RLock()
	defer s.core.bindersMu.RUnlock()

	for i, binder := range s.core.binders {
		if err := binder.prepare(view); err != nil {
			// Undo the ones already built: a candidate is all or nothing.
			for _, prepared := range s.core.binders[:i] {
				prepared.discard()
			}
			return err
		}
	}

	return nil
}

// commitBindings publishes the values built for a candidate that was accepted.
func (s *Settings) commitBindings(revision uint64) {
	s.core.bindersMu.RLock()
	defer s.core.bindersMu.RUnlock()

	for _, binder := range s.core.binders {
		binder.commit(revision)
	}
}

// publish is what the reloader calls: audit the swap, then hand it to the
// application's callback.
func (s *Settings) publish(change Change) {
	s.auditChange(change)

	if s.core.callback != nil {
		s.core.callback(s, change)
	}
}
