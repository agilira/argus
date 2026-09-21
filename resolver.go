// resolver.go: the precedence engine shared by ConfigManager and Settings
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"encoding/json"
	"os"
	"strings"
	"sync"

	flashflags "github.com/agilira/flash-flags"
	"github.com/agilira/go-errors"
)

// Source names the layer a configuration value came from.
//
// It is what Explain reports for each key: an operator looking at a running
// service should never have to guess whether the value in effect came from the
// mounted file or from an environment variable set three deployments ago.
type Source int

const (
	// SourceNone means no layer supplied the key.
	SourceNone Source = iota

	// SourceOverride is an explicit value set by the application itself.
	SourceOverride

	// SourceFlag is a command-line flag: one the user actually set, or the
	// default declared when the flag was registered.
	SourceFlag

	// SourceEnv is an environment variable, found by mapping the key to a
	// name under the configured prefix.
	SourceEnv

	// SourceFile is a configuration file, or a directory of them.
	SourceFile

	// SourceRemote is a remote configuration provider.
	SourceRemote

	// SourceDefault is a default registered by the application.
	SourceDefault
)

// String makes a Source readable in logs and in Explain output.
func (s Source) String() string {
	switch s {
	case SourceOverride:
		return "override"
	case SourceFlag:
		return "flag"
	case SourceEnv:
		return "env"
	case SourceFile:
		return "file"
	case SourceRemote:
		return "remote"
	case SourceDefault:
		return "default"
	default:
		return "none"
	}
}

// MarshalJSON writes a Source as its name.
//
// Explain exists to be logged or served on a debug endpoint, and "source": 4
// tells the operator reading it nothing at all.
func (s Source) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// UnmarshalJSON reads a Source back from its name, so that a tool reading a
// debug endpoint gets the value rather than a string it has to map itself.
//
// An unknown name is an error, not SourceNone: silently decoding to "no source
// supplied this key" would turn a typo into a wrong answer.
func (s *Source) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}

	for candidate := SourceNone; candidate <= SourceDefault; candidate++ {
		if candidate.String() == name {
			*s = candidate
			return nil
		}
	}

	return errors.New(ErrCodeInvalidConfig, "unknown configuration source: "+name)
}

// resolution is what the resolver knows about one key.
//
// fromFlag is not the same as source == SourceFlag being enough to read the
// value from here: when it is set, value is empty on purpose and the caller
// must read the typed value straight from the FlagSet, which already holds it
// in the right type. That is how ConfigManager has always worked.
type resolution struct {
	value    interface{}
	source   Source
	fromFlag bool
	found    bool
}

// resolver holds every configuration layer and answers with the value from the
// highest one that has the key.
//
// Precedence, highest first:
//
//  1. overrides   — set explicitly by the application
//  2. flags       — a flag the command line actually set
//  3. env         — PREFIX_KEY in the environment
//  4. file        — parsed configuration file or directory
//  5. remote      — a remote provider
//  6. flags       — the default declared when the flag was registered
//  7. defaults    — registered by the application
//
// The order is fixed. A library whose precedence can be reordered is a library
// whose behaviour cannot be reasoned about from the outside.
//
// With only layers 1, 2, 4, 6 and 7 switched on this is exactly the order
// ConfigManager has always used; env and remote are the two it lacks.
type resolver struct {
	// swapState holds the file, remote and document layers as one immutable
	// snapshot, published atomically. Readers of those layers take no lock.
	swapState

	// mu guards the layers a reload never touches: an application sets them
	// while it is wiring itself up, not while it is serving.
	mu sync.RWMutex

	overrides map[string]interface{}
	defaults  map[string]interface{}

	flags *flashflags.FlagSet

	envPrefix  string
	envEnabled bool
}

// newResolver returns a resolver with every layer switched off, publishing an
// empty revision 0 so that a reader before the first load finds nothing rather
// than a nil snapshot.
func newResolver() *resolver {
	r := &resolver{
		overrides: make(map[string]interface{}),
		defaults:  make(map[string]interface{}),
	}
	r.current.Store(&snapshot{})
	return r
}

// useFlags installs an already-parsed flag set as the flag layer.
func (r *resolver) useFlags(flags *flashflags.FlagSet) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flags = flags
}

// useEnv switches the environment layer on with the given prefix.
//
// An empty prefix is legal and means the keys are read unprefixed; switching
// the layer on is the deliberate act, not the prefix being non-empty.
func (r *resolver) useEnv(prefix string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.envPrefix = prefix
	r.envEnabled = true
}

// setOverride sets an explicit application override.
func (r *resolver) setOverride(key string, value interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.overrides[key] = value
}

// setDefault registers a fallback for a key with no other source.
func (r *resolver) setDefault(key string, value interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaults[key] = value
}

// setFile replaces the file layer wholesale, as a reload does, leaving the
// other layers of the current snapshot in place.
func (r *resolver) setFile(values map[string]interface{}) error {
	candidate := r.view().derive()
	candidate.file = values

	_, err := r.apply(candidate, nil)

	return err
}

// setRemote replaces the remote layer wholesale.
func (r *resolver) setRemote(values map[string]interface{}) error {
	candidate := r.view().derive()
	candidate.remote = values

	_, err := r.apply(candidate, nil)

	return err
}

// document returns a document from the revision readable right now.
func (r *resolver) document(group, name string) (Document, bool) {
	return r.view().document(group, name)
}

// resolve returns the value for key from the highest-precedence layer that has
// it, and says which layer that was, reading the revision in force.
func (r *resolver) resolve(key string) resolution {
	// One load, before anything else: every layer this call reads from the
	// snapshot must come from the same revision.
	return r.resolveIn(r.view(), key)
}

// resolveIn resolves against one particular revision.
//
// A caller that reads several keys - binding a struct, explaining an instance -
// pins the revision once and reads every key from it, rather than letting a
// swap land between the third field and the fourth.
func (r *resolver) resolveIn(view *snapshot, key string) resolution {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if override, ok := r.overrides[key]; ok {
		return resolution{value: override, source: SourceOverride, found: true}
	}

	if r.flags != nil && r.flags.Changed(key) {
		return resolution{source: SourceFlag, fromFlag: true, found: true}
	}

	if r.envEnabled {
		if value, ok := os.LookupEnv(envKeyName(r.envPrefix, key)); ok {
			return resolution{value: value, source: SourceEnv, found: true}
		}
	}

	if view.file != nil {
		if value, ok := lookupConfigValue(view.file, key); ok {
			return resolution{value: value, source: SourceFile, found: true}
		}
	}

	if view.remote != nil {
		if value, ok := lookupConfigValue(view.remote, key); ok {
			return resolution{value: value, source: SourceRemote, found: true}
		}
	}

	if r.flags != nil && r.flags.Lookup(key) != nil {
		// A registered flag nobody set: its declared default is the value.
		return resolution{source: SourceFlag, fromFlag: true, found: true}
	}

	if fallback, ok := r.defaults[key]; ok {
		return resolution{value: fallback, source: SourceDefault, found: true}
	}

	return resolution{source: SourceNone}
}

// envKeyName maps a configuration key to the environment variable that can
// override it: the prefix, then the key upper-cased with its separators turned
// into underscores. "server.port" under "APP_" is APP_SERVER_PORT.
//
// A prefix without its trailing underscore is forgiven, because forgetting it
// is a typo that would otherwise silently read the wrong variable.
func envKeyName(prefix, key string) string {
	var b strings.Builder
	b.Grow(len(prefix) + len(key) + 1)

	if prefix != "" {
		b.WriteString(strings.ToUpper(prefix))
		if !strings.HasSuffix(prefix, "_") {
			b.WriteByte('_')
		}
	}
	b.WriteString(strings.ToUpper(envSeparators.Replace(key)))

	return b.String()
}

// envSeparators turns the characters a configuration key uses to separate path
// elements into the one an environment variable name can hold.
var envSeparators = strings.NewReplacer(".", "_", "-", "_", " ", "_")
