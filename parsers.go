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
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/agilira/go-errors"
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

// Global registry of custom parsers (production environments can register
// advanced parsers).
//
// The list is held behind an atomic.Pointer and replaced wholesale on
// registration, so the read on the parse path is genuinely lock-free. It used
// to be a plain slice read outside the mutex on the fast path, justified by a
// comment claiming "append-only is safe": appending rewrites the slice header,
// so that read raced with every registration and the race detector confirms it.
var (
	customParsers atomic.Pointer[[]ConfigParser]
	parserMutex   sync.Mutex // serialises registration only
)

// loadParsers returns the current registry snapshot, never nil.
func loadParsers() []ConfigParser {
	if parsers := customParsers.Load(); parsers != nil {
		return *parsers
	}
	return nil
}

// snapshotParsers returns a copy of the registry. Used by tests that need to
// restore the global state they modified.
func snapshotParsers() []ConfigParser {
	current := loadParsers()
	snapshot := make([]ConfigParser, len(current))
	copy(snapshot, current)
	return snapshot
}

// restoreParsers replaces the registry wholesale.
func restoreParsers(parsers []ConfigParser) {
	parserMutex.Lock()
	defer parserMutex.Unlock()
	replacement := make([]ConfigParser, len(parsers))
	copy(replacement, parsers)
	customParsers.Store(&replacement)
}

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

	current := loadParsers()
	updated := make([]ConfigParser, len(current), len(current)+1)
	copy(updated, current)
	updated = append(updated, parser)
	customParsers.Store(&updated)
}

// getConfigMap allocates the map a parser fills.
//
// ═══════════════════════════════════════════════════════════════════════════════
// ENGINEERING NOTE: Why There Is No Pool Here
// ═══════════════════════════════════════════════════════════════════════════════
// This used to draw from a sync.Pool, with a note claiming ~40% fewer parse
// allocations and ~15% more throughput. Neither number was reachable: a parsed
// map is RETURNED to the caller and handed on to user callbacks, which keep it
// for as long as they like. Nothing could put it back, so only the error paths
// ever fed the pool and the hot path allocated every time regardless.
//
// Worse, the shape invited a real bug: adding the "missing" putConfigMap on a
// success path would have recycled a map the application was still reading,
// handing the same map to two callers.
//
// Pooling can only return here if parsing stops handing its map to the caller.
// ═══════════════════════════════════════════════════════════════════════════════
func getConfigMap() map[string]interface{} {
	return make(map[string]interface{})
}

// putConfigMap releases a map a parser allocated but will not return, e.g. when
// parsing fails partway through. It exists so the error paths read naturally;
// there is no pool behind it (see getConfigMap).
func putConfigMap(config map[string]interface{}) {
	_ = config
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
//
// ═══════════════════════════════════════════════════════════════════════════════
// ENGINEERING NOTE: Sub-3ns Format Detection
// ═══════════════════════════════════════════════════════════════════════════════
// This function is called on EVERY config operation, so performance is critical.
// Traditional approaches would use:
//   - filepath.Ext() + strings.ToLower() + map lookup: ~50ns, 2 allocations
//   - regexp matching: ~500ns, multiple allocations
//
// Our approach achieves 2.79ns with ZERO allocations using these techniques:
//
//  1. BACKWARD SCANNING: We scan from the end of the string, not the beginning.
//     Config paths are typically 50-100 chars, but extensions are 3-11 chars.
//     We only examine the bytes we need.
//
//  2. INLINE CASE FOLDING: The |32 trick exploits ASCII encoding. For letters,
//     OR-ing with 32 converts uppercase to lowercase (A=65, a=97, diff=32).
//     This avoids strings.ToLower() which allocates a new string.
//
//  3. PERFECT HASH FOR 4-CHAR EXTENSIONS: We pack 4 bytes into a uint32 and
//     switch on it. The Go compiler turns this into a jump table - O(1) lookup.
//     Example: "json" becomes 0x6a736f6e = 1785688942.
//
//  4. UNROLLED LOOPS: For longer extensions (.properties, .config), we unroll
//     the comparison to avoid loop overhead. Each byte comparison is a single
//     CPU instruction.
//
// This might look like premature optimization, but when processing 1M+ configs
// per second in hot paths, these nanoseconds compound. The benchmark shows
// 2.79ns/op vs 50+ns for the naive approach - an 18x improvement.
// ═══════════════════════════════════════════════════════════════════════════════
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

	// Check last 7 chars for .config
	if length >= 7 &&
		filePath[length-7] == '.' &&
		(filePath[length-6]|32) == 'c' &&
		(filePath[length-5]|32) == 'o' &&
		(filePath[length-4]|32) == 'n' &&
		(filePath[length-3]|32) == 'f' &&
		(filePath[length-2]|32) == 'i' &&
		(filePath[length-1]|32) == 'g' {
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
	// One atomic load gives a stable snapshot for the whole call: registration
	// replaces the pointer rather than mutating the slice we are ranging over.
	parsers := loadParsers()
	if len(parsers) == 0 {
		// No custom parsers, go straight to built-in
		return parseBuiltin(data, format)
	}

	for _, parser := range parsers {
		if parser.Supports(format) {
			return parser.Parse(data)
		}
	}

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
		return nil, errors.New(ErrCodeInvalidConfig, "unsupported format: "+format.String())
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
