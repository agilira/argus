// settings_api_test.go - the rest of the Setup surface: remote, documents, audit
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	flashflags "github.com/agilira/flash-flags"
	"github.com/agilira/go-errors"
)

// settingsRemoteProvider serves a fixed configuration under its own scheme.
type settingsRemoteProvider struct {
	scheme string
	slow   time.Duration

	// mu guards values, and fail is flipped, while the reload goroutine reads
	// them.
	mu     sync.Mutex
	values map[string]interface{}
	fail   atomic.Bool

	// loads counts the calls that actually reached the provider.
	loads atomic.Int64
}

// failingProvider is a provider that is down until somebody says otherwise.
func failingProvider(scheme string, values map[string]interface{}) *settingsRemoteProvider {
	provider := &settingsRemoteProvider{scheme: scheme, values: values}
	provider.fail.Store(true)
	return provider
}

func (p *settingsRemoteProvider) Name() string   { return "settings-test-" + p.scheme }
func (p *settingsRemoteProvider) Scheme() string { return p.scheme }

func (p *settingsRemoteProvider) Validate(string) error { return nil }

func (p *settingsRemoteProvider) Load(ctx context.Context, _ string) (map[string]interface{}, error) {
	p.loads.Add(1)

	if p.slow > 0 {
		select {
		case <-time.After(p.slow):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if p.fail.Load() {
		return nil, errors.New(ErrCodeRemoteConfigError, "the remote is down")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return p.values, nil
}

func (p *settingsRemoteProvider) Watch(context.Context, string) (<-chan map[string]interface{}, error) {
	return nil, errors.New(ErrCodeRemoteConfigError, "watching is not part of this test")
}

func (p *settingsRemoteProvider) HealthCheck(context.Context, string) error { return nil }

// useRemoteProvider registers a provider under its own scheme and returns the
// URL to read it from.
//
// The provider registry is global and has no way to replace an entry, so each
// test brings its own scheme rather than fighting over one.
func useRemoteProvider(t *testing.T, provider *settingsRemoteProvider) string {
	t.Helper()
	if err := RegisterRemoteProvider(provider); err != nil {
		t.Fatalf("RegisterRemoteProvider(%s): %v", provider.scheme, err)
	}
	return provider.scheme + "://config"
}

func TestSettings_RemoteSitsBelowTheLocalFile(t *testing.T) {
	url := useRemoteProvider(t, &settingsRemoteProvider{
		scheme: "settingsremotebelow",
		values: map[string]interface{}{"port": 9999, "region": "remote-eu"},
	})

	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)

	settings, err := Setup("service").
		File(config).
		Remote(url).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	// The local file is the emergency override, so it wins.
	if got := settings.GetInt("port"); got != 8080 {
		t.Errorf("port = %d, want the local file's 8080", got)
	}
	// A key only the remote has is still read.
	if got := settings.GetString("region"); got != "remote-eu" {
		t.Errorf("region = %q, want the remote value", got)
	}
	if got := settings.Explain().KeySource("region"); got != SourceRemote {
		t.Errorf("KeySource(region) = %v", got)
	}
}

func TestSettings_RemoteDownIsSurvivableButReported(t *testing.T) {
	url := useRemoteProvider(t, failingProvider("settingsremotedown", nil))

	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)

	settings, err := Setup("service").
		File(config).
		Remote(url, &RemoteConfigOptions{RetryAttempts: 1, RetryDelay: time.Millisecond}).
		Start()
	if err != nil {
		t.Fatalf("Start: %v, want a start that survives an unreachable remote", err)
	}
	defer func() { _ = settings.Close() }()

	if got := settings.GetInt("port"); got != 8080 {
		t.Errorf("port = %d", got)
	}

	explanation := settings.Explain()
	if len(explanation.Issues) == 0 {
		t.Error("an unreachable remote left no trace in Explain")
	}
	for _, state := range explanation.Sources {
		if state.Source == SourceRemote && state.State != "failed" {
			t.Errorf("remote source state = %q, want failed", state.State)
		}
	}
}

func TestSettings_RemoteAloneAndDownIsFatal(t *testing.T) {
	url := useRemoteProvider(t, failingProvider("settingsremotesole", nil))

	// Nothing local supplies these keys: coming up would mean coming up on an
	// empty configuration.
	fast := &RemoteConfigOptions{RetryAttempts: 1, RetryDelay: time.Millisecond}
	if _, err := Setup("service").Remote(url, fast).Start(); err == nil {
		t.Error("Start succeeded with no configuration at all")
	}
}

func TestSettings_TimeoutBoundsTheFirstLoad(t *testing.T) {
	url := useRemoteProvider(t, &settingsRemoteProvider{scheme: "settingsremoteslow", slow: 5 * time.Second})

	started := time.Now()
	_, err := Setup("service").
		Remote(url).
		Timeout(200 * time.Millisecond).
		Start()

	if err == nil {
		t.Fatal("Start waited for a slow remote instead of giving up")
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Errorf("Start took %v; the timeout did not bound it", elapsed)
	}
}

func TestSettings_RequiredDocumentsAndLimits(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "opus"}`)
	prompts := filepath.Join(dir, "prompts")
	writeDoc(t, filepath.Join(prompts, "system.md"), "be careful")

	settings, err := Setup("agent").
		File(config).
		RequiredDocuments("prompts", filepath.Join(prompts, "*.md")).
		DocumentLimits(1024, 4096, 10).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, ok := settings.Doc("prompts", "system"); !ok {
		t.Error("the required document was not loaded")
	}
	_ = settings.Close()

	// The same group, required and absent, must not start.
	if _, err := Setup("agent").
		File(config).
		RequiredDocuments("prompts", filepath.Join(dir, "missing", "*.md")).
		Start(); err == nil {
		t.Error("Start succeeded without a required prompt")
	}

	// And a prompt over the cap is refused rather than truncated.
	writeDoc(t, filepath.Join(prompts, "huge.md"), strings.Repeat("x", 2048))
	if _, err := Setup("agent").
		File(config).
		RequiredDocuments("prompts", filepath.Join(prompts, "*.md")).
		DocumentLimits(1024, 4096, 10).
		Start(); err == nil {
		t.Error("Start accepted a required prompt over the size cap")
	}
}

func TestSettings_DirPatternsNarrowADirectory(t *testing.T) {
	dir := t.TempDir()
	mount := filepath.Join(dir, "conf.d")
	writeDoc(t, filepath.Join(mount, "app.json"), `{"port": 8080}`)
	writeDoc(t, filepath.Join(mount, "notes.yaml"), "port: 9999\n")

	settings, err := Setup("service").
		Dir(mount).
		DirPatterns("*.json").
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	if got := settings.GetInt("port"); got != 8080 {
		t.Errorf("port = %d, want only the JSON file to have been read", got)
	}
}

func TestSettings_DefaultAuditTrail(t *testing.T) {
	// The default trail is a per-user database; a test writes its own.
	t.Setenv("ARGUS_AUDIT_DIR", t.TempDir())

	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)

	settings, err := Setup("daemon").File(config).Audit().Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if settings.core.auditor == nil {
		t.Error("Audit() left the trail switched off")
	}
	if !settings.core.ownAuditor {
		t.Error("a trail Argus opened must be one Argus closes")
	}
	if err := settings.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestSettings_GetStringSlice(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"),
		`{"regions": ["eu", "us"], "tags": "a, b ,c"}`)
	t.Setenv("SL_HOSTS", "one.example, two.example")

	settings, err := Setup("service").File(config).Env("SL_").Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	if got := settings.GetStringSlice("regions"); len(got) != 2 || got[0] != "eu" {
		t.Errorf("regions = %v", got)
	}
	// A comma-separated string is a list too: it is all an environment
	// variable can carry.
	if got := settings.GetStringSlice("tags"); len(got) != 3 || got[1] != "b" {
		t.Errorf("tags = %v", got)
	}
	if got := settings.GetStringSlice("hosts"); len(got) != 2 || got[1] != "two.example" {
		t.Errorf("hosts = %v", got)
	}
	if got := settings.GetStringSlice("absent"); got != nil {
		t.Errorf("absent = %v, want nil", got)
	}
}

func TestSource_String(t *testing.T) {
	for source, want := range map[Source]string{
		SourceNone:     "none",
		SourceOverride: "override",
		SourceFlag:     "flag",
		SourceEnv:      "env",
		SourceFile:     "file",
		SourceRemote:   "remote",
		SourceDefault:  "default",
		Source(42):     "none",
	} {
		if got := source.String(); got != want {
			t.Errorf("Source(%d).String() = %q, want %q", source, got, want)
		}
	}
}

func TestDocument_ID(t *testing.T) {
	doc := newDocument("prompts", "system", "/tmp/system.md", "be careful", time.Unix(0, 0))

	if got := doc.ID(); got.Group != "prompts" || got.Name != "system" {
		t.Errorf("ID = %+v", got)
	}
	if got := doc.ID().String(); got != "prompts/system" {
		t.Errorf("ID.String() = %q", got)
	}
}

// TestSettings_FileIfPresentIsOptional: the "/etc/app.json if it is there"
// case. Absent is not an error; present and broken still is, because a file
// somebody wrote and got wrong is not the same as a file nobody wrote.
func TestSettings_FileIfPresentIsOptional(t *testing.T) {
	dir := t.TempDir()
	base := writeDoc(t, filepath.Join(dir, "base.json"), `{"port": 8080, "workers": 4}`)
	local := filepath.Join(dir, "local.json")

	settings, err := Setup("service").File(base).FileIfPresent(local).Start()
	if err != nil {
		t.Fatalf("Start without the optional file: %v", err)
	}
	if got := settings.GetInt("port"); got != 8080 {
		t.Errorf("port = %d", got)
	}
	// Explain says it is absent rather than pretending it was read.
	var seen bool
	for _, state := range settings.Explain().Sources {
		if state.Detail == local {
			seen = true
			if state.State != "absent" {
				t.Errorf("optional file state = %q, want absent", state.State)
			}
		}
	}
	if !seen {
		t.Error("Explain does not mention the optional file at all")
	}
	_ = settings.Close()

	// Present, it is read, and being declared later it wins.
	writeDoc(t, local, `{"workers": 16}`)
	settings, err = Setup("service").File(base).FileIfPresent(local).Start()
	if err != nil {
		t.Fatalf("Start with the optional file: %v", err)
	}
	defer func() { _ = settings.Close() }()

	if got := settings.GetInt("workers"); got != 16 {
		t.Errorf("workers = %d, want the optional file's 16", got)
	}
	if got := settings.GetInt("port"); got != 8080 {
		t.Errorf("port = %d, want the base file's 8080", got)
	}
}

func TestSettings_FileIfPresentStillHasToParse(t *testing.T) {
	dir := t.TempDir()
	base := writeDoc(t, filepath.Join(dir, "base.json"), `{"port": 8080}`)
	local := writeDoc(t, filepath.Join(dir, "local.json"), `{"workers":`)

	if _, err := Setup("service").File(base).FileIfPresent(local).Start(); err == nil {
		t.Error("an optional file that does not parse was ignored; a typo would be invisible")
	}
}

// TestSettings_FileIfPresentAppearsAndGoesAway: an operator drops an override
// onto a running box, then removes it. Both are reloads, and the second one
// gives the base file its value back.
func TestSettings_FileIfPresentAppearsAndGoesAway(t *testing.T) {
	dir := t.TempDir()
	base := writeDoc(t, filepath.Join(dir, "base.json"), `{"workers": 4}`)
	local := filepath.Join(dir, "local.json")

	reloads := make(chan Change, 4)
	settings, err := Setup("service").
		File(base).
		FileIfPresent(local).
		MaxStaleness(20 * time.Millisecond).
		OnReload(func(_ *Settings, c Change) { reloads <- c }).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	writeDoc(t, local, `{"workers": 16}`)
	waitForChange(t, reloads, "workers")
	if got := settings.GetInt("workers"); got != 16 {
		t.Errorf("workers = %d after the override appeared", got)
	}

	if err := os.Remove(local); err != nil {
		t.Fatalf("remove: %v", err)
	}
	waitForChange(t, reloads, "workers")
	if got := settings.GetInt("workers"); got != 4 {
		t.Errorf("workers = %d after the override was removed, want the base file's 4", got)
	}
}

// waitForChange waits for a reload that names the given key.
func waitForChange(t *testing.T, reloads <-chan Change, key string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case change := <-reloads:
			if change.HasKey(key) {
				return
			}
		case <-deadline:
			t.Fatalf("no reload naming %q", key)
		}
	}
}

// TestSettings_FileIfPresentUnreadableIsAnError: present but unreadable is not
// absent. A file whose permissions were tightened by mistake must not be
// skipped as if nobody had written it.
func TestSettings_FileIfPresentUnreadableIsAnError(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file modes do not stop this user")
	}
	dir := t.TempDir()
	base := writeDoc(t, filepath.Join(dir, "base.json"), `{"port": 8080}`)
	local := writeDoc(t, filepath.Join(dir, "local.json"), `{"workers": 16}`)
	if err := os.Chmod(local, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}

	if _, err := Setup("service").File(base).FileIfPresent(local).Start(); err == nil {
		t.Error("an unreadable optional file was treated as an absent one")
	}
}

// TestSource_JSON: Explain exists to be served on a debug endpoint, and
// "source": 4 tells an operator nothing. It round-trips, so a tool that reads
// the endpoint back gets the value and not a string it has to map itself.
func TestSource_JSON(t *testing.T) {
	for source, want := range map[Source]string{
		SourceNone:     `"none"`,
		SourceOverride: `"override"`,
		SourceFlag:     `"flag"`,
		SourceEnv:      `"env"`,
		SourceFile:     `"file"`,
		SourceRemote:   `"remote"`,
		SourceDefault:  `"default"`,
	} {
		encoded, err := json.Marshal(source)
		if err != nil {
			t.Fatalf("Marshal(%v): %v", source, err)
		}
		if string(encoded) != want {
			t.Errorf("Marshal(%v) = %s, want %s", source, encoded, want)
		}

		var decoded Source
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatalf("Unmarshal(%s): %v", encoded, err)
		}
		if decoded != source {
			t.Errorf("round trip of %v gave %v", source, decoded)
		}
	}

	// A name nobody knows is an error rather than a silent SourceNone, which
	// would read as "no source supplied this key".
	var decoded Source
	if err := json.Unmarshal([]byte(`"elsewhere"`), &decoded); err == nil {
		t.Error("an unknown source name decoded silently")
	}
}

// TestExplain_MarshalsReadably is the whole point of Explain being a struct.
func TestExplain_MarshalsReadably(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)

	settings, err := Setup("readable").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	encoded, err := json.Marshal(settings.Explain())
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"port":"file"`) {
		t.Errorf("Explain JSON does not name the source in words: %s", encoded)
	}
}

// TestSettings_TypedGettersThroughFlags: a flag holds its value in its own
// type, so the getters read it straight from the flag set rather than
// converting an interface. Every getter has that branch and none of the other
// tests takes it.
func TestSettings_TypedGettersThroughFlags(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"),
		`{"workers": 1, "timeout": "1s", "ratio": 0.1, "debug": false, "regions": ["eu"]}`)

	flags := flashflags.New("tool")
	flags.Int("workers", 2, "workers")
	flags.Duration("timeout", time.Second, "timeout")
	flags.Float64("ratio", 0.5, "ratio")
	flags.Bool("debug", false, "debug")
	flags.StringSlice("regions", []string{"eu"}, "regions")
	flags.Int("declared-only", 7, "a flag nobody set and no file supplies")
	if err := flags.Parse([]string{
		"--workers", "16",
		"--timeout", "45s",
		"--ratio", "0.75",
		"--debug",
		"--regions", "us,ap",
	}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	settings, err := Setup("tool").Flags(flags).File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	if got := settings.GetInt("workers"); got != 16 {
		t.Errorf("workers = %d, want 16 from the flag", got)
	}
	if got := settings.GetDuration("timeout"); got != 45*time.Second {
		t.Errorf("timeout = %v, want 45s from the flag", got)
	}
	if got := settings.GetFloat64("ratio"); got != 0.75 {
		t.Errorf("ratio = %v, want 0.75 from the flag", got)
	}
	if !settings.GetBool("debug") {
		t.Error("debug = false, want true from the flag")
	}
	if got := settings.GetStringSlice("regions"); len(got) != 2 || got[0] != "us" {
		t.Errorf("regions = %v, want the flag's list", got)
	}

	// A declared flag nobody set contributes its own default, below the file
	// but above nothing at all.
	if got := settings.GetInt("declared-only"); got != 7 {
		t.Errorf("declared-only = %d, want the flag's declared 7", got)
	}
}

// TestSettings_GettersWithoutAnySource: every getter has a path for "nobody
// supplied this and there are no flags either". It must be the zero value, not
// a panic on a nil flag set.
func TestSettings_GettersWithoutAnySource(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)

	settings, err := Setup("bare").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	if got := settings.GetString("absent"); got != "" {
		t.Errorf("GetString = %q", got)
	}
	if got := settings.GetInt("absent"); got != 0 {
		t.Errorf("GetInt = %d", got)
	}
	if got := settings.GetBool("absent"); got {
		t.Error("GetBool = true")
	}
	if got := settings.GetDuration("absent"); got != 0 {
		t.Errorf("GetDuration = %v", got)
	}
	if got := settings.GetFloat64("absent"); got != 0 {
		t.Errorf("GetFloat64 = %v", got)
	}
	if got := settings.GetStringSlice("absent"); got != nil {
		t.Errorf("GetStringSlice = %v", got)
	}
	// And a value that is there but of the wrong shape falls back the same way.
	if got := settings.GetDuration("port"); got != 0 {
		t.Errorf("GetDuration of a number = %v", got)
	}
}

// TestSettings_RemoteComingBackIsPublished: while the remote was down the
// snapshot held no remote layer at all. When it answers again, that layer has
// to be published even if the local file shadows every key it carries —
// otherwise the day the local file loses a key, the value behind it would be
// the one from the outage.
func TestSettings_RemoteComingBackIsPublished(t *testing.T) {
	provider := failingProvider("settingsremoterecovers", map[string]interface{}{"port": 9999})
	url := useRemoteProvider(t, provider)

	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"port": 8080}`)

	settings, err := Setup("service").
		File(config).
		Remote(url, &RemoteConfigOptions{RetryAttempts: 1, RetryDelay: time.Millisecond}).
		RemoteInterval(time.Millisecond).
		MaxStaleness(20 * time.Millisecond).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	if len(settings.Explain().Issues) == 0 {
		t.Fatal("the remote was supposed to be down")
	}
	before := settings.Revision()

	// The remote answers again, with a key the local file already shadows: no
	// value an application reads changes, but the state of the source does.
	// Nothing on disk is touched — the remote's own clock brings it back.
	provider.fail.Store(false)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(settings.Explain().Issues) == 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	explanation := settings.Explain()
	if len(explanation.Issues) != 0 {
		t.Fatalf("the recovered remote is still reported as an issue: %v", explanation.Issues)
	}
	if explanation.Revision <= before {
		t.Errorf("revision = %d, want a new one: the remote layer changed", explanation.Revision)
	}
	if got := settings.GetInt("port"); got != 8080 {
		t.Errorf("port = %d, want the local file's 8080 either way", got)
	}
	for _, state := range explanation.Sources {
		if state.Source == SourceRemote && state.State != "loaded" {
			t.Errorf("remote state = %q, want loaded", state.State)
		}
	}
}
