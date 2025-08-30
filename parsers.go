// parsers.go: Universal configuration file parsers for Argus
//
// This file provides parsing support for major configuration formats,
// making Argus truly universal and not a "one-trick pony".
//
// Supported Formats:
// - JSON (.json) - Full production support
// - YAML (.yml, .yaml) - Simple built-in + plugin support
// - TOML (.toml) - Simple built-in + plugin support
// - HCL (.hcl, .tf) - Simple built-in + plugin support
// - INI/Config (.ini, .conf, .cfg) - Simple built-in + plugin support
// - Properties (.properties) - Simple built-in + plugin support
//
// Parser Architecture:
// - Built-in parsers: Simple, fast, zero-dependency for 80% use cases
// - Plugin parsers: Full-featured external parsers for complex production needs
// - Automatic fallback: Try plugins first, fallback to built-in
//
// Copyright (c) 2025 AGILira
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// ConfigFormat represents supported configuration file formats for auto-detection.
// Used by the format detection system to determine appropriate parser selection.
type ConfigFormat int

const (
	FormatJSON ConfigFormat = iota
	FormatYAML
	FormatTOML
	FormatHCL
	FormatINI
	FormatProperties
	FormatUnknown
)

// ConfigParser defines the interface for pluggable configuration parsers
//
// PRODUCTION PARSER INTEGRATION:
// Go binaries are compiled statically, so "plugins" work via compile-time registration:
//
//  1. IMPORT-BASED REGISTRATION (Recommended):
//     Users import parser libraries that auto-register in init():
//
//     import _ "github.com/your-org/argus-yaml-pro"   // Registers advanced YAML parser
//     import _ "github.com/your-org/argus-toml-pro"   // Registers advanced TOML parser
//
//  2. MANUAL REGISTRATION:
//     Users manually register parsers in their main():
//
//     argus.RegisterParser(&MyAdvancedYAMLParser{})
//
//  3. BUILD TAGS (Advanced):
//     Conditional compilation for different parser sets:
//
//     go build -tags "yaml_pro,toml_pro" ./...
//
// Built-in parsers handle 80% of use cases with zero dependencies.
// Production parsers provide full spec compliance and advanced features.
type ConfigParser interface {
	// Parse parses configuration data for supported formats
	Parse(data []byte) (map[string]interface{}, error)

	// Supports returns true if this parser can handle the given format
	Supports(format ConfigFormat) bool

	// Name returns a human-readable name for this parser (for debugging)
	Name() string
}

// Global registry of custom parsers (production environments can register advanced parsers)
var (
	customParsers []ConfigParser
	parserMutex   sync.RWMutex
)

// RegisterParser registers a custom parser for production use cases.
// Custom parsers are tried before built-in parsers, allowing for full
// specification compliance or advanced features not available in built-in parsers.
//
// Example:
//
//	argus.RegisterParser(&MyAdvancedYAMLParser{})
//
// Or via import-based registration:
//
//	import _ "github.com/your-org/argus-yaml-pro"
func RegisterParser(parser ConfigParser) {
	parserMutex.Lock()
	defer parserMutex.Unlock()
	customParsers = append(customParsers, parser)
}

// configMapPool is a sync.Pool for reusing map[string]interface{} to reduce allocations
var configMapPool = sync.Pool{
	New: func() interface{} {
		return make(map[string]interface{})
	},
}

// getConfigMap gets a map from the pool and clears it for reuse.
// Part of the memory optimization system to reduce allocations during parsing.
func getConfigMap() map[string]interface{} {
	config := configMapPool.Get().(map[string]interface{})
	// Clear the map for reuse
	for k := range config {
		delete(config, k)
	}
	return config
}

// putConfigMap returns a map to the pool for reuse.
// Should be called when a map is no longer needed to prevent memory leaks.
func putConfigMap(config map[string]interface{}) {
	configMapPool.Put(config)
}

// String returns the string representation of the config format for debugging and logging.
func (cf ConfigFormat) String() string {
	switch cf {
	case FormatJSON:
		return "JSON"
	case FormatYAML:
		return "YAML"
	case FormatTOML:
		return "TOML"
	case FormatHCL:
		return "HCL"
	case FormatINI:
		return "INI"
	case FormatProperties:
		return "Properties"
	default:
		return "Unknown"
	}
}

// DetectFormat detects the configuration format from file extension
// HYPER-OPTIMIZED: Zero allocations, perfect hashing, unrolled loops
// Note: High cyclomatic complexity (38) is justified for optimal performance
// across 7 configuration formats with zero memory allocation
func DetectFormat(filePath string) ConfigFormat {
	length := len(filePath)
	if length < 3 { // Minimum: ".tf"
		return FormatUnknown
	}

	// Fast backward scan with unrolled loop for common extensions
	// Most files are short, so unrolling the common cases is faster

	// Check last 11 chars for .properties (longest extension)
	if length >= 11 &&
		filePath[length-11] == '.' &&
		(filePath[length-10]|32) == 'p' && // |32 converts to lowercase
		(filePath[length-9]|32) == 'r' &&
		(filePath[length-8]|32) == 'o' &&
		(filePath[length-7]|32) == 'p' &&
		(filePath[length-6]|32) == 'e' &&
		(filePath[length-5]|32) == 'r' &&
		(filePath[length-4]|32) == 't' &&
		(filePath[length-3]|32) == 'i' &&
		(filePath[length-2]|32) == 'e' &&
		(filePath[length-1]|32) == 's' {
		return FormatProperties
	}

	// Check last 8 chars for .config
	if length >= 8 &&
		filePath[length-8] == '.' &&
		(filePath[length-7]|32) == 'c' &&
		(filePath[length-6]|32) == 'o' &&
		(filePath[length-5]|32) == 'n' &&
		(filePath[length-4]|32) == 'f' &&
		(filePath[length-3]|32) == 'i' &&
		(filePath[length-2]|32) == 'g' {
		return FormatINI
	}

	// Check last 5 chars for common extensions: .json, .yaml, .toml, .conf
	if length >= 5 && filePath[length-5] == '.' {
		b1, b2, b3, b4 := filePath[length-4]|32, filePath[length-3]|32, filePath[length-2]|32, filePath[length-1]|32
		// Perfect hash for 4-char extensions
		switch uint32(b1)<<24 | uint32(b2)<<16 | uint32(b3)<<8 | uint32(b4) {
		case 0x6a736f6e: // "json"
			return FormatJSON
		case 0x79616d6c: // "yaml"
			return FormatYAML
		case 0x746f6d6c: // "toml"
			return FormatTOML
		case 0x636f6e66: // "conf"
			return FormatINI
		}
	}

	// Check last 4 chars for: .yml, .hcl, .ini, .cfg
	if length >= 4 && filePath[length-4] == '.' {
		b1, b2, b3 := filePath[length-3]|32, filePath[length-2]|32, filePath[length-1]|32
		// Perfect hash for 3-char extensions
		switch uint32(b1)<<16 | uint32(b2)<<8 | uint32(b3) {
		case 0x796d6c: // "yml"
			return FormatYAML
		case 0x68636c: // "hcl"
			return FormatHCL
		case 0x696e69: // "ini"
			return FormatINI
		case 0x636667: // "cfg"
			return FormatINI
		}
	}

	// Check last 3 chars for: .tf
	if length >= 3 && filePath[length-3] == '.' {
		b1, b2 := filePath[length-2]|32, filePath[length-1]|32
		if b1 == 't' && b2 == 'f' {
			return FormatHCL
		}
	}

	return FormatUnknown
}

// ParseConfig parses configuration data based on the detected format.
// Tries custom parsers first, then falls back to built-in parsers.
// HYPER-OPTIMIZED: Fast path for no custom parsers, reduced lock contention.
//
// Parameters:
//   - data: Raw configuration file bytes
//   - format: Detected configuration format
//
// Returns:
//   - map[string]interface{}: Parsed configuration data
//   - error: Any parsing errors
func ParseConfig(data []byte, format ConfigFormat) (map[string]interface{}, error) {
	// Fast path: Check if we have any custom parsers without locking
	// This is safe because customParsers is only appended to, never modified
	if len(customParsers) == 0 {
		// No custom parsers, go straight to built-in
		return parseBuiltin(data, format)
	}

	// Slow path: Check custom parsers with minimal lock time
	parserMutex.RLock()
	for _, parser := range customParsers {
		if parser.Supports(format) {
			config, err := parser.Parse(data)
			parserMutex.RUnlock()
			return config, err
		}
	}
	parserMutex.RUnlock()

	// No custom parser found, use built-in
	return parseBuiltin(data, format)
}

// parseBuiltin handles built-in parsing without any locks for maximum performance.
// Used as fallback when no custom parsers are available or applicable.
func parseBuiltin(data []byte, format ConfigFormat) (map[string]interface{}, error) {
	switch format {
	case FormatJSON:
		return parseJSON(data)
	case FormatYAML:
		return parseYAML(data)
	case FormatTOML:
		return parseTOML(data)
	case FormatHCL:
		return parseHCL(data)
	case FormatINI:
		return parseINI(data)
	case FormatProperties:
		return parseProperties(data)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// parseValue attempts to parse a string value into the appropriate type.
// Supports automatic type detection for booleans, integers, floats, and strings.
// Used by simple parsers to provide basic type conversion without schemas.
func parseValue(value string) interface{} {
	// Try boolean
	if strings.ToLower(value) == "true" {
		return true
	}
	if strings.ToLower(value) == "false" {
		return false
	}

	// Try integer
	if intVal, err := strconv.Atoi(value); err == nil {
		return intVal
	}

	// Try float
	if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
		return floatVal
	}

	// Return as string
	return value
}
