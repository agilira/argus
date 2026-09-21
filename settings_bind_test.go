// settings_bind_test.go - tests for binding a revision onto a struct
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type serverConfig struct {
	Host string `argus:"host"`
	Port int    `argus:"port"`
}

type agentConfig struct {
	Model       string        `argus:"model,required"`
	Temperature float64       `argus:"temperature"`
	MaxTokens   int64         `argus:"max_tokens"`
	Streaming   bool          `argus:"streaming"`
	Timeout     time.Duration `argus:"timeout"`
	Regions     []string      `argus:"regions"`
	Server      serverConfig  `argus:"server"`

	Untagged string // left alone: only tagged fields are bound
	secret   string // unexported, untouchable
}

const agentJSON = `{
	"model": "claude-opus-5",
	"temperature": 0.2,
	"max_tokens": 4096,
	"streaming": true,
	"timeout": "30s",
	"regions": ["eu", "us"],
	"server": {"host": "localhost", "port": 8080},
	"untagged": "ignored"
}`

func TestBind_EveryKindOfField(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), agentJSON)

	settings, err := Setup("agent").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[agentConfig](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	cfg := bound.Value()
	if cfg.Model != "claude-opus-5" {
		t.Errorf("Model = %q", cfg.Model)
	}
	if cfg.Temperature != 0.2 {
		t.Errorf("Temperature = %v", cfg.Temperature)
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d", cfg.MaxTokens)
	}
	if !cfg.Streaming {
		t.Error("Streaming = false")
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
	if len(cfg.Regions) != 2 || cfg.Regions[0] != "eu" {
		t.Errorf("Regions = %v", cfg.Regions)
	}
	if cfg.Server.Host != "localhost" || cfg.Server.Port != 8080 {
		t.Errorf("Server = %+v: a nested struct is addressed under its own tag", cfg.Server)
	}
	if cfg.Untagged != "" {
		t.Errorf("Untagged = %q, want an untagged field left alone", cfg.Untagged)
	}
	if cfg.secret != "" {
		t.Errorf("secret = %q", cfg.secret)
	}
	if bound.Revision() != settings.Revision() {
		t.Errorf("bound revision %d, settings on %d", bound.Revision(), settings.Revision())
	}
}

// TestBind_FollowsPrecedence: binding reads through the same layers as the
// getters, so an environment variable outranks the file here too.
func TestBind_FollowsPrecedence(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), agentJSON)
	t.Setenv("BIND_TEMPERATURE", "0.9")
	t.Setenv("BIND_SERVER_PORT", "9090")

	settings, err := Setup("agent").File(config).Env("BIND_").Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[agentConfig](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	cfg := bound.Value()
	if cfg.Temperature != 0.9 {
		t.Errorf("Temperature = %v, want the environment's 0.9", cfg.Temperature)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, want the environment's 9090", cfg.Server.Port)
	}
}

// TestBind_RequiredFieldMissing: a field marked required is the application
// saying it cannot run without it, and Bind says so by name.
func TestBind_RequiredFieldMissing(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"temperature": 0.2}`)

	settings, err := Setup("agent").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[agentConfig](settings)
	if err == nil {
		t.Fatalf("Bind succeeded without the required model: %+v", bound.Value())
	}
	if !strings.Contains(err.Error(), "model") || !strings.Contains(err.Error(), "Model") {
		t.Errorf("error does not name the field and its key: %v", err)
	}
}

func TestBind_UnsupportedFieldType(t *testing.T) {
	type unsupported struct {
		Extras map[string]string `argus:"extras"`
	}

	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"extras": {}}`)

	settings, err := Setup("agent").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	if _, err := Bind[unsupported](settings); err == nil {
		t.Error("a field type with no binding was accepted")
	}
}

func TestBind_NotAStruct(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "opus"}`)

	settings, err := Setup("agent").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	if _, err := Bind[string](settings); err == nil {
		t.Error("binding onto a non-struct was accepted")
	}
}

// TestBind_ReloadDeliversANewValue is the rule the whole design turns on: a
// reload hands over a new value. It never writes into the one the application
// is already reading, which would be a data race inside user code.
func TestBind_ReloadDeliversANewValue(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), agentJSON)

	reloaded := make(chan uint64, 4)
	settings, err := Setup("agent").
		File(config).
		MaxStaleness(20 * time.Millisecond).
		OnReload(func(_ *Settings, c Change) { reloaded <- c.Revision }).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[agentConfig](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	before := bound.Value()

	writeDoc(t, config, `{"model": "claude-opus-5", "temperature": 0.9, "server": {"host": "localhost", "port": 8080}}`)
	select {
	case <-reloaded:
	case <-time.After(5 * time.Second):
		t.Fatal("no reload")
	}

	after := bound.Value()
	if after == before {
		t.Fatal("Value returned the same pointer across a revision")
	}
	if after.Temperature != 0.9 {
		t.Errorf("new value Temperature = %v", after.Temperature)
	}
	if before.Temperature != 0.2 {
		t.Errorf("the value held before the reload changed underneath: %v", before.Temperature)
	}
	if bound.Revision() != settings.Revision() {
		t.Errorf("bound revision %d, settings on %d", bound.Revision(), settings.Revision())
	}
}

// TestBind_RevisionThatCannotBindIsRefused: from the moment an application
// binds a type, a revision that does not satisfy it is not a revision. The
// last good value keeps serving and the error handler is told.
func TestBind_RevisionThatCannotBindIsRefused(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), agentJSON)

	failures := make(chan error, 4)
	settings, err := Setup("agent").
		File(config).
		MaxStaleness(20 * time.Millisecond).
		OnError(func(err error) { failures <- err }).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[agentConfig](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	good := bound.Value()
	revision := settings.Revision()

	// The required key goes away.
	writeDoc(t, config, `{"temperature": 0.5}`)

	select {
	case err := <-failures:
		if !strings.Contains(err.Error(), "model") {
			t.Errorf("refusal does not name the missing key: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a revision that cannot bind was accepted in silence")
	}

	if settings.Revision() != revision {
		t.Errorf("revision = %d, want the unchanged %d", settings.Revision(), revision)
	}
	if bound.Value() != good {
		t.Error("the bound value moved to a revision that was refused")
	}
	if got := settings.GetFloat64("temperature"); got != 0.2 {
		t.Errorf("temperature = %v, want the last good revision's 0.2", got)
	}
}

// TestBind_OnASubtree: one agent, or one tenant, bound to its own struct.
func TestBind_OnASubtree(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"),
		`{"agents": {"writer": {"host": "localhost", "port": 7070}}}`)

	settings, err := Setup("agent").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[serverConfig](settings.Sub("agents.writer"))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := bound.Value(); got.Host != "localhost" || got.Port != 7070 {
		t.Errorf("bound subtree = %+v", got)
	}
}

// TestBind_ConcurrentReadsDuringReloads: Value is read on the hot path while
// revisions are published underneath it. Run with -race.
func TestBind_ConcurrentReadsDuringReloads(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), agentJSON)

	settings, err := Setup("agent").File(config).MaxStaleness(time.Millisecond).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[agentConfig](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if cfg := bound.Value(); cfg.Model != "claude-opus-5" {
						t.Errorf("Model = %q mid-reload", cfg.Model)
						return
					}
				}
			}
		}()
	}

	for i := 0; i < 20; i++ {
		writeDoc(t, config, `{"model": "claude-opus-5", "temperature": 0.`+string(rune('0'+i%10))+`}`)
		time.Sleep(5 * time.Millisecond)
	}
	close(stop)
	wg.Wait()
}

// TestBind_WhileReloadsAreRunning: an application may bind long after Start,
// with the configuration already moving. Every bound value must belong to a
// revision that existed, and no bind may land a revision behind. Run with
// -race.
func TestBind_WhileReloadsAreRunning(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), agentJSON)

	settings, err := Setup("agent").File(config).MaxStaleness(time.Millisecond).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	stop := make(chan struct{})
	var writer sync.WaitGroup
	writer.Add(1)
	go func() {
		defer writer.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				writeDoc(t, config, `{"model": "claude-opus-5", "temperature": 0.`+
					string(rune('0'+i%10))+`, "server": {"host": "localhost", "port": 8080}}`)
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()

	var binders sync.WaitGroup
	for i := 0; i < 16; i++ {
		binders.Add(1)
		go func() {
			defer binders.Done()
			bound, err := Bind[agentConfig](settings)
			if err != nil {
				t.Errorf("Bind: %v", err)
				return
			}
			if cfg := bound.Value(); cfg.Model != "claude-opus-5" {
				t.Errorf("Model = %q", cfg.Model)
			}
			if bound.Revision() == 0 {
				t.Error("bound to revision 0")
			}
		}()
	}
	binders.Wait()
	close(stop)
	writer.Wait()
}

// TestBind_ValueThatDoesNotConvert: a key whose value cannot become the
// field's type is a mistake somebody made in a file, and binding it to the
// zero value would hide it. The getters are lenient by contract; binding is
// not.
func TestBind_ValueThatDoesNotConvert(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"),
		`{"model": "claude-opus-5", "max_tokens": "quite a lot"}`)

	settings, err := Setup("agent").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	_, err = Bind[agentConfig](settings)
	if err == nil {
		t.Fatal("a value that cannot become an int64 was bound to zero")
	}
	for _, want := range []string{"MaxTokens", "max_tokens"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q: %v", want, err)
		}
	}

	// The lenient contract of the getters is unchanged.
	if got := settings.GetInt("max_tokens"); got != 0 {
		t.Errorf("GetInt = %d, want the documented zero value", got)
	}
}

// TestBind_BadValueAtReloadIsRefused: the same mistake, made while the
// application is running, keeps the last good value serving.
func TestBind_BadValueAtReloadIsRefused(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), agentJSON)

	failures := make(chan error, 4)
	settings, err := Setup("agent").
		File(config).
		MaxStaleness(20 * time.Millisecond).
		OnError(func(err error) { failures <- err }).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[agentConfig](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	good := bound.Value()
	revision := settings.Revision()

	writeDoc(t, config, `{"model": "claude-opus-5", "temperature": "warm"}`)

	select {
	case err := <-failures:
		if !strings.Contains(err.Error(), "temperature") {
			t.Errorf("refusal does not name the key: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a value that cannot convert was accepted in silence")
	}

	if settings.Revision() != revision {
		t.Errorf("revision moved to %d", settings.Revision())
	}
	if bound.Value() != good {
		t.Error("the bound value moved to a refused revision")
	}
}

// TestBind_StringSliceFromACommaSeparatedString: an environment variable can
// only carry one string, and a list written that way is not a mistake.
func TestBind_StringSliceFromACommaSeparatedString(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.json"), `{"model": "opus"}`)
	t.Setenv("SLICE_REGIONS", "eu, us ,ap")

	settings, err := Setup("agent").File(config).Env("SLICE_").Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[agentConfig](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := bound.Value().Regions; len(got) != 3 || got[2] != "ap" {
		t.Errorf("Regions = %v", got)
	}
}
