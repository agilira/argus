// settings_reload.go: the reload cycle and the backend that drives it
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// The backend is deliberately behind an interface that the public API does not
// mention. Setup chooses it and Explain reports what it chose; the builder
// never grows a .Poll() or .Notify() knob. That is what lets fsnotify arrive
// later as pure addition, changing latency and one line of Explain rather than
// the API and the README.

package argus

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// defaultStaleness is how long a change may go unnoticed when the application
// does not say. A second is quiet on the CPU even with a thousand files —
// 1000 files at 200 ms costs about 8.6% of a core, and this is five times
// slower than that — and fast enough that nobody watches a prompt not reload.
const defaultStaleness = time.Second

// defaultRemoteInterval is how often a remote provider is read again when the
// application does not say. A remote is a network call and a change there is
// somebody else's deploy, not a file save: it belongs on a slower clock than
// the stat pass.
const defaultRemoteInterval = 30 * time.Second

// defaultLoadTimeout bounds one load. A remote provider that is slow must not
// hang the application, at boot or later, with no explanation.
const defaultLoadTimeout = 10 * time.Second

// changeBackend notices that something may have moved and says so. What it
// watches, and how, is its own business: the cycle it drives stats the
// declared paths itself and does nothing when they have not changed, so a
// backend that cries wolf costs a fingerprint and nothing more.
type changeBackend interface {
	start(onChange func()) error
	close() error
	describe() string
}

// pollBackend is the backend that ships: a ticker.
type pollBackend struct {
	interval time.Duration

	mu     sync.Mutex
	done   chan struct{}
	closed bool
}

func (b *pollBackend) start(onChange func()) error {
	b.mu.Lock()
	if b.interval <= 0 {
		b.interval = defaultStaleness
	}
	b.done = make(chan struct{})
	done := b.done
	interval := b.interval
	b.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				onChange()
			}
		}
	}()

	return nil
}

func (b *pollBackend) close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed || b.done == nil {
		b.closed = true
		return nil
	}
	close(b.done)
	b.closed = true

	return nil
}

func (b *pollBackend) describe() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	interval := b.interval
	if interval <= 0 {
		interval = defaultStaleness
	}

	return "polling every " + interval.String()
}

// reloader owns one cycle: notice, build, validate, publish.
type reloader struct {
	sources *configSources
	res     *resolver
	backend changeBackend

	// validate runs against a candidate before it is published, and refuses it
	// by returning an error. onCommit runs after a candidate is published.
	// Together they are how a bound struct is built from a revision that may
	// still be rejected.
	validate func(*snapshot) error
	onCommit func(uint64)

	onReload func(Change)
	onError  func(error)

	loadTimeout time.Duration

	// mu serialises cycles. Callbacks run outside it: user code that calls
	// Close from inside OnReload would otherwise deadlock against its own
	// reload.
	mu sync.Mutex

	// lastPrint is what the declared paths looked like at the last cycle.
	lastPrint string

	// lastRemote is when the remote provider was last read.
	lastRemote time.Time

	// lastError is the failure already reported. A file that stays broken is
	// worth saying once, not at every poll.
	lastError string

	// refusal is the last candidate that did not become a revision, readable
	// without the lock: Explain is what a debug endpoint calls, and a cycle
	// waiting on a slow remote must not hold it up.
	refusal atomic.Pointer[Refusal]

	closeOnce sync.Once
}

// cycleOutcome is what one cycle did, decided under the lock and acted on
// outside it.
type cycleOutcome struct {
	published bool
	change    Change
	err       error

	// report is false for a failure identical to the one already reported.
	report bool
}

// start performs the first load synchronously and then hands the cycle to the
// backend.
//
// The first load is loud on purpose. An application that comes up on an empty
// configuration because its file was missing will fail later, somewhere else,
// in a way nobody traces back to here.
func (r *reloader) start() error {
	if r.backend == nil {
		r.backend = &pollBackend{interval: defaultStaleness}
	}

	if outcome := r.attempt(true); outcome.err != nil {
		return outcome.err
	}

	return r.backend.start(r.cycle)
}

// close stops the backend and waits for a cycle already in flight.
//
// It is safe to call more than once. What it does not wait for is the
// application's own callback: a callback that called Close would then be
// waiting for itself.
func (r *reloader) close() error {
	var err error
	r.closeOnce.Do(func() {
		if r.backend != nil {
			err = r.backend.close()
		}

		// The backend has stopped handing out ticks; this waits for the cycle
		// that was already running, so that no revision is being published
		// when Close returns.
		r.awaitCycle()
	})

	return err
}

// awaitCycle returns once no cycle is running. Taking the lock and giving it
// back is the whole of it: the cycle holds that lock from the first stat to
// the last swap.
func (r *reloader) awaitCycle() {
	r.mu.Lock()
	defer r.mu.Unlock()
}

// pause holds the cycle still while fn runs.
//
// Bind needs this: it reads the revision in force, builds a value from it and
// registers itself, and a swap landing between the reading and the registering
// would leave a bound value one revision behind with nothing to correct it.
func (r *reloader) pause(fn func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return fn()
}

// cycle is what the backend calls. Errors go to the error handler, because
// there is nobody else left to return them to.
func (r *reloader) cycle() {
	r.notify(r.attempt(false))
}

// notify runs the application's callbacks, outside the lock: user code that
// calls Close from inside OnReload would otherwise deadlock against its own
// reload.
//
// onReload and onError are assigned once, before the backend is started, and
// only read afterwards.
func (r *reloader) notify(outcome cycleOutcome) {
	switch {
	case outcome.err != nil:
		if outcome.report && r.onError != nil {
			r.onError(outcome.err)
		}
	case outcome.published && r.onReload != nil &&
		(len(outcome.change.Keys) > 0 || len(outcome.change.Documents) > 0):
		r.onReload(outcome.change)
	}
}

// attempt runs one cycle under the lock and says what it did.
//
// The whole decision is taken here so that there is one place that can hold
// the lock and one place that can release it.
func (r *reloader) attempt(initial bool) cycleOutcome {
	r.mu.Lock()
	defer r.mu.Unlock()

	if initial {
		// The first load always builds, and remembers what it saw: without
		// this the first tick after Start rebuilds everything for nothing.
		r.lastPrint = r.sources.fingerprint()
	} else if !r.worthBuilding() {
		return cycleOutcome{}
	}

	candidate, err := r.buildCandidate(initial)
	if err != nil {
		return r.fail(err)
	}

	previous := r.res.view()
	change := Change{
		Keys:      changedKeys(previous, candidate),
		Documents: documentChanges(previous, candidate),
	}

	// The fingerprint moved but nothing in it did: a file was touched, or
	// Kubernetes remounted a ConfigMap by swapping the ..data symlink, which
	// moves every timestamp underneath it. Publishing that would advance the
	// revision of an application whose configuration is exactly what it was.
	if !initial && len(change.Keys) == 0 && len(change.Documents) == 0 &&
		sameIssues(previous.issues, candidate.issues) {
		r.succeed()
		return cycleOutcome{}
	}

	// The content identity is worked out before the swap, so that a reader of
	// the published revision never finds it missing.
	candidate.digest = r.res.digestOf(candidate)

	// A file that does not parse, a required document that is absent or a cap
	// that is exceeded have already refused the candidate by this point. What
	// is left is validation that needs the assembled revision: the struct
	// types the application has bound.
	revision, err := r.res.apply(candidate, r.validate)
	if err != nil {
		return r.fail(err)
	}
	if r.onCommit != nil {
		r.onCommit(revision)
	}
	change.Revision = revision
	r.succeed()

	return cycleOutcome{published: true, change: change}
}

// worthBuilding decides whether anything can have changed since the last
// cycle. It runs at the poll interval, so it only stats.
//
// A cycle that failed is not retried on its own: a file that does not parse is
// tried again when somebody fixes it, which moves the fingerprint, and a
// remote that is down is tried again when it falls due. Retrying anything else
// on a timer would mean calling the network at the poll interval.
func (r *reloader) worthBuilding() bool {
	if r.remoteDue() {
		return true
	}

	print := r.sources.fingerprint()
	if print == r.lastPrint {
		return false
	}
	r.lastPrint = print

	return true
}

// remoteDue reports whether the remote provider should be read again. It
// changes without anything happening on disk, so it has a clock of its own.
func (r *reloader) remoteDue() bool {
	return r.sources.remote != "" && time.Since(r.lastRemote) >= r.sources.remoteInterval
}

// buildCandidate assembles one candidate revision, bounded by the load
// timeout so that a slow remote cannot hold the cycle for ever.
func (r *reloader) buildCandidate(initial bool) (*snapshot, error) {
	timeout := r.loadTimeout
	if timeout <= 0 {
		timeout = defaultLoadTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fetchRemote := initial || r.remoteDue()

	candidate, _, err := r.sources.build(ctx, r.res.view(), fetchRemote)
	if err != nil {
		return nil, err
	}
	if fetchRemote {
		r.lastRemote = time.Now()
	}

	return candidate, nil
}

// fail records a candidate that did not become a revision, and says whether
// the failure is worth reporting. The fingerprint was remembered before the
// build, so a file that stays broken is reported once rather than at every
// poll — but it is recorded every time, because "failing since 14:02, twelve
// cycles" and "failed once" are different situations.
func (r *reloader) fail(err error) cycleOutcome {
	cycles := 1
	if previous := r.refusal.Load(); previous != nil {
		cycles = previous.Cycles + 1
	}
	r.refusal.Store(&Refusal{
		Reason:   err.Error(),
		At:       time.Now(),
		Cycles:   cycles,
		Revision: r.res.revision(),
	})

	message := err.Error()
	if message == r.lastError {
		return cycleOutcome{err: err}
	}
	r.lastError = message

	return cycleOutcome{err: err, report: true}
}

// succeed records a cycle that came to an end without an error. The state says
// what is true now, so a refusal that has been overtaken is gone.
func (r *reloader) succeed() {
	r.lastError = ""
	r.refusal.Store(nil)
}

// sameIssues reports whether two revisions had the same trouble. A remote that
// came back up is a change worth publishing even when no key moved, because
// the values are no longer the emergency ones.
func sameIssues(previous, candidate []sourceIssue) bool {
	if len(previous) != len(candidate) {
		return false
	}
	for i := range previous {
		if previous[i] != candidate[i] {
			return false
		}
	}
	return true
}

// changedKeys names the keys whose value differs between two revisions, in
// dotted form, so that an application watching "server.port" is told about
// "server.port" rather than about "server".
func changedKeys(previous, candidate *snapshot) []string {
	before := make(map[string]interface{})
	flattenInto(before, "", previous.file)
	flattenInto(before, "", previous.remote)

	after := make(map[string]interface{})
	flattenInto(after, "", candidate.file)
	flattenInto(after, "", candidate.remote)

	var changed []string
	for key, value := range after {
		if old, ok := before[key]; !ok || !reflect.DeepEqual(old, value) {
			changed = append(changed, key)
		}
	}
	for key := range before {
		if _, ok := after[key]; !ok {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)

	return changed
}

// flattenInto walks nested maps into dotted keys.
func flattenInto(into map[string]interface{}, prefix string, values map[string]interface{}) {
	for key, value := range values {
		full := key
		if prefix != "" {
			full = prefix + "." + key
		}
		if nested, ok := value.(map[string]interface{}); ok {
			flattenInto(into, full, nested)
			continue
		}
		into[full] = value
	}
}
