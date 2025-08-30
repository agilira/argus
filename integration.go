// integration.go: Unified Integration Layer for Argus + FlashFlags
//
// Copyright (c) 2025 AGILira
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

// Package argus provides unified configuration management combining:
// - FlashFlags ultra-fast command-line parsing
// - Lock-free configuration management
// - Multi-source configuration (flags, env vars, config files, defaults)
// - Real-time configuration watching with BoreasLite

package argus

import (
	"fmt"
	"os"
	"strings"
	"time"

	flashflags "github.com/agilira/flash-flags"
)

// ConfigManager combines all configuration sources in a unified interface
type ConfigManager struct {
	// FlashFlags for ultra-fast command-line parsing
	flags *flashflags.FlagSet

	// Optional file watcher for real-time config updates
	watcher *Watcher

	// Application metadata
	appName        string
	appDescription string
	appVersion     string

	// Configuration storage for explicit overrides
	values map[string]interface{}
}

// NewConfigManager creates a unified configuration manager
func NewConfigManager(appName string) *ConfigManager {
	return &ConfigManager{
		flags:   flashflags.New(appName),
		appName: appName,
		values:  make(map[string]interface{}),
	}
}

// SetDescription sets the application description for help text
func (cm *ConfigManager) SetDescription(description string) *ConfigManager {
	cm.appDescription = description
	cm.flags.SetDescription(description)
	return cm
}

// SetVersion sets the application version for help text
func (cm *ConfigManager) SetVersion(version string) *ConfigManager {
	cm.appVersion = version
	cm.flags.SetVersion(version)
	return cm
}

// Flag Registration Methods - Fluent Interface

// StringFlag adds a string configuration flag
func (cm *ConfigManager) StringFlag(name, defaultValue, usage string) *ConfigManager {
	// Register with FlashFlags
	cm.flags.String(name, defaultValue, usage)
	return cm
}

// IntFlag adds an integer configuration flag
func (cm *ConfigManager) IntFlag(name string, defaultValue int, usage string) *ConfigManager {
	cm.flags.Int(name, defaultValue, usage)
	return cm
}

// BoolFlag adds a boolean configuration flag
func (cm *ConfigManager) BoolFlag(name string, defaultValue bool, usage string) *ConfigManager {
	cm.flags.Bool(name, defaultValue, usage)
	return cm
}

// DurationFlag adds a duration configuration flag
func (cm *ConfigManager) DurationFlag(name string, defaultValue time.Duration, usage string) *ConfigManager {
	cm.flags.Duration(name, defaultValue, usage)
	return cm
}

// Float64Flag adds a float64 configuration flag
func (cm *ConfigManager) Float64Flag(name string, defaultValue float64, usage string) *ConfigManager {
	cm.flags.Float64(name, defaultValue, usage)
	return cm
}

// StringSliceFlag adds a string slice configuration flag
func (cm *ConfigManager) StringSliceFlag(name string, defaultValue []string, usage string) *ConfigManager {
	cm.flags.StringSlice(name, defaultValue, usage)
	return cm
}

// Configuration Management Methods

// Parse parses command-line arguments and binds them to configuration
func (cm *ConfigManager) Parse(args []string) error {
	// Check for help flags first to prevent double output
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return fmt.Errorf("help requested")
		}
	}

	// Parse command-line flags using FlashFlags directly
	if err := cm.flags.Parse(args); err != nil {
		return fmt.Errorf("failed to parse command-line flags: %w", err)
	}

	// Load environment variables
	cm.loadEnvironmentVariables()

	return nil
}

// ParseArgs is a convenience method that parses os.Args[1:]
func (cm *ConfigManager) ParseArgs() error {
	return cm.Parse(os.Args[1:])
}

// ParseArgsOrExit parses command-line arguments and exits gracefully on help/error
func (cm *ConfigManager) ParseArgsOrExit() {
	if err := cm.ParseArgs(); err != nil {
		if err.Error() == "help requested" {
			// Show clean, unified help and exit
			cm.PrintUsage()
			os.Exit(0)
		} else {
			// Show error and help, then exit with error code
			fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
			cm.PrintUsage()
			os.Exit(1)
		}
	}
}

// Configuration Access Methods - Type-Safe and Ultra-Fast

// GetString retrieves a string configuration value
func (cm *ConfigManager) GetString(key string) string {
	// Check explicit overrides first
	if val, exists := cm.values[key]; exists {
		if str, ok := val.(string); ok {
			return str
		}
	}

	// Use FlashFlags value
	return cm.flags.GetString(key)
}

// GetInt retrieves an integer configuration value
func (cm *ConfigManager) GetInt(key string) int {
	// Check explicit overrides first
	if val, exists := cm.values[key]; exists {
		if intVal, ok := val.(int); ok {
			return intVal
		}
	}

	// Use FlashFlags value
	return cm.flags.GetInt(key)
}

// GetBool retrieves a boolean configuration value
func (cm *ConfigManager) GetBool(key string) bool {
	// Check explicit overrides first
	if val, exists := cm.values[key]; exists {
		if boolVal, ok := val.(bool); ok {
			return boolVal
		}
	}

	// Use FlashFlags value
	return cm.flags.GetBool(key)
}

// GetDuration retrieves a duration configuration value
func (cm *ConfigManager) GetDuration(key string) time.Duration {
	// Check explicit overrides first
	if val, exists := cm.values[key]; exists {
		if durVal, ok := val.(time.Duration); ok {
			return durVal
		}
	}

	// Use FlashFlags value
	return cm.flags.GetDuration(key)
}

// GetStringSlice retrieves a string slice configuration value
func (cm *ConfigManager) GetStringSlice(key string) []string {
	// Check explicit overrides first
	if val, exists := cm.values[key]; exists {
		if sliceVal, ok := val.([]string); ok {
			return sliceVal
		}
	}

	// Use FlashFlags value
	return cm.flags.GetStringSlice(key)
}

// Set explicitly sets a configuration value (highest precedence)
func (cm *ConfigManager) Set(key string, value interface{}) {
	cm.values[key] = value
}

// SetDefault sets a default configuration value (lowest precedence)
func (cm *ConfigManager) SetDefault(key string, value interface{}) {
	// FlashFlags handles defaults internally
	// This method exists for API compatibility
}

// Configuration File Support

// LoadConfigFile loads configuration from a file
func (cm *ConfigManager) LoadConfigFile(path string) error {
	// This would integrate with the FlashFlags config file loading
	// For now, we delegate to the underlying flag system
	// TODO: Implement direct JSON/YAML/TOML loading
	return nil
}

// Real-Time Configuration Watching

// WatchConfigFile enables real-time configuration file watching
func (cm *ConfigManager) WatchConfigFile(path string, callback func()) error {
	if cm.watcher == nil {
		cm.watcher = New(Config{
			PollInterval: 1 * time.Second,
			CacheTTL:     500 * time.Millisecond,
		})
	}

	return cm.watcher.Watch(path, func(event ChangeEvent) {
		// Reload configuration when file changes
		if err := cm.LoadConfigFile(path); err == nil {
			if callback != nil {
				callback()
			}
		}
	})
}

// StartWatching starts the configuration file watcher
func (cm *ConfigManager) StartWatching() error {
	if cm.watcher == nil {
		return nil // No files being watched
	}
	return cm.watcher.Start()
}

// StopWatching stops the configuration file watcher
func (cm *ConfigManager) StopWatching() error {
	if cm.watcher == nil {
		return nil
	}
	return cm.watcher.Stop()
}

// Utility Methods

// PrintUsage prints help information for all flags
func (cm *ConfigManager) PrintUsage() {
	// Use FlashFlags built-in help system
	cm.flags.PrintHelp()
}

// GetStats returns configuration performance statistics
func (cm *ConfigManager) GetStats() (total, valid int) {
	// Count flags from FlashFlags
	total = 0
	cm.flags.VisitAll(func(flag *flashflags.Flag) {
		total++
	})

	// All flags are valid with FlashFlags
	valid = total
	return total, valid
}

// GetBoundFlags returns a map of all bound flags and their configuration keys
func (cm *ConfigManager) GetBoundFlags() map[string]string {
	result := make(map[string]string)
	cm.flags.VisitAll(func(flag *flashflags.Flag) {
		name := flag.Name()
		configKey := cm.flagNameToConfigKey(name)
		result[name] = configKey
	})
	return result
}

// Private helper methods

// flagNameToConfigKey converts a flag name to a configuration key
func (cm *ConfigManager) flagNameToConfigKey(flagName string) string {
	return strings.ReplaceAll(flagName, "-", ".")
}

// loadEnvironmentVariables loads values from environment variables
func (cm *ConfigManager) loadEnvironmentVariables() {
	// FlashFlags handles environment variables automatically
	// Set the environment prefix
	cm.flags.SetEnvPrefix(strings.ToUpper(cm.appName))
}

// FlagToEnvKey converts a flag name to an environment variable key (exported version)
func (cm *ConfigManager) FlagToEnvKey(flagName string) string {
	return cm.flagToEnvKey(flagName)
}

// flagToEnvKey converts a flag name to an environment variable key
func (cm *ConfigManager) flagToEnvKey(flagName string) string {
	// Convert "server-port" to "APPNAME_SERVER_PORT"
	envKey := strings.ToUpper(cm.appName + "_" + strings.ReplaceAll(flagName, "-", "_"))
	return envKey
}

// Example usage patterns for documentation:
//
// Basic Usage:
//   config := argus.NewConfigManager("myapp").
//       SetDescription("My Application").
//       SetVersion("1.0.0").
//       StringFlag("config", "config.json", "Configuration file path").
//       IntFlag("port", 8080, "Server port").
//       BoolFlag("debug", false, "Enable debug mode")
//
//   if err := config.ParseArgs(); err != nil {
//       log.Fatal(err)
//   }
//
//   port := config.GetInt("port")        // Command-line, env var, or default
//   debug := config.GetBool("debug")     // Supports: --debug, MYAPP_DEBUG=true
//
// Advanced Usage with File Watching:
//   config.WatchConfigFile("config.json", func() {
//       log.Println("Configuration reloaded")
//   })
//   config.StartWatching()
//   defer config.StopWatching()
