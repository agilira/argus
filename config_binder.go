// config_binder.go - Ultra-fast configuration binding system
//
// This module implements a high-performance configuration binding pattern
// that eliminates reflection overhead while providing excellent developer experience.
// It follows the "bind pattern" approach for zero-allocation config binding.
//
// Copyright (c) 2025 AGILira
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unsafe"
)

// bindKind represents the type of binding for ultra-fast type switching
type bindKind uint8

const (
	bindString bindKind = iota
	bindInt
	bindInt64
	bindBool
	bindFloat64
	bindDuration
)

// binding represents a single configuration binding with minimal memory footprint
type binding struct {
	target   unsafe.Pointer // Raw pointer to target variable
	key      string         // Configuration key (e.g., "database.host")
	defValue string         // Default value as string (universal representation)
	kind     bindKind       // Type of binding for fast switching
}

// ConfigBinder provides ultra-fast configuration binding with fluent API
type ConfigBinder struct {
	bindings []binding              // Pre-allocated slice of bindings
	config   map[string]interface{} // Configuration source
	err      error                  // Accumulated error state
}

// NewConfigBinder creates a new high-performance configuration binder
func NewConfigBinder(config map[string]interface{}) *ConfigBinder {
	return &ConfigBinder{
		bindings: make([]binding, 0, 16), // Pre-allocate for common case
		config:   config,
	}
}

// BindString binds a string configuration value with optional default
func (cb *ConfigBinder) BindString(target *string, key string, defaultValue ...string) *ConfigBinder {
	if cb.err != nil {
		return cb // Fast path: skip if already in error state
	}

	defVal := ""
	if len(defaultValue) > 0 {
		defVal = defaultValue[0]
	}

	cb.bindings = append(cb.bindings, binding{
		target:   unsafe.Pointer(target), // #nosec G103 - intentional unsafe.Pointer usage for zero-reflection binding
		key:      key,
		defValue: defVal,
		kind:     bindString,
	})

	return cb
}

// BindInt binds an integer configuration value with optional default
func (cb *ConfigBinder) BindInt(target *int, key string, defaultValue ...int) *ConfigBinder {
	if cb.err != nil {
		return cb
	}

	defVal := "0"
	if len(defaultValue) > 0 {
		defVal = strconv.Itoa(defaultValue[0])
	}

	cb.bindings = append(cb.bindings, binding{
		target:   unsafe.Pointer(target), // #nosec G103 - intentional unsafe.Pointer usage for zero-reflection binding
		key:      key,
		defValue: defVal,
		kind:     bindInt,
	})

	return cb
}

// BindInt64 binds an int64 configuration value with optional default
func (cb *ConfigBinder) BindInt64(target *int64, key string, defaultValue ...int64) *ConfigBinder {
	if cb.err != nil {
		return cb
	}

	defVal := "0"
	if len(defaultValue) > 0 {
		defVal = strconv.FormatInt(defaultValue[0], 10)
	}

	cb.bindings = append(cb.bindings, binding{
		target:   unsafe.Pointer(target), // #nosec G103 - intentional unsafe.Pointer usage for zero-reflection binding
		key:      key,
		defValue: defVal,
		kind:     bindInt64,
	})

	return cb
}

// BindBool binds a boolean configuration value with optional default
func (cb *ConfigBinder) BindBool(target *bool, key string, defaultValue ...bool) *ConfigBinder {
	if cb.err != nil {
		return cb
	}

	defVal := "false"
	if len(defaultValue) > 0 && defaultValue[0] {
		defVal = "true"
	}

	cb.bindings = append(cb.bindings, binding{
		target:   unsafe.Pointer(target), // #nosec G103 - intentional unsafe.Pointer usage for zero-reflection binding
		key:      key,
		defValue: defVal,
		kind:     bindBool,
	})

	return cb
}

// BindFloat64 binds a float64 configuration value with optional default
func (cb *ConfigBinder) BindFloat64(target *float64, key string, defaultValue ...float64) *ConfigBinder {
	if cb.err != nil {
		return cb
	}

	defVal := "0.0"
	if len(defaultValue) > 0 {
		defVal = strconv.FormatFloat(defaultValue[0], 'f', -1, 64)
	}

	cb.bindings = append(cb.bindings, binding{
		target:   unsafe.Pointer(target), // #nosec G103 - intentional unsafe.Pointer usage for zero-reflection binding
		key:      key,
		defValue: defVal,
		kind:     bindFloat64,
	})

	return cb
}

// BindDuration binds a time.Duration configuration value with optional default
func (cb *ConfigBinder) BindDuration(target *time.Duration, key string, defaultValue ...time.Duration) *ConfigBinder {
	if cb.err != nil {
		return cb
	}

	defVal := "0s"
	if len(defaultValue) > 0 {
		defVal = defaultValue[0].String()
	}

	cb.bindings = append(cb.bindings, binding{
		target:   unsafe.Pointer(target), // #nosec G103 - intentional unsafe.Pointer usage for zero-reflection binding
		key:      key,
		defValue: defVal,
		kind:     bindDuration,
	})

	return cb
}

// Apply executes all bindings in a single optimized pass
// This is where the magic happens - ultra-fast batch processing
func (cb *ConfigBinder) Apply() error {
	if cb.err != nil {
		return cb.err
	}

	// Single loop - maximum performance
	for _, b := range cb.bindings {
		if err := cb.applyBinding(b); err != nil {
			return fmt.Errorf("failed to bind key '%s': %w", b.key, err)
		}
	}

	return nil
}

// applyBinding applies a single binding with zero-allocation type switching
func (cb *ConfigBinder) applyBinding(b binding) error {
	// Get value from config with nested key support
	value, exists := cb.getValue(b.key)
	if !exists {
		// Use default value
		value = b.defValue
	}

	// Ultra-fast type switching without reflection
	switch b.kind {
	case bindString:
		*(*string)(b.target) = cb.toString(value)
	case bindInt:
		val, err := cb.toInt(value)
		if err != nil {
			return err
		}
		*(*int)(b.target) = val
	case bindInt64:
		val, err := cb.toInt64(value)
		if err != nil {
			return err
		}
		*(*int64)(b.target) = val
	case bindBool:
		val, err := cb.toBool(value)
		if err != nil {
			return err
		}
		*(*bool)(b.target) = val
	case bindFloat64:
		val, err := cb.toFloat64(value)
		if err != nil {
			return err
		}
		*(*float64)(b.target) = val
	case bindDuration:
		val, err := cb.toDuration(value)
		if err != nil {
			return err
		}
		*(*time.Duration)(b.target) = val
	default:
		return fmt.Errorf("unsupported binding kind: %d", b.kind)
	}

	return nil
}

// getValue retrieves a value from config with support for nested keys (e.g., "database.host")
func (cb *ConfigBinder) getValue(key string) (interface{}, bool) {
	if !strings.Contains(key, ".") {
		// Simple key - direct lookup
		val, exists := cb.config[key]
		return val, exists
	}

	// Nested key - traverse the map
	parts := strings.Split(key, ".")
	current := cb.config

	for i, part := range parts {
		val, exists := current[part]
		if !exists {
			return nil, false
		}

		if i == len(parts)-1 {
			// Last part - return the value
			return val, true
		}

		// Intermediate part - must be a map
		if nestedMap, ok := val.(map[string]interface{}); ok {
			current = nestedMap
		} else {
			return nil, false
		}
	}

	return nil, false
}

// Type conversion methods with minimal allocations

func (cb *ConfigBinder) toString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (cb *ConfigBinder) toInt(value interface{}) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("cannot convert %T to int", value)
	}
}

func (cb *ConfigBinder) toInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to int64", value)
	}
}

func (cb *ConfigBinder) toBool(value interface{}) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		return strconv.ParseBool(v)
	case int:
		return v != 0, nil
	case int64:
		return v != 0, nil
	case float64:
		return v != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", value)
	}
}

func (cb *ConfigBinder) toFloat64(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", value)
	}
}

func (cb *ConfigBinder) toDuration(value interface{}) (time.Duration, error) {
	switch v := value.(type) {
	case time.Duration:
		return v, nil
	case string:
		return time.ParseDuration(v)
	case int64:
		return time.Duration(v), nil
	case int:
		return time.Duration(v), nil
	default:
		return 0, fmt.Errorf("cannot convert %T to time.Duration", value)
	}
}

// BindFromConfig creates a new ConfigBinder from a parsed configuration map
// This is the main entry point for users
func BindFromConfig(config map[string]interface{}) *ConfigBinder {
	return NewConfigBinder(config)
}
