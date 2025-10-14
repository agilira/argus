// Integration tests for the Argus CLI example
//
// These tests verify that the CLI manager initializes correctly and that
// all core commands are properly registered and accessible.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agilira/argus/cmd/cli"
)

// TestCLIManagerInitialization verifies that the CLI manager initializes correctly
func TestCLIManagerInitialization(t *testing.T) {
	manager := cli.NewManager()
	if manager == nil {
		t.Fatal("Expected manager to be initialized, got nil")
	}
}

// TestCLIHelp verifies that the help command works
func TestCLIHelp(t *testing.T) {
	manager := cli.NewManager()

	// Test --help flag
	err := manager.Run([]string{"--help"})
	if err != nil {
		t.Errorf("Expected no error from --help, got: %v", err)
	}
}

// TestCLIVersion verifies that the version command works
func TestCLIVersion(t *testing.T) {
	manager := cli.NewManager()

	// Test --version flag
	err := manager.Run([]string{"--version"})
	if err != nil {
		t.Errorf("Expected no error from --version, got: %v", err)
	}
}

// TestConfigCommandsExist verifies that config subcommands are registered
func TestConfigCommandsExist(t *testing.T) {
	manager := cli.NewManager()

	// Test that config command help works
	err := manager.Run([]string{"config", "--help"})
	if err != nil {
		t.Errorf("Expected no error from 'config --help', got: %v", err)
	}
}

// TestConfigGetCommand verifies the config get command with a test file
func TestConfigGetCommand(t *testing.T) {
	// Create a temporary test config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test-config.yaml")

	// Write a simple YAML config
	content := []byte(`
server:
  host: localhost
  port: 8080
database:
  name: testdb
`)
	if err := os.WriteFile(configFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	manager := cli.NewManager()

	// Test config get
	err := manager.Run([]string{"config", "get", configFile, "server.port"})
	if err != nil {
		t.Errorf("Expected no error from 'config get', got: %v", err)
	}
}

// TestConfigSetCommand verifies the config set command
func TestConfigSetCommand(t *testing.T) {
	// Create a temporary test config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test-config.yaml")

	// Write a simple YAML config
	content := []byte(`
server:
  host: localhost
  port: 8080
`)
	if err := os.WriteFile(configFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	manager := cli.NewManager()

	// Test config set
	err := manager.Run([]string{"config", "set", configFile, "server.port", "9000"})
	if err != nil {
		t.Errorf("Expected no error from 'config set', got: %v", err)
	}

	// Verify the value was set by reading it back
	err = manager.Run([]string{"config", "get", configFile, "server.port"})
	if err != nil {
		t.Errorf("Expected no error from 'config get' after set, got: %v", err)
	}
}

// TestConfigValidateCommand verifies the config validate command
func TestConfigValidateCommand(t *testing.T) {
	// Create a temporary test config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "test-config.yaml")

	// Write a valid YAML config
	content := []byte(`
server:
  host: localhost
  port: 8080
  timeout: 30s
`)
	if err := os.WriteFile(configFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	manager := cli.NewManager()

	// Test config validate
	err := manager.Run([]string{"config", "validate", configFile})
	if err != nil {
		t.Errorf("Expected no error from 'config validate', got: %v", err)
	}
}

// TestConfigConvertCommand verifies the config convert command
func TestConfigConvertCommand(t *testing.T) {
	// Create a temporary test config file
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "test-config.yaml")
	jsonFile := filepath.Join(tmpDir, "test-config.json")

	// Write a YAML config
	content := []byte(`
server:
  host: localhost
  port: 8080
`)
	if err := os.WriteFile(yamlFile, content, 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	manager := cli.NewManager()

	// Test config convert
	err := manager.Run([]string{"config", "convert", yamlFile, jsonFile})
	if err != nil {
		t.Errorf("Expected no error from 'config convert', got: %v", err)
	}

	// Verify the JSON file was created
	if _, err := os.Stat(jsonFile); os.IsNotExist(err) {
		t.Error("Expected JSON file to be created after conversion")
	}
}

// TestWatchCommand verifies that the watch command exists and accepts arguments
func TestWatchCommand(t *testing.T) {
	manager := cli.NewManager()

	// Test that watch command help works
	err := manager.Run([]string{"watch", "--help"})
	if err != nil {
		t.Errorf("Expected no error from 'watch --help', got: %v", err)
	}
}

// TestAuditCommand verifies that audit commands are registered
func TestAuditCommand(t *testing.T) {
	manager := cli.NewManager()

	// Test that audit command help works
	err := manager.Run([]string{"audit", "--help"})
	if err != nil {
		t.Errorf("Expected no error from 'audit --help', got: %v", err)
	}
}

// TestBenchmarkCommand verifies that the benchmark command exists
func TestBenchmarkCommand(t *testing.T) {
	manager := cli.NewManager()

	// Test that benchmark command help works
	err := manager.Run([]string{"benchmark", "--help"})
	if err != nil {
		t.Errorf("Expected no error from 'benchmark --help', got: %v", err)
	}
}

// TestInfoCommand verifies that the info command exists
func TestInfoCommand(t *testing.T) {
	manager := cli.NewManager()

	// Test that info command help works
	err := manager.Run([]string{"info", "--help"})
	if err != nil {
		t.Errorf("Expected no error from 'info --help', got: %v", err)
	}
}

// TestCompletionCommand verifies that completion command exists
func TestCompletionCommand(t *testing.T) {
	manager := cli.NewManager()

	// Test that completion command help works
	err := manager.Run([]string{"completion", "--help"})
	if err != nil {
		t.Errorf("Expected no error from 'completion --help', got: %v", err)
	}
}
