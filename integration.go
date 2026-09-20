// integration.go: Unified Integration Layer for Argus + FlashFlags
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

// Package argus provides unified configuration management combining:
// - FlashFlags ultra-fast command-line parsing
// - Lock-free configuration management
// - Multi-source configuration (flags, env vars, config files, defaults)
// - Real-time configuration watching with BoreasLite

package argus

import (
	stderrors "errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	flashflags "github.com/agilira/flash-flags"
	"github.com/agilira/go-errors"
)

// ErrHelpRequested is the sentinel returned by ConfigManager.Parse when the
// arguments asked for help rather than for work.
//
// Callers detect it with IsHelpRequested. Comparing error strings does not
// work: go-errors renders an *Error as "[CODE]: message", so the historical
// `err.Error() == "help requested"` check in ParseArgsOrExit never matched and
// --help exited 1 with an error on stderr. The sentinel carries its own error
// code because go-errors matches errors.Is on the code alone — sharing
// ErrCodeInvalidConfig would have made every configuration error look like a
// help request.
var ErrHelpRequested = errors.New(ErrCodeHelpRequested, "help requested")

// IsHelpRequested reports whether err signals that help was requested.
func IsHelpRequested(err error) bool {
	return stderrors.Is(err, ErrHelpRequested)
}

// ConfigManager combines all configuration sources in a unified interface.
// Integrates FlashFlags for command-line parsing with Argus file watching
// and provides a fluent API for configuration management.
//
// Key features:
//   - Ultra-fast command-line parsing via FlashFlags
//   - Real-time configuration file watching
//   - Multi-source configuration (flags, env vars, files, defaults)
//   - Type-safe configuration access
//   - Automatic environment variable mapping
//
// Configuration sources, highest precedence first:
//
//  1. Set          — an explicit override from the application
//  2. flags        — a flag the command line or the environment actually set
//  3. LoadConfigFile — values parsed from a configuration file
//  4. flags        — the default declared when the flag was registered
//  5. SetDefault   — a default for a key with no registered flag
//
// Layers 3 and 5 live in maps guarded by mu, because WatchConfigFile reloads
// the file from the watcher's goroutine while the application reads values
// from its own.
type ConfigManager struct {
	// FlashFlags for ultra-fast command-line parsing
	flags *flashflags.FlagSet

	// Optional file watcher for real-time config updates
	watcher *Watcher

	// Application metadata
	appName        string
	appDescription string
	appVersion     string

	// mu guards values, fileValues and defaults.
	mu sync.RWMutex

	// Configuration storage for explicit overrides
	values map[string]interface{}

	// Values parsed from the configuration file, if one was loaded
	fileValues map[string]interface{}

	// Fallbacks registered through SetDefault
	defaults map[string]interface{}
}

// NewConfigManager creates a unified configuration manager with FlashFlags integration.
// The appName is used for environment variable prefixing and help text generation.
//
// Example:
//
//	config := argus.NewConfigManager("myapp").
//	    SetDescription("My Application").
//	    SetVersion("1.0.0").
//	    StringFlag("port", "8080", "Server port")
func NewConfigManager(appName string) *ConfigManager {
	return &ConfigManager{
		flags:    flashflags.New(appName),
		appName:  appName,
		values:   make(map[string]interface{}),
		defaults: make(map[string]interface{}),
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

// Parse parses command-line arguments and binds them to configuration.
//
// Environment lookup is enabled before parsing, not after: FlashFlags reads the
// environment from inside Parse, so a prefix installed afterwards arrives too
// late and every APPNAME_* variable is ignored.
//
// --help and -h are left to FlashFlags, which only claims a spelling the
// application has not registered itself. Intercepting those two strings here
// made an application's own --help flag unreachable.
func (cm *ConfigManager) Parse(args []string) error {
	cm.flags.SetEnvPrefix(strings.ToUpper(cm.appName))

	if err := cm.flags.Parse(args); err != nil {
		// FlashFlags signals help with this exact message (see its Parse doc).
		if err.Error() == "help requested" {
			return ErrHelpRequested
		}
		return errors.Wrap(err, ErrCodeInvalidConfig, "failed to parse command-line flags")
	}

	return nil
}

// ParseArgs is a convenience method that parses os.Args[1:]
func (cm *ConfigManager) ParseArgs() error {
	return cm.Parse(os.Args[1:])
}

// ParseArgsOrExit parses command-line arguments and exits gracefully on help/error
func (cm *ConfigManager) ParseArgsOrExit() {
	if err := cm.ParseArgs(); err != nil {
		if IsHelpRequested(err) {
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

// resolve returns the value for key from the highest-precedence source that
// has one, together with whether the flag layer should be consulted for its
// registered default. See the ConfigManager doc comment for the full order.
//
// fromFlag is true when a registered flag was actually set (command line or
// environment); in that case the caller reads the typed value straight from
// FlashFlags rather than converting an interface{}.
func (cm *ConfigManager) resolve(key string) (value interface{}, fromFlag bool, found bool) {
	cm.mu.RLock()
	override, hasOverride := cm.values[key]
	cm.mu.RUnlock()
	if hasOverride {
		return override, false, true
	}

	if cm.flags.Changed(key) {
		return nil, true, true
	}

	cm.mu.RLock()
	fileValues := cm.fileValues
	cm.mu.RUnlock()
	if fileValues != nil {
		if fileValue, ok := lookupConfigValue(fileValues, key); ok {
			return fileValue, false, true
		}
	}

	if cm.flags.Lookup(key) != nil {
		return nil, true, true // registered flag: use its declared default
	}

	cm.mu.RLock()
	fallback, hasFallback := cm.defaults[key]
	cm.mu.RUnlock()
	if hasFallback {
		return fallback, false, true
	}

	return nil, false, false
}

// GetString retrieves a string configuration value
func (cm *ConfigManager) GetString(key string) string {
	value, fromFlag, found := cm.resolve(key)
	if fromFlag || !found {
		return cm.flags.GetString(key)
	}
	return convertToString(value)
}

// GetInt retrieves an integer configuration value
func (cm *ConfigManager) GetInt(key string) int {
	value, fromFlag, found := cm.resolve(key)
	if fromFlag || !found {
		return cm.flags.GetInt(key)
	}
	if converted, err := convertToInt(value); err == nil {
		return converted
	}
	return cm.flags.GetInt(key)
}

// GetBool retrieves a boolean configuration value
func (cm *ConfigManager) GetBool(key string) bool {
	value, fromFlag, found := cm.resolve(key)
	if fromFlag || !found {
		return cm.flags.GetBool(key)
	}
	if converted, err := convertToBool(value); err == nil {
		return converted
	}
	return cm.flags.GetBool(key)
}

// GetDuration retrieves a duration configuration value
func (cm *ConfigManager) GetDuration(key string) time.Duration {
	value, fromFlag, found := cm.resolve(key)
	if fromFlag || !found {
		return cm.flags.GetDuration(key)
	}
	if converted, err := convertToDuration(value); err == nil {
		return converted
	}
	return cm.flags.GetDuration(key)
}

// GetFloat64 retrieves a float64 configuration value
func (cm *ConfigManager) GetFloat64(key string) float64 {
	value, fromFlag, found := cm.resolve(key)
	if fromFlag || !found {
		return cm.flags.GetFloat64(key)
	}
	if converted, err := convertToFloat64(value); err == nil {
		return converted
	}
	return cm.flags.GetFloat64(key)
}

// GetStringSlice retrieves a string slice configuration value
func (cm *ConfigManager) GetStringSlice(key string) []string {
	value, fromFlag, found := cm.resolve(key)
	if fromFlag || !found {
		return cm.flags.GetStringSlice(key)
	}

	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			result = append(result, convertToString(item))
		}
		return result
	case string:
		if typed == "" {
			return nil
		}
		parts := strings.Split(typed, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	default:
		return cm.flags.GetStringSlice(key)
	}
}

// Set explicitly sets a configuration value (highest precedence)
func (cm *ConfigManager) Set(key string, value interface{}) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.values[key] = value
}

// SetDefault registers a fallback for a key with no registered flag (lowest
// precedence).
//
// A registered flag always carries its own default, which is more specific
// than this one and therefore wins; SetDefault exists for keys that only ever
// come from a configuration file.
func (cm *ConfigManager) SetDefault(key string, value interface{}) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.defaults[key] = value
}

// Configuration File Support

// LoadConfigFile loads configuration from a file.
//
// The format is detected from the extension and the whole set of supported
// formats applies (JSON, YAML, TOML, HCL, INI, Properties). Values land in the
// configuration-file layer: below anything the command line or the environment
// set, above the defaults declared with the flags. A key is matched flat first
// and then as a nested path, so both "server.port" as a literal key and a
// nested server: { port: } resolve.
//
// A file that does not exist, cannot be read, or does not parse is an error.
// The previous implementation returned nil without reading anything, which
// made WatchConfigFile report success while reloading nothing.
func (cm *ConfigManager) LoadConfigFile(path string) error {
	if err := ValidateSecurePath(path); err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "unsafe configuration file path").
			WithContext("path", path)
	}

	format := DetectFormat(path)
	if format == FormatUnknown {
		return errors.New(ErrCodeInvalidConfig, "unsupported configuration file format").
			WithContext("path", path)
	}

	// #nosec G304 -- path is validated by ValidateSecurePath above
	data, err := os.ReadFile(path)
	if err != nil {
		return errors.Wrap(err, ErrCodeFileNotFound, "failed to read configuration file").
			WithContext("path", path)
	}

	parsed, err := ParseConfig(data, format)
	if err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "failed to parse "+format.String()+" configuration file").
			WithContext("path", path)
	}

	cm.mu.Lock()
	cm.fileValues = parsed
	cm.mu.Unlock()

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
