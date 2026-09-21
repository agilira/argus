// settings_reload_test.go - tests for the reload cycle, coalescing and the backend seam
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// skipAhead moves the settings' clock forward, so that anything on a timer —
// the remote interval — falls due without the test waiting for it.
func skipAhead(s *Settings, d time.Duration) {
	offset := d
	s.core.reloader.now = func() time.Time { return time.Now().Add(offset) }
}

// handBackend is a change backend driven by the test instead of by a clock.
// Coalescing tested against wall-clock timing is a test that fails on a busy
// machine and proves nothing on a quiet one.
type handBackend struct {
	mu       sync.Mutex
	onChange func()
	closed   bool
}

func (b *handBackend) start(onChange func()) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onChange = onChange
	return nil
}

func (b *handBackend) close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	return nil
}

func (b *handBackend) describe() string { return "driven by hand" }

// tick is what a real backend does when it thinks something moved.
func (b *handBackend) tick() {
	b.mu.Lock()
	onChange := b.onChange
	b.mu.Unlock()
	if onChange != nil {
		onChange()
	}
}

// newTestReloader wires a reloader over a temporary application: one config
// file, one prompts directory.
func newTestReloader(t *testing.T) (*reloader, *handBackend, string) {
	t.Helper()
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "claude-opus-5", "temperature": 0.2}`)
	writeDoc(t, filepath.Join(dir, "prompts", "system.md"), "be careful")

	backend := &handBackend{}
	r := &reloader{
		sources: &configSources{
			files: []fileSource{{path: filepath.Join(dir, "config.json")}},
			docs: &documentStore{
				sources: []documentSource{{group: "prompts", pattern: filepath.Join(dir, "prompts", "*.md")}},
				limits:  defaultDocumentLimits(),
			},
		},
		res:     newResolver(),
		backend: backend,
	}

	return r, backend, dir
}

func TestReload_FirstLoadIsSynchronousAndLoud(t *testing.T) {
	r, _, dir := newTestReloader(t)

	if err := r.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = r.close() }()

	if r.res.revision() != 1 {
		t.Errorf("revision after start = %d, want 1", r.res.revision())
	}
	if got := r.res.resolve("model").value; got != "claude-opus-5" {
		t.Errorf("model = %v", got)
	}

	// And a broken file at start is an error, not an application that comes up
	// on an empty configuration.
	writeDoc(t, filepath.Join(dir, "broken.json"), `{"model":`)
	broken := &reloader{
		sources: &configSources{files: []fileSource{{path: filepath.Join(dir, "broken.json")}}},
		res:     newResolver(),
		backend: &handBackend{},
	}
	if err := broken.start(); err == nil {
		t.Error("start accepted a configuration file that does not parse")
	}
	if broken.res.revision() != 0 {
		t.Error("a failed start published a revision")
	}
}

// TestReload_CoalescesASingleCycle: five documents saved in one go are one
// revision. Too many swaps looks like nothing at all from the outside, which
// is why this is asserted rather than watched.
func TestReload_CoalescesASingleCycle(t *testing.T) {
	r, backend, dir := newTestReloader(t)
	var changes []Change
	r.onReload = func(c Change) { changes = append(changes, c) }

	if err := r.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = r.close() }()

	for _, name := range []string{"one", "two", "three", "four", "five"} {
		writeDoc(t, filepath.Join(dir, "prompts", name+".md"), "skill "+name)
	}
	backend.tick()

	if got := r.res.revision(); got != 2 {
		t.Errorf("revision = %d, want 2: five files saved together are one swap", got)
	}
	if len(changes) != 1 {
		t.Fatalf("%d reload callbacks, want 1", len(changes))
	}
	if len(changes[0].Documents) != 5 {
		t.Errorf("Change lists %d documents, want 5: %+v", len(changes[0].Documents), changes[0].Documents)
	}
	if !changes[0].HasDocument("prompts", "three") {
		t.Error("Change does not mention prompts/three")
	}
}

// TestReload_NothingMovedNothingPublished: a cycle that finds the same files
// must not burn a revision, or a service that logs every reload becomes a
// service that logs every poll.
func TestReload_NothingMovedNothingPublished(t *testing.T) {
	r, backend, _ := newTestReloader(t)
	reloads := 0
	r.onReload = func(Change) { reloads++ }

	if err := r.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = r.close() }()

	for i := 0; i < 5; i++ {
		backend.tick()
	}

	if got := r.res.revision(); got != 1 {
		t.Errorf("revision = %d after five quiet cycles, want 1", got)
	}
	if reloads != 0 {
		t.Errorf("%d reload callbacks with nothing changed", reloads)
	}
}

// TestReload_ChangedKeysAreNamed: an application that reloads a connection
// pool wants to know whether the pool size moved, not that "something" did.
func TestReload_ChangedKeysAreNamed(t *testing.T) {
	r, backend, dir := newTestReloader(t)
	var change Change
	r.onReload = func(c Change) { change = c }

	if err := r.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = r.close() }()

	writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "claude-opus-5", "temperature": 0.9}`)
	backend.tick()

	if !change.HasKey("temperature") {
		t.Errorf("Change.Keys = %v, want temperature", change.Keys)
	}
	if change.HasKey("model") {
		t.Errorf("Change.Keys = %v, but model did not move", change.Keys)
	}
}

// TestReload_RefusedCandidateKeepsTheLastGoodRevision: somebody saves a broken
// file over a running application. It keeps serving what it had, says so, and
// recovers by itself when the file is fixed.
func TestReload_RefusedCandidateKeepsTheLastGoodRevision(t *testing.T) {
	r, backend, dir := newTestReloader(t)
	var reported []error
	r.onError = func(err error) { reported = append(reported, err) }

	if err := r.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = r.close() }()

	config := filepath.Join(dir, "config.json")
	writeDoc(t, config, `{"model": "claude-opus-`)
	backend.tick()

	if got := r.res.revision(); got != 1 {
		t.Errorf("revision = %d, want the unchanged 1", got)
	}
	if got := r.res.resolve("model").value; got != "claude-opus-5" {
		t.Errorf("model = %v, want the last good value", got)
	}
	if len(reported) != 1 {
		t.Fatalf("%d errors reported, want 1", len(reported))
	}

	writeDoc(t, config, `{"model": "claude-opus-5", "temperature": 0.4}`)
	backend.tick()

	if got := r.res.revision(); got != 2 {
		t.Errorf("revision = %d after the file was fixed, want 2", got)
	}
}

// TestReload_BackendIsReplaceable is the seam fsnotify arrives through in
// v1.8.0: a different backend drives the same cycle, and nothing above it
// changes. The builder never grows a knob for which one is in use.
func TestReload_BackendIsReplaceable(t *testing.T) {
	r, _, dir := newTestReloader(t)

	// A backend that notices changes its own way — here, a channel somebody
	// else writes to.
	signal := make(chan struct{}, 1)
	r.backend = &channelBackend{signal: signal}

	published := make(chan uint64, 4)
	r.onReload = func(c Change) { published <- c.Revision }

	if err := r.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = r.close() }()

	writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "claude-opus-5", "temperature": 0.7}`)
	signal <- struct{}{}

	select {
	case rev := <-published:
		if rev != 2 {
			t.Errorf("published revision %d, want 2", rev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the replacement backend never drove a cycle")
	}
}

// channelBackend watches nothing: it reloads when somebody says so.
type channelBackend struct {
	signal chan struct{}
	done   chan struct{}
	once   sync.Once
}

func (b *channelBackend) start(onChange func()) error {
	b.done = make(chan struct{})
	go func() {
		for {
			select {
			case <-b.done:
				return
			case <-b.signal:
				onChange()
			}
		}
	}()
	return nil
}

func (b *channelBackend) close() error {
	b.once.Do(func() { close(b.done) })
	return nil
}

func (b *channelBackend) describe() string { return "driven by a channel" }

// TestReload_PollBackendStops: the shipped backend is a poller, and a poller
// that outlives its handle is a goroutine leak in every test that uses Argus.
func TestReload_PollBackendStops(t *testing.T) {
	ticks := make(chan struct{}, 16)
	backend := &pollBackend{interval: time.Millisecond}

	if err := backend.start(func() { ticks <- struct{}{} }); err != nil {
		t.Fatalf("start: %v", err)
	}

	select {
	case <-ticks:
	case <-time.After(5 * time.Second):
		t.Fatal("the poller never ticked")
	}

	if err := backend.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// Draining what is already queued, the poller must go quiet.
	for len(ticks) > 0 {
		<-ticks
	}
	time.Sleep(20 * time.Millisecond)
	if len(ticks) > 0 {
		t.Error("the poller kept ticking after close")
	}
	if err := backend.close(); err != nil {
		t.Errorf("second close: %v", err)
	}
}

// TestReload_DocumentDeletionIsAChange: a skill file removed from the
// directory is a change like any other, and the document goes away.
func TestReload_DocumentDeletionIsAChange(t *testing.T) {
	r, backend, dir := newTestReloader(t)
	writeDoc(t, filepath.Join(dir, "prompts", "tone.md"), "warm")

	var change Change
	r.onReload = func(c Change) { change = c }
	if err := r.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = r.close() }()

	if err := os.Remove(filepath.Join(dir, "prompts", "tone.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	backend.tick()

	if !change.HasDocument("prompts", "tone") {
		t.Errorf("Change does not mention the deleted document: %+v", change)
	}
	if _, ok := r.res.document("prompts", "tone"); ok {
		t.Error("the deleted document is still readable")
	}
}

// TestReload_TouchDoesNotBurnARevision: a file whose timestamp moves but whose
// content does not is not a change. Kubernetes remounts a ConfigMap by
// swapping the ..data symlink, which moves every timestamp underneath it, and
// an application logging "now on revision N" would see churn that means
// nothing.
func TestReload_TouchDoesNotBurnARevision(t *testing.T) {
	r, backend, dir := newTestReloader(t)
	reloads := 0
	r.onReload = func(Change) { reloads++ }

	if err := r.start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = r.close() }()

	config := filepath.Join(dir, "config.json")
	// Same bytes, new timestamp.
	later := time.Now().Add(time.Second)
	if err := os.Chtimes(config, later, later); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	backend.tick()

	if got := r.res.revision(); got != 1 {
		t.Errorf("revision = %d after a touch, want the unchanged 1", got)
	}
	if reloads != 0 {
		t.Errorf("%d reload callbacks for a file that did not change", reloads)
	}

	// And a real change still lands.
	writeDoc(t, config, `{"model": "claude-opus-5", "temperature": 0.9}`)
	backend.tick()
	if got := r.res.revision(); got != 2 {
		t.Errorf("revision = %d after a real change, want 2", got)
	}
}

// TestReload_RecoversWhenTheRemoteComesBack: nothing on disk changes, so the
// only thing that can bring the remote values in is the remote's own clock.
func TestReload_RecoversWhenTheRemoteComesBack(t *testing.T) {
	provider := failingProvider("reloadretries", map[string]interface{}{"port": 8080})
	url := useRemoteProvider(t, provider)

	backend := &handBackend{}
	failures := make(chan error, 8)
	settings, err := Setup("service").
		File(writeDoc(t, filepath.Join(t.TempDir(), "config.json"), `{"local": true}`)).
		Remote(url, &RemoteConfigOptions{RetryAttempts: 1, RetryDelay: time.Millisecond}).
		OnError(func(err error) { failures <- err }).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	settings.core.reloader.backend = backend
	skipAhead(settings, time.Hour)
	_ = backend.start(settings.core.reloader.cycle)

	if got := settings.GetInt("port"); got != 0 {
		t.Fatalf("port = %d before the remote came up", got)
	}

	// Nothing on disk changes. The remote starts answering.
	provider.fail.Store(false)
	backend.tick()

	if got := settings.GetInt("port"); got != 8080 {
		t.Errorf("port = %d: the cycle never read the remote again", got)
	}
	if issues := settings.Explain().Issues; len(issues) != 0 {
		t.Errorf("the recovered remote is still an issue: %v", issues)
	}
}

// TestReload_RefreshesTheRemote: a remote provider is a source that changes
// without anything happening on disk. A cycle that only looks at files would
// read it once at Start and never again.
func TestReload_RefreshesTheRemote(t *testing.T) {
	provider := &settingsRemoteProvider{
		scheme: "reloadrefreshes",
		values: map[string]interface{}{"port": 8080},
	}
	url := useRemoteProvider(t, provider)

	backend := &handBackend{}
	settings, err := Setup("service").
		File(writeDoc(t, filepath.Join(t.TempDir(), "config.json"), `{"local": true}`)).
		Remote(url).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	settings.core.reloader.backend = backend
	skipAhead(settings, time.Hour)
	_ = backend.start(settings.core.reloader.cycle)

	if got := settings.GetInt("port"); got != 8080 {
		t.Fatalf("port = %d at start", got)
	}

	provider.mu.Lock()
	provider.values = map[string]interface{}{"port": 9090}
	provider.mu.Unlock()
	backend.tick()

	if got := settings.GetInt("port"); got != 9090 {
		t.Errorf("port = %d: the remote was read once and never again", got)
	}
	if settings.Explain().KeySource("port") != SourceRemote {
		t.Error("the value is not coming from the remote")
	}
}

// TestReload_ReportsARepeatedFailureOnce: a file that stays broken must not
// fill the log at every poll, and a different failure must still be heard.
func TestReload_ReportsARepeatedFailureOnce(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)

	backend := &handBackend{}
	failures := make(chan error, 8)
	settings, err := Setup("service").
		File(config).
		OnError(func(err error) { failures <- err }).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	settings.core.reloader.backend = backend
	_ = backend.start(settings.core.reloader.cycle)

	writeDoc(t, config, `{"port": `)
	for i := 0; i < 3; i++ {
		backend.tick()
	}

	if len(failures) != 1 {
		t.Errorf("%d errors for one broken file, want 1", len(failures))
	}
	<-failures

	// A different failure is a different thing to say.
	if err := os.Remove(config); err != nil {
		t.Fatalf("remove: %v", err)
	}
	backend.tick()
	if len(failures) != 1 {
		t.Errorf("%d errors after the file went away, want 1", len(failures))
	}
}

// TestReload_LocalChangeDoesNotCallTheRemote: a file save must not turn into a
// network call. The remote has its own clock, and a poll interval of 20 ms
// would otherwise hammer a provider twenty times a second.
func TestReload_LocalChangeDoesNotCallTheRemote(t *testing.T) {
	provider := &settingsRemoteProvider{
		scheme: "reloadlocalonly",
		values: map[string]interface{}{"remote_key": "value"},
	}
	url := useRemoteProvider(t, provider)

	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"local": 1}`)

	backend := &handBackend{}
	settings, err := Setup("service").
		File(config).
		Remote(url).
		RemoteInterval(time.Hour).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	settings.core.reloader.backend = backend
	_ = backend.start(settings.core.reloader.cycle)

	loadsAfterStart := provider.loads.Load()

	for i := 0; i < 5; i++ {
		writeDoc(t, config, `{"local": `+string(rune('1'+i))+`}`)
		backend.tick()
	}

	if got := provider.loads.Load(); got != loadsAfterStart {
		t.Errorf("the remote was called %d times for local file changes", got-loadsAfterStart)
	}
	// And the values it supplied at start are still there.
	if got := settings.GetString("remote_key"); got != "value" {
		t.Errorf("remote_key = %q: the remote layer was dropped when it was not refetched", got)
	}
}

// TestReload_RemoteFailingLaterKeepsItsLastValues: a provider that answered
// once and then goes quiet must not take its keys with it. The application
// keeps serving what the remote last said, Explain reports the failure, and a
// key that only the remote supplies does not vanish over a network blip.
func TestReload_RemoteFailingLaterKeepsItsLastValues(t *testing.T) {
	provider := &settingsRemoteProvider{
		scheme: "reloadremotestale",
		values: map[string]interface{}{"remote_key": "from-remote"},
	}
	url := useRemoteProvider(t, provider)

	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"local": 1}`)

	backend := &handBackend{}
	settings, err := Setup("service").
		File(config).
		Remote(url, &RemoteConfigOptions{RetryAttempts: 1, RetryDelay: time.Millisecond}).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	settings.core.reloader.backend = backend
	skipAhead(settings, time.Hour)
	_ = backend.start(settings.core.reloader.cycle)

	if got := settings.GetString("remote_key"); got != "from-remote" {
		t.Fatalf("remote_key = %q at start", got)
	}

	provider.fail.Store(true)
	backend.tick()

	if got := settings.GetString("remote_key"); got != "from-remote" {
		t.Errorf("remote_key = %q: a failed fetch threw away what the remote had already said", got)
	}
	issues := settings.Explain().Issues
	if len(issues) == 0 {
		t.Error("a remote that stopped answering is not reported")
	}
}

// TestExplain_ReportsAStaleRemote: "failed" and "stale" are different things
// to an operator. Failed means no values; stale means the ones in force are
// the last the remote gave.
func TestExplain_ReportsAStaleRemote(t *testing.T) {
	provider := &settingsRemoteProvider{
		scheme: "explainstaleremote",
		values: map[string]interface{}{"remote_key": "from-remote"},
	}
	url := useRemoteProvider(t, provider)

	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"local": 1}`)

	backend := &handBackend{}
	settings, err := Setup("service").
		File(config).
		Remote(url, &RemoteConfigOptions{RetryAttempts: 1, RetryDelay: time.Millisecond}).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	settings.core.reloader.backend = backend
	skipAhead(settings, time.Hour)
	_ = backend.start(settings.core.reloader.cycle)

	provider.fail.Store(true)
	backend.tick()

	var seen string
	for _, state := range settings.Explain().Sources {
		if state.Source == SourceRemote {
			seen = state.State
		}
	}
	if seen != "stale" {
		t.Errorf("remote state = %q, want stale", seen)
	}
}
