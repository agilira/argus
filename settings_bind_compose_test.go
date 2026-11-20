// settings_bind_compose_test.go - binding maps, lists and pointers, strictly
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"errors"

	"path/filepath"
	"strings"
	"testing"
	"time"

	flashflags "github.com/agilira/flash-flags"
)

type mcpServer struct {
	Command string            `argus:"command,required"`
	Args    []string          `argus:"args"`
	Env     map[string]string `argus:"env"`
	Timeout time.Duration     `argus:"timeout"`
}

type role struct {
	Model string    `argus:"model"`
	Tools *[]string `argus:"tools"` // absent: every tool; empty: none
	Turns *int      `argus:"turns"`
}

type team struct {
	Name    string               `argus:"name,required"`
	Roles   map[string]role      `argus:"roles"`
	Servers map[string]mcpServer `argus:"servers"`
	Ports   []int                `argus:"ports"`
	Owners  []role               `argus:"owners"`
	Lead    *role                `argus:"lead"`
	Labels  map[string]string    `argus:"labels"`
	Server  serverConfig         `argus:"server"`
}

const teamYAML = `
name: acme
roles:
  writer:
    model: opus
    tools: [read, write]
  public:
    tools: []
  free: {}
servers:
  files:
    command: mcp-files
    args: ["--root", "/srv"]
    env: {TOKEN: "${FILES_TOKEN}"}
    timeout: 5s
ports: [80, 443]
owners:
  - model: sonnet
    turns: 3
lead:
  model: haiku
labels: {tier: gold}
server: {host: localhost, port: 8080}
`

func bindTeam(t *testing.T, contents string, opts ...BindOption) (*Bound[team], error) {
	t.Helper()
	config := writeDoc(t, filepath.Join(t.TempDir(), "config.yaml"), contents)
	settings, err := Setup("team").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = settings.Close() })
	return Bind[team](settings, opts...)
}

func TestBind_MapsListsAndPointers(t *testing.T) {
	bound, err := bindTeam(t, teamYAML, Strict())
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	got := bound.Value()

	writer := got.Roles["writer"]
	if writer.Model != "opus" || writer.Tools == nil || strings.Join(*writer.Tools, ",") != "read,write" {
		t.Errorf("writer = %+v", writer)
	}
	if public := got.Roles["public"]; public.Tools == nil || len(*public.Tools) != 0 {
		t.Errorf("an empty list must stay distinct from an absent one: %+v", public)
	}
	if free, ok := got.Roles["free"]; !ok || free.Tools != nil || free.Turns != nil {
		t.Errorf("an empty entry is present with every field zero: %+v (present %v)", free, ok)
	}
	files := got.Servers["files"]
	if files.Command != "mcp-files" || strings.Join(files.Args, " ") != "--root /srv" ||
		files.Env["TOKEN"] != "${FILES_TOKEN}" || files.Timeout != 5*time.Second {
		t.Errorf("files = %+v", files)
	}
	if len(got.Ports) != 2 || got.Ports[1] != 443 {
		t.Errorf("ports = %v", got.Ports)
	}
	if len(got.Owners) != 1 || got.Owners[0].Model != "sonnet" || got.Owners[0].Turns == nil || *got.Owners[0].Turns != 3 {
		t.Errorf("owners = %+v", got.Owners)
	}
	if got.Lead == nil || got.Lead.Model != "haiku" {
		t.Errorf("lead = %+v", got.Lead)
	}
	if got.Labels["tier"] != "gold" || got.Server.Port != 8080 {
		t.Errorf("labels = %v, server = %+v", got.Labels, got.Server)
	}
}

func TestBind_AbsentCompositesStayNil(t *testing.T) {
	bound, err := bindTeam(t, "name: acme\n", Strict())
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	got := bound.Value()
	if got.Roles != nil || got.Ports != nil || got.Lead != nil || got.Owners != nil {
		t.Errorf("absent keys bound to values: %+v", got)
	}
}

// The YAML parser turns a null into an empty map: a list or a value set to
// null is absent; a struct set to null is an empty one, as {} is.
func TestBind_NullIsAbsent(t *testing.T) {
	bound, err := bindTeam(t, "name: acme\nports: null\nlead: null\nroles:\n  writer:\n    tools: null\n    turns: null\n", Strict())
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	got := bound.Value()
	if got.Ports != nil || got.Roles["writer"].Tools != nil || got.Roles["writer"].Turns != nil {
		t.Errorf("null bound to a value: %+v", got)
	}
	if got.Lead == nil || got.Lead.Model != "" {
		t.Errorf("lead = %+v, want an empty role", got.Lead)
	}
}

func TestBind_ComposedValuesThatDoNotFit(t *testing.T) {
	cases := map[string]struct{ yaml, names string }{
		"a map that is a list":       {"name: a\nroles: [x]\n", "roles"},
		"a list that is a map":       {"name: a\nports: {a: 1}\n", "ports"},
		"an entry that is not a map": {"name: a\nroles:\n  writer: opus\n", "roles.writer"},
		"a bad element":              {"name: a\nports: [80, eighty]\n", "ports[1]"},
		"a bad value deep down":      {"name: a\nservers:\n  f:\n    command: x\n    timeout: soon\n", "servers.f.timeout"},
		"a required field missing":   {"name: a\nservers:\n  f:\n    args: [x]\n", "servers.f.command"},
		"a bad pointed value":        {"name: a\nowners:\n  - turns: many\n", "owners[0].turns"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := bindTeam(t, c.yaml)
			if err == nil {
				t.Fatal("bound")
			}
			if !strings.Contains(err.Error(), c.names) {
				t.Errorf("the error does not name %s: %v", c.names, err)
			}
		})
	}
}

func TestBind_StrictRefusesWhatTheTypeDoesNotName(t *testing.T) {
	cases := map[string]struct{ yaml, names string }{
		"at the top":         {"name: a\nnmae: b\n", "nmae"},
		"in a nested struct": {"name: a\nserver: {hots: x}\n", "server.hots"},
		"in a map entry":     {"name: a\nroles:\n  writer: {modle: x}\n", "roles.writer.modle"},
		"in a list element":  {"name: a\nowners:\n  - {tunrs: 1}\n", "owners[0].tunrs"},
		"in a pointed value": {"name: a\nlead: {x: 1}\n", "lead.x"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := bindTeam(t, c.yaml, Strict())
			if err == nil {
				t.Fatal("an unknown key was accepted")
			}
			if !strings.Contains(err.Error(), c.names) {
				t.Errorf("the error does not name %s: %v", c.names, err)
			}
		})
	}
}

func TestBind_LenientIgnoresWhatTheTypeDoesNotName(t *testing.T) {
	bound, err := bindTeam(t, "name: a\nnmae: b\nroles:\n  writer: {modle: x, model: y}\n")
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if bound.Value().Roles["writer"].Model != "y" {
		t.Errorf("roles = %+v", bound.Value().Roles)
	}
}

func TestBind_StrictOnASubtree(t *testing.T) {
	config := writeDoc(t, filepath.Join(t.TempDir(), "config.yaml"),
		"agents:\n  writer: {host: h, port: 1}\n  other: {anything: 1}\nelsewhere: 2\n")
	settings, err := Setup("agent").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	if _, err := Bind[serverConfig](settings.Sub("agents.writer"), Strict()); err != nil {
		t.Fatalf("keys outside the subtree were held against it: %v", err)
	}
	if _, err := Bind[serverConfig](settings.Sub("agents.other"), Strict()); err == nil ||
		!strings.Contains(err.Error(), "agents.other.anything") {
		t.Errorf("an unknown key in the subtree: %v", err)
	}
}

func TestBind_StrictWithDottedKeys(t *testing.T) {
	config := writeDoc(t, filepath.Join(t.TempDir(), "config.json"), `{"model": "m", "server.port": 1, "server.hots": "x"}`)
	settings, err := Setup("agent").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	if _, err := Bind[agentConfig](settings, Strict()); err == nil || !strings.Contains(err.Error(), "server.hots") {
		t.Errorf("a dotted unknown key: %v", err)
	}
}

type checkedTeam struct {
	Name  string          `argus:"name"`
	Roles map[string]role `argus:"roles"`
}

var errNoName = errors.New("a team needs a name")

func (c *checkedTeam) Validate() error {
	if c.Name == "" {
		return errNoName
	}
	return nil
}

func TestBind_ValidateRefuses(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.yaml"), "name: acme\n")
	failures := make(chan error, 4)
	settings, err := Setup("team").File(config).
		MaxStaleness(20 * time.Millisecond).
		OnError(func(err error) { failures <- err }).
		Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[checkedTeam](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	good := bound.Value()

	writeDoc(t, config, "name: \"\"\nroles: {x: {model: m}}\n")
	select {
	case err := <-failures:
		if !errors.Is(err, errNoName) {
			t.Errorf("the refusal does not carry Validate's error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a value Validate refused was published")
	}
	if bound.Value() != good {
		t.Error("the bound value moved to a refused revision")
	}
}

func TestBind_ValidateRefusesAtBind(t *testing.T) {
	config := writeDoc(t, filepath.Join(t.TempDir(), "config.yaml"), "roles: {}\n")
	settings, err := Setup("team").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	if _, err := Bind[checkedTeam](settings); !errors.Is(err, errNoName) {
		t.Errorf("Bind = %v, want Validate's error", err)
	}
}

func TestBind_ComposedTypesThatCannotBind(t *testing.T) {
	type intKeys struct {
		M map[int]string `argus:"m"`
	}
	type chans struct {
		C []chan int `argus:"c"`
	}
	type deep struct {
		M map[string]struct {
			F func() `argus:"f"`
		} `argus:"m"`
	}
	config := writeDoc(t, filepath.Join(t.TempDir(), "config.yaml"), "x: 1\n")
	settings, err := Setup("t").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	if _, err := Bind[intKeys](settings); err == nil {
		t.Error("a map with non-string keys was accepted")
	}
	if _, err := Bind[chans](settings); err == nil {
		t.Error("a list of channels was accepted")
	}
	if _, err := Bind[deep](settings); err == nil {
		t.Error("a struct with an unbindable field inside a map was accepted")
	}
}

type node struct {
	Name     string `argus:"name"`
	Children []node `argus:"children"`
}

type tree struct {
	Root node `argus:"root"`
}

func TestBind_RecursiveTypes(t *testing.T) {
	config := writeDoc(t, filepath.Join(t.TempDir(), "config.yaml"),
		"root:\n  name: a\n  children:\n    - name: b\n      children: [{name: c}]\n")
	settings, err := Setup("t").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[tree](settings, Strict())
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := bound.Value().Root; got.Children[0].Children[0].Name != "c" {
		t.Errorf("tree = %+v", got)
	}
}

func TestBind_ComposedFromTheEnvironmentIsRefused(t *testing.T) {
	t.Setenv("TEAM_ROLES", "writer")
	config := writeDoc(t, filepath.Join(t.TempDir(), "config.yaml"), "name: a\n")
	settings, err := Setup("team").File(config).Env("TEAM_").Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()
	if _, err := Bind[team](settings); err == nil || !strings.Contains(err.Error(), "roles") {
		t.Errorf("a map from a string: %v", err)
	}
}

func TestBind_ComposedValuesAreNotShared(t *testing.T) {
	dir := t.TempDir()
	config := writeDoc(t, filepath.Join(dir, "config.yaml"), teamYAML)
	settings, err := Setup("team").File(config).Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	a, err := Bind[team](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	b, err := Bind[team](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	a.Value().Servers["files"].Args[0] = "changed"
	a.Value().Labels["tier"] = "changed"
	if b.Value().Servers["files"].Args[0] != "--root" || b.Value().Labels["tier"] != "gold" {
		t.Error("two bound values share the configuration's own maps or lists")
	}
	if settings.Sub("labels").GetString("tier") != "gold" {
		t.Error("writing into a bound value wrote into the configuration")
	}
}

// Audit 20/11: what the first version of composed binding got wrong.

func startWith(t *testing.T, name, contents string, setup func(*SetupBuilder)) *Settings {
	t.Helper()
	config := writeDoc(t, filepath.Join(t.TempDir(), name), contents)
	b := Setup("t").File(config)
	if setup != nil {
		setup(b)
	}
	settings, err := b.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = settings.Close() })
	return settings
}

func TestBind_YAMLNullOfAMapIsAnEmptyMap(t *testing.T) {
	bound, err := bindTeam(t, "name: a\nlabels: null\n", Strict())
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := bound.Value().Labels; got == nil || len(got) != 0 {
		t.Errorf("labels = %#v, want the empty map YAML has always read null as", got)
	}
}

func TestBind_YAMLKeysThatAreNotStrings(t *testing.T) {
	bound, err := bindTeam(t, "name: a\nlabels: {200: ok, 404: missing}\n", Strict())
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := bound.Value().Labels; got["200"] != "ok" || got["404"] != "missing" {
		t.Errorf("labels = %v", got)
	}
}

func TestBind_DefaultsWrittenInGo(t *testing.T) {
	settings := startWith(t, "config.yaml", "name: a\n", func(b *SetupBuilder) {
		b.Defaults(map[string]interface{}{
			"ports":   []int{80},
			"labels":  map[string]string{"tier": "gold"},
			"servers": map[string]interface{}{"f": map[interface{}]interface{}{"command": "x", "args": []string{"y"}}},
		})
	})
	bound, err := Bind[team](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	got := bound.Value()
	if len(got.Ports) != 1 || got.Labels["tier"] != "gold" || got.Servers["f"].Args[0] != "y" {
		t.Errorf("team = %+v", got)
	}
}

func TestBind_AFlagSetsAPointer(t *testing.T) {
	flags := flashflags.New("t")
	flags.Int("turns", 0, "")
	if err := flags.Parse([]string{"--turns", "5"}); err != nil {
		t.Fatal(err)
	}
	settings := startWith(t, "config.yaml", "model: m\n", func(b *SetupBuilder) { b.Flags(flags) })
	bound, err := Bind[role](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := bound.Value().Turns; got == nil || *got != 5 {
		t.Errorf("turns = %v", got)
	}
}

func TestBind_AListFromTheEnvironment(t *testing.T) {
	t.Setenv("T_PORTS", "80, 443")
	settings := startWith(t, "config.yaml", "name: a\n", func(b *SetupBuilder) { b.Env("T_") })
	bound, err := Bind[team](settings)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := bound.Value().Ports; len(got) != 2 || got[1] != 443 {
		t.Errorf("ports = %v", got)
	}
}

func TestBind_CommaSeparatedInsideAComposite(t *testing.T) {
	bound, err := bindTeam(t, "name: a\nservers:\n  f: {command: x, args: \"a,b\"}\n", Strict())
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := bound.Value().Servers["f"].Args; len(got) != 2 || got[1] != "b" {
		t.Errorf("args = %v", got)
	}
}

func TestBind_CompositesFromFlatFiles(t *testing.T) {
	settings := startWith(t, "config.properties",
		"name=a\nservers.f.command=x\nservers.f.timeout=2s\nlabels.tier=gold\n", nil)
	bound, err := Bind[team](settings, Strict())
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	got := bound.Value()
	if got.Servers["f"].Command != "x" || got.Servers["f"].Timeout != 2*time.Second || got.Labels["tier"] != "gold" {
		t.Errorf("team = %+v", got)
	}

	typo := startWith(t, "config.properties", "name=a\nservers.f.command=x\nservers.f.comand=y\n", nil)
	if _, err := Bind[team](typo, Strict()); err == nil || !strings.Contains(err.Error(), "servers.f.comand") {
		t.Errorf("a flat unknown key inside a composite: %v", err)
	}
}

func TestBind_StrictUnderASubWrittenFlat(t *testing.T) {
	settings := startWith(t, "config.properties", "app.host=h\napp.typo=1\n", nil)
	if _, err := Bind[serverConfig](settings.Sub("app"), Strict()); err == nil || !strings.Contains(err.Error(), "app.typo") {
		t.Errorf("properties: %v", err)
	}
	mixed := startWith(t, "config.json", `{"app": {"host": "h"}, "app.typo": 1}`, nil)
	if _, err := Bind[serverConfig](mixed.Sub("app"), Strict()); err == nil || !strings.Contains(err.Error(), "app.typo") {
		t.Errorf("nested and flat: %v", err)
	}
}

type limited struct {
	CPU int `argus:"limits.cpu"`
}

type withLimits struct {
	Items map[string]limited `argus:"items"`
}

func TestBind_DottedTagsInsideAComposite(t *testing.T) {
	settings := startWith(t, "config.yaml", "items:\n  a:\n    limits: {cpu: 2}\n", nil)
	bound, err := Bind[withLimits](settings, Strict())
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if got := bound.Value().Items["a"].CPU; got != 2 {
		t.Errorf("cpu = %d", got)
	}
	typo := startWith(t, "config.yaml", "items:\n  a:\n    limits: {cpus: 2}\n", nil)
	if _, err := Bind[withLimits](typo, Strict()); err == nil || !strings.Contains(err.Error(), "items.a.limits.cpus") {
		t.Errorf("an unknown key under a dotted tag: %v", err)
	}
}

func TestBind_NullDoesNotSkipRequired(t *testing.T) {
	for name, body := range map[string]string{
		"a map value":     `{"name": "a", "servers": {"files": null}}`,
		"a list element":  `{"name": "a", "list": [null]}`,
		"a YAML-ish null": `{"name": "a", "servers": {"files": {}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			type withList struct {
				Name    string               `argus:"name"`
				Servers map[string]mcpServer `argus:"servers"`
				List    []mcpServer          `argus:"list"`
			}
			settings := startWith(t, "config.json", body, nil)
			if _, err := Bind[withList](settings); err == nil || !strings.Contains(err.Error(), "command") {
				t.Errorf("a null passed for a struct with a required field: %v", err)
			}
		})
	}
}
