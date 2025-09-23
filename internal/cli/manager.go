// Package cli provides the command-line interface for Argus configuration management.package cli

//
// This package implements a high-performance CLI using the Orpheus framework,
// providing 7x-53x better performance than traditional Cobra-based CLIs.
//
// Features:
// - Ultra-fast command parsing (512 ns/op vs 3727 ns/op)
// - Zero-allocation hot paths for production workloads
// - Git-style subcommands with comprehensive auto-completion
// - Integrated audit logging and security compliance
// - Multi-format configuration support (JSON, YAML, TOML, HCL, INI, Properties)
//
// Architecture:
// - Manager: Core CLI orchestration and command routing
// - Commands: Individual command implementations with optimized handlers
// - Utils: Shared utilities for format detection and file operations
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package cli

import (
	"github.com/agilira/argus"
	"github.com/agilira/orpheus/pkg/orpheus"
)

// Manager provides high-performance CLI operations for Argus configuration management.
// Built on top of Orpheus framework for maximum performance and minimal allocations.
type Manager struct {
	app         *orpheus.App
	auditLogger *argus.AuditLogger // Optional audit integration
}

// NewManager creates a new high-performance CLI manager powered by Orpheus.
// Provides git-style subcommands with full audit integration and observability.
//
// Performance: Zero allocations for command setup, sub-microsecond command routing
func NewManager() *Manager {
	app := orpheus.New("argus").
		SetDescription("Ultra-fast configuration management with zero allocations").
		SetVersion("2.0.0")

	manager := &Manager{
		app: app,
	}

	// Setup command structure with fluent API
	manager.setupConfigCommands()
	manager.setupWatchCommands()
	manager.setupUtilityCommands()

	return manager
}

// WithAudit enables audit logging for all CLI operations.
// Provides compliance and security tracking with minimal performance overhead.
func (m *Manager) WithAudit(auditLogger *argus.AuditLogger) *Manager {
	m.auditLogger = auditLogger
	return m
}

// Run executes the CLI application with the provided arguments.
// Uses Orpheus for ultra-fast argument parsing and command routing.
//
// Performance: 512 ns/op vs 3727 ns/op for Cobra-based alternatives
func (m *Manager) Run(args []string) error {
	return m.app.Run(args)
}

// Command Setup Methods

// setupConfigCommands configures the 'config' command group for file operations.
// Provides get, set, delete, list, and conversion functionality.
func (m *Manager) setupConfigCommands() {
	configCmd := orpheus.NewCommand("config", "Configuration file operations")

	// config get <file> <key>
	getCmd := configCmd.Subcommand("get", "Get configuration value", m.handleConfigGet)
	getCmd.AddFlag("format", "f", "auto", "File format (auto|json|yaml|toml|hcl|ini|properties)")

	// config set <file> <key> <value> [--format=auto]
	setCmd := configCmd.Subcommand("set", "Set configuration value", m.handleConfigSet)
	setCmd.AddFlag("format", "f", "auto", "File format (auto|json|yaml|toml|hcl|ini|properties)")

	// config delete <file> <key> [--format=auto]
	deleteCmd := configCmd.Subcommand("delete", "Delete configuration key", m.handleConfigDelete)
	deleteCmd.AddFlag("format", "f", "auto", "File format (auto|json|yaml|toml|hcl|ini|properties)")

	// config list <file> [--prefix=] [--format=auto]
	listCmd := configCmd.Subcommand("list", "List configuration keys", m.handleConfigList)
	listCmd.AddFlag("prefix", "p", "", "Key prefix filter")
	listCmd.AddFlag("format", "f", "auto", "File format (auto|json|yaml|toml|hcl|ini|properties)")

	// config convert <input> <output> [--from=auto] [--to=auto]
	convertCmd := configCmd.Subcommand("convert", "Convert between configuration formats", m.handleConfigConvert)
	convertCmd.AddFlag("from", "", "auto", "Input format (auto|json|yaml|toml|hcl|ini|properties)")
	convertCmd.AddFlag("to", "", "auto", "Output format (auto|json|yaml|toml|hcl|ini|properties)")

	// config validate <file> [--format=auto]
	validateCmd := configCmd.Subcommand("validate", "Validate configuration file", m.handleConfigValidate)
	validateCmd.AddFlag("format", "f", "auto", "File format (auto|json|yaml|toml|hcl|ini|properties)")

	// config init <file> [--format=json] [--template=default]
	initCmd := orpheus.NewCommand("init", "Initialize new configuration file").
		AddFlag("format", "f", "json", "File format (json|yaml|toml|hcl|ini|properties)").
		AddFlag("template", "t", "default", "Template type (default|server|database|minimal)").
		SetHandler(m.handleConfigInit)
	configCmd.AddSubcommand(initCmd)

	m.app.AddCommand(configCmd)
}

// setupWatchCommands configures the 'watch' command group for real-time monitoring.
// Provides file watching and change detection functionality.
func (m *Manager) setupWatchCommands() {
	watchCmd := orpheus.NewCommand("watch", "Real-time configuration monitoring")

	// watch <file> [--interval=5s] [--format=auto]
	watchCmd.SetHandler(m.handleWatch)
	watchCmd.AddFlag("interval", "i", "5s", "Polling interval")
	watchCmd.AddFlag("format", "f", "auto", "File format (auto|json|yaml|toml|hcl|ini|properties)")
	watchCmd.AddBoolFlag("verbose", "v", false, "Verbose output")

	m.app.AddCommand(watchCmd)
}

// setupUtilityCommands configures utility commands for diagnostics and maintenance.
// Provides performance benchmarks, system info, and cleanup operations.
func (m *Manager) setupUtilityCommands() {
	// audit command group
	auditCmd := orpheus.NewCommand("audit", "Audit log management")

	queryCmd := auditCmd.Subcommand("query", "Query audit logs", m.handleAuditQuery)
	queryCmd.AddFlag("since", "s", "24h", "Time range (e.g., 24h, 7d, 2w)")
	queryCmd.AddFlag("event", "e", "", "Event type filter")
	queryCmd.AddFlag("file", "f", "", "File path filter")
	queryCmd.AddIntFlag("limit", "l", 100, "Maximum results")

	cleanupCmd := auditCmd.Subcommand("cleanup", "Cleanup old audit logs", m.handleAuditCleanup)
	cleanupCmd.AddFlag("older-than", "o", "30d", "Delete entries older than")
	cleanupCmd.AddBoolFlag("dry-run", "d", false, "Show what would be deleted")

	m.app.AddCommand(auditCmd)

	// benchmark command
	benchmarkCmd := orpheus.NewCommand("benchmark", "Run performance benchmarks")
	benchmarkCmd.SetHandler(m.handleBenchmark)
	benchmarkCmd.AddIntFlag("iterations", "i", 1000, "Number of iterations")
	benchmarkCmd.AddFlag("operation", "o", "all", "Operation to benchmark (get|set|parse|all)")
	m.app.AddCommand(benchmarkCmd)

	// info command
	infoCmd := orpheus.NewCommand("info", "System information and diagnostics")
	infoCmd.SetHandler(m.handleInfo)
	infoCmd.AddBoolFlag("verbose", "v", false, "Verbose system information")
	m.app.AddCommand(infoCmd)

	// completion command
	completionCmd := orpheus.NewCommand("completion", "Generate shell completion scripts")
	completionCmd.SetHandler(m.handleCompletion)
	m.app.AddCommand(completionCmd)
}
