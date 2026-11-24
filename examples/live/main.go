// main.go: a running program that follows its configuration.
//
// It reads config.yaml, the AGENT_* environment variables and the prompts in
// prompts/*.md from the current directory, prints where each value came from,
// then prints every revision as it happens. Edit the files while it runs: a
// change becomes a new revision, a broken file is refused and the previous
// revision keeps serving. assets/demo.sh records exactly that.
//
// Copyright (c) 2026 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/agilira/argus"
)

var keys = []string{"model", "temperature", "workers"}

func main() {
	// running is read by OnError, which can fire before Start has returned.
	var running atomic.Pointer[argus.Settings]

	settings, err := argus.Setup("agent").
		File("config.yaml").
		Env("AGENT_").
		Documents("prompts", "prompts/*.md").
		MaxStaleness(200 * time.Millisecond).
		OnReload(func(s *argus.Settings, changed argus.Change) {
			for _, key := range changed.Keys {
				fmt.Printf("revision %d  %s = %s\n", changed.Revision, key, s.GetString(key))
			}
			for _, doc := range changed.Documents {
				fmt.Printf("revision %d  document %s/%s\n", changed.Revision, doc.Group, doc.Name)
			}
		}).
		OnError(func(err error) {
			fmt.Printf("refused   %v\n", err)
			if s := running.Load(); s != nil {
				fmt.Printf("          revision %d keeps serving\n", s.Revision())
			}
		}).
		Start()
	if err != nil {
		log.Fatalf("agent: %v", err)
	}
	defer func() { _ = settings.Close() }()
	running.Store(settings)

	explained := settings.Explain()
	fmt.Printf("revision %d\n", settings.Revision())
	for _, key := range keys {
		fmt.Printf("  %-12s %-16s %s\n", key, settings.GetString(key), explained.KeySource(key))
	}
	var names []string
	for _, doc := range settings.Docs("prompts") {
		names = append(names, doc.Name)
	}
	fmt.Printf("  %-12s %s\n", "prompts", strings.Join(names, ", "))

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
