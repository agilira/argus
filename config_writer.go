// config_writer.go: Configuration Writing System for Argus
//
// This file implements zero-allocation configuration writing with atomic operations,
// format preservation, and comprehensive audit integration.
//
// Philosophy:
// - Zero allocations in hot paths through pre-allocated buffers
// - Atomic operations to prevent corruption during concurrent access
// - Format preservation maintains original structure and style
// - Comprehensive audit trail for all modifications
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"encoding/json"
	"fmt"
	"hash"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/agilira/go-errors"
	"go.yaml.in/yaml/v4"
)

// ConfigWriter provides zero-allocation configuration writing capabilities.
// It maintains the original format and structure while enabling programmatic
// modifications with full audit integration.
//
// Performance characteristics:
//   - SetValue: 115 ns/op, 32 B, 1 alloc for simple keys
//   - WriteConfig: 17 us/op on a local filesystem, I/O bound
//   - Memory usage: Fixed 8KB + config size
//
// Thread safety: Safe for concurrent reads, serialized writes
type ConfigWriter struct {
	// Core configuration
	filePath string
	format   ConfigFormat

	// Pre-allocated buffers for zero-allocation operations
	keyBuffer   []string // Reused for dot-notation parsing
	valueBuffer []byte   // Reused for serialization
	tempBuffer  []byte   // Reused for file operations

	// Current state - copy-on-write semantics
	config       map[string]interface{}
	originalHash uint64 // Fast dirty detection

	// doc is a YAML file's document, edited along with config so that what
	// the writer does not change (comments, order, the rest) is written as
	// it was; nil for the other formats and for a YAML file made anew.
	doc *yaml.Node

	// Optional audit integration
	auditLogger *AuditLogger // Optional - can be nil for performance

	// Thread safety
	mu      sync.RWMutex
	writing bool // Prevents concurrent writes
}

// ConfigFormat is already defined in parsers.go - we reuse it here

// newConfigWriter creates a new ConfigWriter instance with pre-allocated buffers.
// Internal constructor - not exposed to prevent misuse.
//
// Performance: 1,114 ns/op, 3,408 B, 5 allocs
// NewConfigWriter creates a new zero-allocation configuration writer.
// The buffer sizes are optimized for typical configuration files.
//
// Performance: 1,114 ns/op, 3,408 B, 5 allocs
func NewConfigWriter(filePath string, format ConfigFormat, initialConfig map[string]interface{}) (*ConfigWriter, error) {
	return NewConfigWriterWithAudit(filePath, format, initialConfig, nil)
}

// NewConfigWriterWithAudit creates a new configuration writer with optional audit logging.
// Provides the same performance characteristics as NewConfigWriter with optional audit integration.
//
// Parameters:
//   - filePath: Path to the configuration file
//   - format: Configuration format (JSON, YAML, TOML, etc.)
//   - initialConfig: Initial configuration data (can be nil)
//   - auditLogger: Optional audit logger for compliance (can be nil for performance)
//
// Performance: 1,114 ns/op with no audit logger; an audit logger adds one
// buffered write per operation (~5us)
func NewConfigWriterWithAudit(filePath string, format ConfigFormat, initialConfig map[string]interface{}, auditLogger *AuditLogger) (*ConfigWriter, error) {
	if filePath == "" {
		return nil, errors.New(ErrCodeConfigWriterError, "filePath cannot be empty")
	}

	writer := &ConfigWriter{
		filePath:    filePath,
		format:      format,
		keyBuffer:   make([]string, 0, 8),  // Pre-allocate for deep nesting
		valueBuffer: make([]byte, 0, 1024), // 1KB buffer for serialization
		tempBuffer:  make([]byte, 0, 2048), // 2KB buffer for temp operations
		auditLogger: auditLogger,           // Optional audit integration
	}

	// Initialize with provided configuration
	if initialConfig != nil {
		writer.config = deepCopy(initialConfig)
		// For new files, originalHash should be 0 so WriteConfig() always writes the first time
		writer.originalHash = 0
		return writer, nil
	}
	// No configuration given: the file's, when there is one. A writer that
	// started empty replaced an existing file with only the keys it was
	// then given, every other entry lost (22/11).
	writer.config = make(map[string]interface{})
	if _, err := os.Stat(filePath); err == nil {
		if err := writer.reloadFromFile(); err != nil {
			return nil, errors.Wrap(err, ErrCodeInvalidConfig, "cannot read the configuration to edit")
		}
	}
	return writer, nil
}

// SetValue sets a configuration value using dot notation.
// Supports nested keys like "database.connection.host".
//
// Performance: 115 ns/op, 32 B, 1 alloc for simple keys
//
//	149 ns/op, 48 B, 1 alloc for nested keys (map creation)
//
// Examples:
//
//	writer.SetValue("port", 8080)                    // Simple key
//	writer.SetValue("database.host", "localhost")    // Nested key
//	writer.SetValue("features.auth.enabled", true)   // Deep nesting
func (w *ConfigWriter) SetValue(key string, value interface{}) error {
	if key == "" {
		return errors.New(ErrCodeInvalidConfig, "key cannot be empty")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writing {
		return errors.New(ErrCodeWatcherBusy, "writer is currently performing atomic write")
	}

	// Parse dot notation using pre-allocated buffer
	w.keyBuffer = w.keyBuffer[:0] // Reset without allocation
	w.keyBuffer = parseDotNotation(key, w.keyBuffer)

	// Take a snapshot before modification for audit purposes
	var oldConfig map[string]interface{}
	if w.auditLogger != nil {
		oldConfig = deepCopy(w.config)
	}

	// The document first: a value it cannot hold leaves both unchanged.
	if w.doc != nil {
		if err := setYAMLPath(w.doc, w.keyBuffer, value); err != nil {
			return errors.Wrap(err, ErrCodeInvalidConfig, "failed to set nested value")
		}
	}
	// Apply change to config copy
	if err := w.setNestedValue(w.config, w.keyBuffer, value); err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig, "failed to set nested value")
	}

	// Audit logging for configuration changes (optional, zero overhead when disabled)
	if w.auditLogger != nil {
		w.auditLogger.LogConfigChange(w.filePath, oldConfig, w.config)
	}

	return nil
}

// WriteConfig atomically writes the current configuration to disk.
// Uses temporary file + rename for atomic operation to prevent corruption.
//
// Performance: 17 us/op, 1,048 B, 29 allocs on a local filesystem; I/O bound
//
//	Memory: 0 additional allocations for serialization
//
// Atomicity guarantee: Either succeeds completely or leaves original unchanged
func (w *ConfigWriter) WriteConfig() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writing {
		return errors.New(ErrCodeConfigWriterError, "concurrent write operation in progress")
	}

	w.writing = true
	defer func() { w.writing = false }()

	// Check if changes exist (fast hash comparison)
	currentHash := hashConfig(w.config)
	if currentHash == w.originalHash {
		return nil // No changes to write
	}

	// Serialize using pre-allocated buffer
	w.valueBuffer = w.valueBuffer[:0] // Reset without allocation
	serialized, err := w.serializeConfig(w.config, w.valueBuffer)
	if err != nil {
		return errors.Wrap(err, ErrCodeSerializationError, "serialization failed")
	}

	// Atomic write operation
	if err := w.atomicWrite(serialized); err != nil {
		return errors.Wrap(err, ErrCodeIOError, "atomic write failed")
	}

	// Update hash after successful write
	w.originalHash = currentHash

	// Audit logging for file write operations (optional)
	if w.auditLogger != nil {
		w.auditLogger.LogFileWatch("config_written", w.filePath)
	}

	return nil
}

// WriteConfigAs writes the configuration to a different file path.
// Useful for backups or exporting to different locations.
//
// The original file path and watcher remain unchanged.
func (w *ConfigWriter) WriteConfigAs(filePath string) error {
	if filePath == "" {
		return errors.New(ErrCodeConfigWriterError, "filePath cannot be empty")
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	// Serialize current config
	w.valueBuffer = w.valueBuffer[:0]
	serialized, err := w.serializeConfig(w.config, w.valueBuffer)
	if err != nil {
		return errors.Wrap(err, ErrCodeSerializationError, "serialization failed")
	}

	// Write to target file atomically
	if err := writeFileAtomically(filePath, serialized); err != nil {
		return errors.Wrap(err, ErrCodeIOError, "failed to write configuration file")
	}

	// Audit logging for file export operations (optional)
	if w.auditLogger != nil {
		w.auditLogger.LogFileWatch("config_exported", filePath)
	}

	return nil
}

// GetValue retrieves a configuration value using dot notation.
// Returns nil if the key doesn't exist.
//
// Performance: 24.5 ns/op, 0 allocs for a top-level key; 111 ns/op, 1 alloc nested
func (w *ConfigWriter) GetValue(key string) interface{} {
	if key == "" {
		return nil
	}

	w.mu.RLock()
	defer w.mu.RUnlock()

	w.keyBuffer = w.keyBuffer[:0]
	w.keyBuffer = parseDotNotation(key, w.keyBuffer)

	return w.getNestedValue(w.config, w.keyBuffer)
}

// HasChanges returns true if the configuration has unsaved changes.
// Uses fast hash comparison for O(1) performance.
func (w *ConfigWriter) HasChanges() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return hashConfig(w.config) != w.originalHash
}

// GetConfig returns a deep copy of the current configuration.
// This enables CLI operations like list and convert without exposing internal state.
//
// Performance: O(n) where n is config size, allocates new map
func (w *ConfigWriter) GetConfig() map[string]interface{} {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return deepCopy(w.config)
}

// DeleteValue removes a configuration key using dot notation.
// Returns true if the key existed and was deleted, false otherwise.
//
// Performance: 190 ns/op, 0 allocs
//
// Examples:
//
//	writer.DeleteValue("port")                    // Delete simple key
//	writer.DeleteValue("database.host")          // Delete nested key
//	writer.DeleteValue("features.auth.enabled")  // Delete deep nested key
func (w *ConfigWriter) DeleteValue(key string) bool {
	if key == "" {
		return false
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writing {
		return false // Cannot delete during write operation
	}

	// Take a snapshot before modification for audit purposes
	var oldConfig map[string]interface{}
	if w.auditLogger != nil {
		oldConfig = deepCopy(w.config)
	}

	// Parse dot notation using pre-allocated buffer
	w.keyBuffer = w.keyBuffer[:0]
	w.keyBuffer = parseDotNotation(key, w.keyBuffer)

	// Delete from config
	deleted := w.deleteNestedValue(w.config, w.keyBuffer)
	if deleted && w.doc != nil {
		deleteYAMLPath(w.doc, w.keyBuffer)
	}

	// Audit logging for configuration changes (optional)
	if deleted && w.auditLogger != nil {
		w.auditLogger.LogConfigChange(w.filePath, oldConfig, w.config)
	}

	return deleted
}

// ListKeys returns all configuration keys in dot notation format.
// Optionally filters by prefix for hierarchical listing.
//
// Performance: O(n) where n is total number of keys
func (w *ConfigWriter) ListKeys(prefix string) []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var keys []string
	w.collectKeys(w.config, "", prefix, &keys)
	sort.Strings(keys) // the same list each time: a map has no order
	return keys
}

// Reset discards all changes and reverts to the last saved state.
// Loads the configuration from the original file to restore its state.
// Useful for canceling operations or handling errors.
//
// Performance: I/O bound, reads and parses original file
func (w *ConfigWriter) Reset() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.writing {
		return errors.New(ErrCodeWatcherBusy, "cannot reset during write operation")
	}

	// Take a snapshot before reset for audit purposes
	var oldConfig map[string]interface{}
	if w.auditLogger != nil {
		oldConfig = deepCopy(w.config)
	}

	// Try to reload from original file
	if err := w.reloadFromFile(); err != nil {
		// If file doesn't exist or can't be read, reset to empty config
		w.config = make(map[string]interface{})
		w.originalHash = hashConfig(w.config)

		// Audit the reset operation (optional)
		if w.auditLogger != nil {
			w.auditLogger.LogConfigChange(w.filePath, oldConfig, w.config)
		}

		return errors.Wrap(err, ErrCodeIOError, "failed to reload from file, reset to empty config")
	}

	// Audit the successful reset operation (optional)
	if w.auditLogger != nil {
		w.auditLogger.LogConfigChange(w.filePath, oldConfig, w.config)
	}

	return nil
}

// reloadFromFile loads the configuration from the original file.
// Internal method that handles file reading, parsing, and state update.
func (w *ConfigWriter) reloadFromFile() error {
	// Check if file exists
	if _, err := os.Stat(w.filePath); os.IsNotExist(err) {
		// File doesn't exist, reset to empty config
		w.config = make(map[string]interface{})
		w.originalHash = hashConfig(w.config)
		w.doc = nil
		return nil
	}

	// Read file content
	data, err := os.ReadFile(w.filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// The document first (YAML): its errors, then the values' checks.
	var doc *yaml.Node
	if w.format == FormatYAML {
		if doc, err = loadYAMLDocument(data); err != nil {
			return fmt.Errorf("failed to parse configuration: %w", err)
		}
	}
	config, err := ParseConfig(data, w.format)
	if err != nil {
		return fmt.Errorf("failed to parse configuration: %w", err)
	}
	w.doc = doc

	// Update internal state
	w.config = config
	w.originalHash = hashConfig(w.config)

	return nil
}

// parseDotNotation splits a dot-notation key into components.
// Reuses provided buffer to avoid allocations.
//
// Performance: 88 ns/op, 48 B, 1 alloc for a three-part key
func parseDotNotation(key string, buffer []string) []string {
	if !strings.Contains(key, ".") {
		// Simple key - no splitting needed
		return append(buffer, key)
	}

	// Split by dots, reusing buffer
	parts := strings.Split(key, ".")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			buffer = append(buffer, part)
		}
	}

	return buffer
}

// setNestedValue sets a value in nested map structure using key path.
// Creates intermediate maps as needed.
//
// Complexity: O(depth), typically 2-4 levels
func (w *ConfigWriter) setNestedValue(config map[string]interface{}, keyPath []string, value interface{}) error {
	if len(keyPath) == 0 {
		return errors.New(ErrCodeConfigWriterError, "empty key path")
	}

	// Check the path first: a path refused half-way left the maps made
	// before the refusal in the configuration (22/11).
	probe := config
	for _, key := range keyPath[:len(keyPath)-1] {
		next, exists := probe[key]
		if !exists {
			break // the rest is made
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return errors.New(ErrCodeConfigWriterError,
				fmt.Sprintf("key '%s' is not a map, cannot set nested value", key))
		}
		probe = nextMap
	}

	current := config

	// Navigate to parent of target key
	for i := 0; i < len(keyPath)-1; i++ {
		key := keyPath[i]

		next, exists := current[key]
		if !exists {
			// Create intermediate map
			next = make(map[string]interface{})
			current[key] = next
		}

		current = next.(map[string]interface{}) // checked above
	}

	// Set final value
	finalKey := keyPath[len(keyPath)-1]
	current[finalKey] = value

	return nil
}

// getNestedValue retrieves a value from nested map structure.
// Returns nil if path doesn't exist.
//
// Performance: 29 ns/op, 0 allocs
func (w *ConfigWriter) getNestedValue(config map[string]interface{}, keyPath []string) interface{} {
	current := config

	for i, key := range keyPath {
		value, exists := current[key]
		if !exists {
			return nil
		}

		// The final key's value. By position: compared by name, a.b.a ended
		// at the first a (22/11).
		if i == len(keyPath)-1 {
			return value
		}

		// Continue navigation
		nextMap, ok := value.(map[string]interface{})
		if !ok {
			return nil
		}

		current = nextMap
	}

	return nil
}

// deleteNestedValue deletes a key from nested map structure using key path.
// Returns true if the key existed and was deleted, false otherwise.
//
// Performance: O(depth), typically 2-4 levels
func (w *ConfigWriter) deleteNestedValue(config map[string]interface{}, keyPath []string) bool {
	if len(keyPath) == 0 {
		return false
	}

	// Special case: single key
	if len(keyPath) == 1 {
		key := keyPath[0]
		if _, exists := config[key]; exists {
			delete(config, key)
			return true
		}
		return false
	}

	current := config

	// Navigate to parent of target key
	for i := 0; i < len(keyPath)-1; i++ {
		key := keyPath[i]

		next, exists := current[key]
		if !exists {
			return false // Path doesn't exist
		}

		// Type assertion for nested maps
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return false // Cannot traverse non-map
		}

		current = nextMap
	}

	// Delete final key
	finalKey := keyPath[len(keyPath)-1]
	if _, exists := current[finalKey]; exists {
		delete(current, finalKey)
		return true
	}

	return false
}

// collectKeys recursively collects all keys in dot notation format.
// Filters by prefix if provided.
func (w *ConfigWriter) collectKeys(config map[string]interface{}, currentPrefix, filterPrefix string, keys *[]string) {
	for key, value := range config {
		fullKey := key
		if currentPrefix != "" {
			fullKey = currentPrefix + "." + key
		}

		// Apply prefix filter
		if filterPrefix != "" && !strings.HasPrefix(fullKey, filterPrefix) {
			// Skip if this key doesn't match, but continue for nested structures
			if nestedMap, ok := value.(map[string]interface{}); ok {
				// Check if any nested key could match the prefix
				if strings.HasPrefix(filterPrefix, fullKey+".") {
					w.collectKeys(nestedMap, fullKey, filterPrefix, keys)
				}
			}
			continue
		}

		if nestedMap, ok := value.(map[string]interface{}); ok {
			// Recursively collect nested keys
			w.collectKeys(nestedMap, fullKey, filterPrefix, keys)
		} else {
			// Add leaf key
			*keys = append(*keys, fullKey)
		}
	}
}

// atomicWrite performs atomic file write using temporary file + rename.
func (w *ConfigWriter) atomicWrite(data []byte) error {
	return writeFileAtomically(w.filePath, data)
}

// writeFileAtomically writes data to path through a temporary file in the same
// directory, then renames it into place.
//
// UNPREDICTABLE NAME: the temporary file comes from os.CreateTemp, which opens
// with O_CREATE|O_EXCL and a random suffix. The previous name was
// "<path>.tmp.<UnixNano>" written with os.WriteFile, which has no O_EXCL: a
// guessed name pre-created as a symlink would have redirected the write
// (CWE-377). CreateTemp also creates the file 0600 from the start.
//
// DURABILITY: the temporary file is fsynced before the rename, and the
// directory is fsynced after it. Rename alone is atomic with respect to other
// processes, but on a crash the metadata can land before the data and leave a
// zero-length configuration file — the very corruption this function exists to
// prevent.
func writeFileAtomically(path string, data []byte) error {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tmp.Name()

	discard := func() {
		_ = tmp.Close()
		_ = os.Remove(tempPath)
	}

	if _, err := tmp.Write(data); err != nil {
		discard()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		discard()
		return fmt.Errorf("failed to flush temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return syncDir(dir)
}

// syncDir flushes a directory entry so a completed rename survives a crash.
//
// Windows does not allow opening a directory as a file and provides the
// guarantee through the filesystem instead, so the operation is skipped there
// rather than reported as a failure.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	handle, err := os.Open(dir) // #nosec G304 -- dir is the parent of a path the caller already owns
	if err != nil {
		return fmt.Errorf("failed to open directory for sync: %w", err)
	}
	defer func() { _ = handle.Close() }()

	if err := handle.Sync(); err != nil {
		return fmt.Errorf("failed to sync directory: %w", err)
	}

	return nil
}

// serializeConfig converts the configuration map to the original format.
// Uses pre-allocated buffer to minimize allocations.
func (w *ConfigWriter) serializeConfig(config map[string]interface{}, buffer []byte) ([]byte, error) {
	switch w.format {
	case FormatJSON:
		return serializeJSON(config, buffer)
	case FormatYAML:
		return w.serializeYAML(config)
	case FormatTOML:
		return serializeTOML(config, buffer)
	case FormatHCL:
		return serializeHCL(config, buffer)
	case FormatINI:
		return serializeINI(config, buffer)
	case FormatProperties:
		return serializeProperties(config, buffer)
	default:
		return nil, fmt.Errorf("unsupported format: %v", w.format)
	}
}

// Helper functions for zero-allocation operations

// deepCopy creates a deep copy of the configuration map.
// Uses pre-allocated buffers where possible.
func deepCopy(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}

	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		switch val := v.(type) {
		case map[string]interface{}:
			dst[k] = deepCopy(val)
		case []interface{}:
			dst[k] = deepCopySlice(val)
		default:
			dst[k] = val
		}
	}
	return dst
}

// deepCopySlice creates a deep copy of a slice.
func deepCopySlice(src []interface{}) []interface{} {
	if src == nil {
		return nil
	}

	dst := make([]interface{}, len(src))
	for i, v := range src {
		switch val := v.(type) {
		case map[string]interface{}:
			dst[i] = deepCopy(val)
		case []interface{}:
			dst[i] = deepCopySlice(val)
		default:
			dst[i] = val
		}
	}
	return dst
}

// hashConfig computes a fast hash of the configuration for change detection.
// Uses FNV-1a for speed and good distribution.
//
// Performance: 848 ns/op, 200 B, 17 allocs for a six-key config
func hashConfig(config map[string]interface{}) uint64 {
	if config == nil {
		return 0
	}

	h := fnv.New64a()
	hashValue(h, config)
	return h.Sum64()
}

// hashValue hashes a configuration value: each with its type, so 1 and
// "1" differ, floats exact (%f made 1e-7 and 0 the same), and a map's keys
// in order. They were not sorted, whatever the comment said: the same
// configuration hashed differently from one call to the next, and
// HasChanges and WriteConfig's skip of an unchanged file went by chance.
func hashValue(h hash.Hash64, v interface{}) {
	write := func(s string) { _, _ = h.Write([]byte(s)) } // #nosec G104 -- hash.Write never fails
	if n, ok := formatNumber(v); ok {
		write("n:" + n + ";")
		return
	}
	switch val := v.(type) {
	case nil:
		write("nil;")
	case bool:
		write("b:" + strconv.FormatBool(val) + ";")
	case string:
		write("s:" + strconv.Itoa(len(val)) + ":" + val)
	case map[string]interface{}:
		write("m:" + strconv.Itoa(len(val)) + "{")
		for _, k := range sortedConfigKeys(val) {
			write(strconv.Itoa(len(k)) + ":" + k)
			hashValue(h, val[k])
		}
		write("}")
	case []interface{}:
		write("l:" + strconv.Itoa(len(val)) + "[")
		for _, item := range val {
			hashValue(h, item)
		}
		write("]")
	default:
		write(fmt.Sprintf("o:%T:%v;", val, val))
	}
}

// serializeJSON converts configuration to JSON format with proper indentation.
// Reuses the provided buffer when possible to minimize allocations.
// Returns formatted JSON bytes ready for file writing.
func serializeJSON(config map[string]interface{}, buffer []byte) ([]byte, error) {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeSerializationError, "JSON marshal failed")
	}

	// Reuse buffer if it has sufficient capacity
	if cap(buffer) >= len(data) {
		buffer = buffer[:len(data)]
		copy(buffer, data)
		return buffer, nil
	}

	return data, nil
}

// serializeYAML converts configuration to YAML format using built-in serialization.
// Provides basic YAML output compatible with most parsers without external dependencies.
// For advanced YAML features, register a custom parser using RegisterParser().
//
// Performance: Zero allocations when buffer has sufficient capacity
// serializeYAML writes the document the writer edits, or, for a file it
// makes or converts, the values with their keys sorted. Through the YAML
// encoder: quoting, escapes and numbers exact (a hand-written writer put
// 1234567.5 as 1.23457e+06 and left keys with : or # unquoted).
func (w *ConfigWriter) serializeYAML(config map[string]interface{}) ([]byte, error) {
	doc := w.doc
	if doc == nil {
		var err error
		if doc, err = yamlDocumentOf(config); err != nil {
			return nil, errors.Wrap(err, ErrCodeSerializationError, "YAML encode failed")
		}
	}
	data, err := encodeYAMLDocument(doc)
	if err != nil {
		return nil, errors.Wrap(err, ErrCodeSerializationError, "YAML encode failed")
	}
	return data, nil
}
