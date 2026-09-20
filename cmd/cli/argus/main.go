// argus: the Argus configuration management command-line tool.
//
// WHY THIS LIVES HERE: cmd/cli is a library package (package cli) exposing a
// Manager, and a directory holds exactly one package, so the executable needs
// its own directory inside that module. It cannot go in the root module either
// — the CLI depends on Orpheus, and the library must not.
//
// Until now no package main existed anywhere in the repository. The release
// workflow built the root LIBRARY package into files named argus-linux-amd64
// and published them as "CLI Binary": they were ar archives, not executables,
// while the release notes told people to run ./argus config get.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"errors"
	"fmt"
	"os"

	cli "github.com/agilira/argus/cmd/cli"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stderr))
}

// run is separated from main so the exit path is testable.
func run(args []string, stderr *os.File) int {
	manager := cli.NewManager()

	if err := manager.Run(args); err != nil {
		// Orpheus reports a help request as an error; it is not a failure.
		if isHelp(err) {
			return 0
		}
		fmt.Fprintf(stderr, "argus: %v\n", err)
		return 1
	}

	return 0
}

// isHelp reports whether err only means the user asked for usage.
func isHelp(err error) bool {
	var helpErr interface{ IsHelp() bool }
	if errors.As(err, &helpErr) {
		return helpErr.IsHelp()
	}
	return false
}
