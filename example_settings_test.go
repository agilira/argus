// example_settings_test.go - the documented examples, compiled and run
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// These are the snippets the README and the API reference show. Written here
// they are compiled by the test suite and rendered by godoc, so a rename can
// no longer leave the documentation describing an API that is gone.

package argus_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/agilira/argus"
)

// scaffold writes a small application's configuration into a temporary
// directory and returns it.
func scaffold() (dir string, cleanup func()) {
	dir, err := os.MkdirTemp("", "argus-example-*")
	if err != nil {
		log.Fatal(err)
	}

	files := map[string]string{
		"config.json":       `{"model": "claude-opus-5", "port": 8080, "timeout": "30s"}`,
		"prompts/system.md": "You are a careful assistant.",
	}
	for name, contents := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			log.Fatal(err)
		}
	}

	return dir, func() { _ = os.RemoveAll(dir) }
}

// ExampleSetup declares where configuration comes from and reads it.
func ExampleSetup() {
	dir, cleanup := scaffold()
	defer cleanup()

	settings, err := argus.Setup("agent").
		File(filepath.Join(dir, "config.json")).
		Env("AGENT_").
		Documents("prompts", filepath.Join(dir, "prompts", "*.md")).
		Start()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = settings.Close() }()

	prompt, _ := settings.Doc("prompts", "system")

	fmt.Println(settings.GetString("model"))
	fmt.Println(settings.GetInt("port"))
	fmt.Println(prompt.String())

	// Output:
	// claude-opus-5
	// 8080
	// You are a careful assistant.
}

// ExampleBind maps a revision onto a struct type.
func ExampleBind() {
	dir, cleanup := scaffold()
	defer cleanup()

	type Config struct {
		Model   string        `argus:"model,required"`
		Timeout time.Duration `argus:"timeout"`
		Port    int           `argus:"port"`
	}

	settings, err := argus.Setup("agent").File(filepath.Join(dir, "config.json")).Start()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := argus.Bind[Config](settings)
	if err != nil {
		log.Fatal(err)
	}

	cfg := bound.Value()
	fmt.Println(cfg.Model, cfg.Port, cfg.Timeout)

	// Output: claude-opus-5 8080 30s
}

// ExampleSettings_Explain answers the question an operator has about a
// running instance: which revision, and where did each value come from?
func ExampleSettings_Explain() {
	dir, cleanup := scaffold()
	defer cleanup()

	if err := os.Setenv("EXPLAINED_PORT", "9090"); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("EXPLAINED_PORT") }()

	settings, err := argus.Setup("explained").
		File(filepath.Join(dir, "config.json")).
		Env("EXPLAINED_").
		Start()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = settings.Close() }()

	report := settings.Explain()

	fmt.Println(report.Revision)
	fmt.Println(report.KeySource("model"))
	fmt.Println(report.KeySource("port"))

	// Output:
	// 1
	// file
	// env
}

// ExampleSettings_Sub reads one subtree without repeating its prefix.
func ExampleSettings_Sub() {
	dir, err := os.MkdirTemp("", "argus-example-sub-*")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path,
		[]byte(`{"agents": {"writer": {"model": "opus", "temperature": 0.7}}}`), 0o600); err != nil {
		log.Fatal(err)
	}

	settings, err := argus.Setup("agents").File(path).Start()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = settings.Close() }()

	writer := settings.Sub("agents.writer")
	fmt.Println(writer.GetString("model"), writer.GetFloat64("temperature"))

	// Output: opus 0.7
}
