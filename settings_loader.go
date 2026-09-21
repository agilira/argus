// settings_loader.go: building one candidate out of every declared source
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// A cycle is two steps, and the cheap one runs first. fingerprint only stats
// the declared paths — the measured cost of that path is under a microsecond
// per file — and a build happens only when it moved. That is also where
// coalescing comes from: five files saved in one go are one fingerprint
// change, so they are one build and one swap, not five.

package argus

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/agilira/go-errors"
)

// defaultConfigPatterns are the file names a configuration directory is
// scanned for. They match the directory watcher's, so a ConfigMap mount reads
// the same either way.
var defaultConfigPatterns = []string{"*.yaml", "*.yml", "*.json", "*.toml", "*.ini"}

// fileSource is one declared configuration file.
//
// optional is the difference between "/etc/app.json if it is there" and a file
// the application cannot run without. An optional file that is absent is not a
// problem; an optional file that is present and does not parse still is, since
// somebody wrote it and got it wrong.
type fileSource struct {
	path     string
	optional bool
}

// sourceIssue records a source that could not be used while the candidate was
// still good enough to publish — a remote provider that is down while a local
// file resolves the same keys. Explain reports these; they are the difference
// between "this key is missing" and "this key is missing because the remote
// has been unreachable for an hour".
type sourceIssue struct {
	Source Source
	Detail string
}

// configSources is everything a candidate is built from, in precedence order
// within each kind: directories first, then explicit files, which are the more
// specific declaration and therefore win.
type configSources struct {
	dirs           []string
	dirPatterns    []string
	files          []fileSource
	remote         string
	remoteOpts     *RemoteConfigOptions
	remoteInterval time.Duration
	docs           *documentStore
	remoteIsSole   bool // no local source resolves the same keys
}

// build reads every source and returns a candidate snapshot.
//
// An error means the candidate must be refused: a configuration file that does
// not parse, or is not there, would otherwise publish an empty revision and
// take the application's settings away from it silently.
//
// previous is the revision in force, and fetchRemote says whether the remote
// provider is due. A rebuild caused by a local file carries the remote layer
// forward rather than calling the network again: the two are on different
// clocks on purpose.
func (c *configSources) build(ctx context.Context, previous *snapshot, fetchRemote bool) (*snapshot, []documentSkip, error) {
	merged := make(map[string]interface{})
	var issues []sourceIssue

	for _, dir := range c.dirs {
		files, skipped, err := c.configFilesIn(dir)
		if err != nil {
			return nil, nil, err
		}
		for _, skip := range skipped {
			issues = append(issues, sourceIssue{Source: SourceFile, Detail: skip})
		}
		for _, path := range files {
			if err := mergeConfigFile(merged, path); err != nil {
				return nil, nil, err
			}
		}
	}

	for _, file := range c.files {
		if err := mergeConfigFile(merged, file.path); err != nil {
			if file.optional && isMissingFile(err) {
				issues = append(issues, sourceIssue{Source: SourceFile, Detail: file.path + ": absent"})
				continue
			}
			return nil, nil, err
		}
	}

	candidate := &snapshot{file: merged}

	if c.remote != "" {
		switch {
		case !fetchRemote:
			// Not due: whatever the last fetch produced still stands.
			if previous != nil {
				candidate.remote = previous.remote
				candidate.remoteErr = previous.remoteErr
			}
		default:
			values, err := LoadRemoteConfigWithContext(ctx, c.remote, c.remoteOpts)
			switch {
			case err == nil:
				candidate.remote = values
			case c.remoteIsSole:
				// Nothing else supplies these keys: publishing without them
				// would be publishing an application with no configuration.
				return nil, nil, errors.Wrap(err, ErrCodeRemoteConfigError,
					"remote configuration is unreachable and nothing else supplies it")
			default:
				// The local file is the emergency override, and this is the
				// emergency. What the remote last said still stands — losing
				// a key over a network blip would be worse than serving it a
				// little stale — and the failure is reported.
				if previous != nil {
					candidate.remote = previous.remote
				}
				candidate.remoteErr = err.Error()
			}
		}
		if candidate.remoteErr != "" {
			issues = append(issues, sourceIssue{Source: SourceRemote, Detail: candidate.remoteErr})
		}
	}

	var skipped []documentSkip
	if c.docs != nil {
		docs, docSkips, err := c.docs.load()
		if err != nil {
			return nil, nil, err
		}
		candidate.docs = docs
		skipped = docSkips
		for _, skip := range docSkips {
			issues = append(issues, sourceIssue{
				Source: SourceFile,
				Detail: skip.ID.String() + ": " + skip.Reason,
			})
		}
	}
	candidate.issues = issues

	return candidate, skipped, nil
}

// fingerprint is the cheap half of a cycle: what every declared path looks
// like from the outside, without opening any of them.
//
// A path that is missing contributes its absence, so that deleting a
// configuration file is a change like any other — the cycle then rebuilds,
// refuses the candidate, and the application keeps the last good revision.
func (c *configSources) fingerprint() string {
	digest := sha256.New()
	// One buffer for the whole pass: a cycle runs at the poll interval, and a
	// fresh allocation per watched file is the kind of cost that only shows up
	// once somebody points a thousand documents at it.
	line := make([]byte, 0, 256)

	for _, dir := range c.dirs {
		files, _, err := c.configFilesIn(dir)
		if err != nil {
			line = appendUnreadable(line[:0], dir)
			_, _ = digest.Write(line)
			continue
		}
		for _, path := range files {
			line = appendStatPrint(line[:0], path)
			_, _ = digest.Write(line)
		}
	}

	for _, file := range c.files {
		line = appendStatPrint(line[:0], file.path)
		_, _ = digest.Write(line)
	}

	if c.docs != nil {
		for _, source := range c.docs.sources {
			matches, _, err := source.scan()
			if err != nil {
				line = appendUnreadable(line[:0], source.pattern)
				_, _ = digest.Write(line)
				continue
			}
			for _, path := range matches {
				line = appendStatPrint(line[:0], path)
				_, _ = digest.Write(line)
			}
		}
	}

	return hex.EncodeToString(digest.Sum(nil))
}

// appendStatPrint appends one path's observable state to buf.
//
// Lstat, not Stat: what is recorded is the entry in the directory, so that a
// symlink repointed at another file is a change even when both files have the
// same size and time. That is the Kubernetes ConfigMap swap.
func appendStatPrint(buf []byte, path string) []byte {
	info, err := os.Lstat(path)
	if err != nil {
		return appendUnreadable(buf, path)
	}

	buf = append(buf, path...)
	buf = append(buf, '|')
	buf = strconv.AppendInt(buf, info.Size(), 10)
	buf = append(buf, '|')
	buf = strconv.AppendInt(buf, info.ModTime().UnixNano(), 10)
	buf = append(buf, '|')
	buf = strconv.AppendUint(buf, uint64(info.Mode()), 10)

	return append(buf, 0)
}

// appendUnreadable records a path that could not be looked at, which is itself
// a state worth noticing a change in.
func appendUnreadable(buf []byte, path string) []byte {
	buf = append(buf, path...)
	buf = append(buf, "|unreadable"...)
	return append(buf, 0)
}

// configFilesIn lists the configuration files in a directory, sorted, so that
// a merge is the same on every machine and every boot, together with the
// entries it would not read and why.
func (c *configSources) configFilesIn(dir string) (files []string, skipped []string, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, errors.Wrap(err, ErrCodeFileNotFound,
			"cannot read the configuration directory: "+dir)
	}

	patterns := c.dirPatterns
	if len(patterns) == 0 {
		patterns = defaultConfigPatterns
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		for _, pattern := range patterns {
			if ok, _ := filepath.Match(pattern, entry.Name()); ok {
				path, reason := usableConfigFile(dir, entry.Name())
				if reason == "" {
					files = append(files, path)
				} else {
					skipped = append(skipped, filepath.Join(dir, entry.Name())+": "+reason)
				}
				break
			}
		}
	}
	sort.Strings(files)
	sort.Strings(skipped)

	return files, skipped, nil
}

// usableConfigFile decides whether a directory entry may be merged into the
// configuration, applying to this path the two guards the document store and
// the directory watcher already apply to theirs.
//
// A symlink is resolved and must stay inside the directory: a Kubernetes
// ConfigMap mount is a directory of symlinks into ..data, so they cannot be
// refused outright, but one pointing outside would pull a file the operator
// never put there into the application's configuration.
//
// What is left must be a regular file. os.ReadFile on a FIFO blocks until
// somebody writes to the other end, and a FIFO dropped into a watched
// directory would hang every reload from then on.
func usableConfigFile(dir, name string) (path, reason string) {
	path = filepath.Join(dir, name)

	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", "cannot be resolved: " + err.Error()
	}
	if !withinRoot(dir, real) {
		return "", "resolves outside the configuration directory"
	}

	info, err := os.Stat(real)
	if err != nil {
		return "", "cannot be read: " + err.Error()
	}
	if !info.Mode().IsRegular() {
		return "", "is not a regular file"
	}

	return path, ""
}

// isMissingFile reports whether an error is a file that is simply not there,
// as opposed to one that is there and unusable.
func isMissingFile(err error) bool {
	return stderrors.Is(err, fs.ErrNotExist)
}

// mergeConfigFile parses one file and merges it over what is already there.
//
// The merge is shallow, as the directory watcher's is: a later file replaces a
// whole key rather than merging into it, which is the only rule that stays
// predictable once three files and a nested map are involved.
func mergeConfigFile(into map[string]interface{}, path string) error {
	// Stat before open, as everywhere else that Argus reads a file it did not
	// create: os.ReadFile on a FIFO blocks until somebody writes to the other
	// end, and a configuration file that is a named pipe would hang Start with
	// no error and no log line.
	info, err := os.Stat(path)
	if err != nil {
		return errors.Wrap(err, ErrCodeFileNotFound,
			"cannot read the configuration file: "+path)
	}
	if !info.Mode().IsRegular() {
		return errors.New(ErrCodeInvalidConfig,
			"the configuration file is not a regular file: "+path)
	}

	data, err := os.ReadFile(path) // #nosec G304 -- a path the application declared
	if err != nil {
		return errors.Wrap(err, ErrCodeFileNotFound,
			"cannot read the configuration file: "+path)
	}

	values, err := ParseConfig(data, DetectFormat(path))
	if err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig,
			"cannot parse the configuration file: "+path)
	}

	for key, value := range values {
		into[key] = value
	}

	return nil
}
