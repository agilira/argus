// Argus CLI Example - Complete command-line interface
//
// This example demonstrates the Orpheus-powered CLI for Argus configuration
// management. The shipped binary is cmd/cli/argus; this example shows how to
// embed the same Manager in an application of your own.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"os"

	"github.com/agilira/argus/cmd/cli"
)

func main() {
	// Create the Orpheus-powered CLI manager
	// Manager automatically sets up all commands and subcommands
	manager := cli.NewManager()

	// Run the application
	if err := manager.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
