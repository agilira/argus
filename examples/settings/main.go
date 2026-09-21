// main.go: the advertised front door, end to end.
//
// One call says where configuration comes from — a file, the environment, a
// directory of prompts — and returns a handle that stays current while the
// program runs. Edit config.json or prompts/system.md while this is running
// and watch the revision move.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/agilira/argus"
)

// Config is what this example binds the configuration onto. Only tagged
// fields are bound, and a nested struct reads under its own tag.
type Config struct {
	Model       string  `argus:"model,required"`
	Temperature float64 `argus:"temperature"`
}

func main() {
	dir, err := scaffold()
	if err != nil {
		log.Fatalf("cannot lay out the example: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// The one call. Everything below is the application, not Argus.
	settings, err := argus.Setup("example-agent").
		File(filepath.Join(dir, "config.json")).
		FileIfPresent(filepath.Join(dir, "local.json")).
		Env("EXAMPLE_").
		Documents("prompts", filepath.Join(dir, "prompts", "*.md")).
		MaxStaleness(200 * time.Millisecond).
		OnReload(func(s *argus.Settings, changed argus.Change) {
			fmt.Printf("\nrevision %d\n", changed.Revision)
			for _, key := range changed.Keys {
				fmt.Printf("  key      %s = %v\n", key, s.GetString(key))
			}
			for _, doc := range changed.Documents {
				fmt.Printf("  document %s\n", doc)
			}
		}).
		OnError(func(err error) {
			// The previous revision is still in force; this says why the new
			// one was refused.
			fmt.Printf("\nrefused: %v\n", err)
		}).
		Start()
	if err != nil {
		log.Fatalf("argus: %v", err) // a missing file is loud, not an empty config
	}
	defer func() { _ = settings.Close() }()

	// A struct kept in step with the configuration. Every revision produces a
	// new value; the one already in hand never changes underneath its reader.
	bound, err := argus.Bind[Config](settings)
	if err != nil {
		log.Fatalf("argus: %v", err)
	}
	cfg := bound.Value()

	fmt.Printf("model       %s\n", cfg.Model)
	fmt.Printf("temperature %v\n", cfg.Temperature)

	if prompt, ok := settings.Doc("prompts", "system"); ok {
		fmt.Printf("prompt      %s (%d bytes, revision %d)\n",
			prompt.Name, prompt.Size, prompt.Revision)
	}

	// Explain is a struct, so it can be served on a debug endpoint: which
	// revision is this instance on, and where did each value come from?
	report, _ := json.MarshalIndent(settings.Explain(), "", "  ")
	fmt.Printf("\n%s\n", report)

	fmt.Printf("\nEditing %s\n", dir)
	fmt.Println("Try it: change a value, save a broken file, add a prompt.")
	fmt.Println("Ctrl-C to stop.")

	demonstrate(dir)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	fmt.Printf("\nstopped on revision %d, model %q\n",
		settings.Revision(), bound.Value().Model)
}

// scaffold writes the little application this example configures.
func scaffold() (string, error) {
	dir, err := os.MkdirTemp("", "argus-settings-example-*")
	if err != nil {
		return "", err
	}

	files := map[string]string{
		"config.json":       `{"model": "claude-opus-5", "temperature": 0.2}`,
		"prompts/system.md": "You are a careful assistant. Cite your sources.",
	}
	for name, contents := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			return "", err
		}
	}

	return dir, nil
}

// demonstrate makes the three interesting things happen by itself, so that
// running the example shows them without anybody editing a file.
func demonstrate(dir string) {
	go func() {
		time.Sleep(500 * time.Millisecond)

		// 1. Two files saved together are one revision, not two.
		_ = os.WriteFile(filepath.Join(dir, "config.json"),
			[]byte(`{"model": "claude-opus-5", "temperature": 0.9}`), 0o600)
		_ = os.WriteFile(filepath.Join(dir, "prompts", "tone.md"),
			[]byte("Be brief."), 0o600)

		// 2. A broken file is refused, and the last good revision stays.
		time.Sleep(time.Second)
		_ = os.WriteFile(filepath.Join(dir, "config.json"),
			[]byte(`{"model": "claude-opus`), 0o600)

		// 3. Fixing it recovers by itself.
		time.Sleep(time.Second)
		_ = os.WriteFile(filepath.Join(dir, "config.json"),
			[]byte(`{"model": "claude-opus-5", "temperature": 0.4}`), 0o600)
	}()
}
