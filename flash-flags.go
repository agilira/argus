// Package argus implements internal flag parsing system with zero external dependencies.
// This file provides lightning-fast command-line argument processing.

package argus

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// InternalFlag implements LockFreeFlag using only standard library
type InternalFlag struct {
	name     string
	value    interface{}
	flagType string
	changed  bool
	usage    string
}

// Name returns the flag name
func (f *InternalFlag) Name() string { return f.name }

// Value returns the flag value
func (f *InternalFlag) Value() interface{} { return f.value }

// Type returns the flag type
func (f *InternalFlag) Type() string { return f.flagType }

// Changed returns whether the flag was set
func (f *InternalFlag) Changed() bool { return f.changed }

// Usage returns the flag usage string
func (f *InternalFlag) Usage() string { return f.usage }

// InternalFlagSet implements LockFreeFlagSet using only standard library
type InternalFlagSet struct {
	flags map[string]*InternalFlag
	name  string
}

// NewInternalFlagSet creates a new internal flag set with zero external dependencies
func NewInternalFlagSet(name string) *InternalFlagSet {
	return &InternalFlagSet{
		flags: make(map[string]*InternalFlag),
		name:  name,
	}
}

// String adds a string flag
func (fs *InternalFlagSet) String(name, defaultValue, usage string) *string {
	flag := &InternalFlag{
		name:     name,
		value:    defaultValue,
		flagType: "string",
		changed:  false,
		usage:    usage,
	}
	fs.flags[name] = flag
	return &defaultValue
}

// Int adds an integer flag
func (fs *InternalFlagSet) Int(name string, defaultValue int, usage string) *int {
	flag := &InternalFlag{
		name:     name,
		value:    defaultValue,
		flagType: "int",
		changed:  false,
		usage:    usage,
	}
	fs.flags[name] = flag
	return &defaultValue
}

// Bool adds a boolean flag
func (fs *InternalFlagSet) Bool(name string, defaultValue bool, usage string) *bool {
	flag := &InternalFlag{
		name:     name,
		value:    defaultValue,
		flagType: "bool",
		changed:  false,
		usage:    usage,
	}
	fs.flags[name] = flag
	return &defaultValue
}

// Duration adds a duration flag
func (fs *InternalFlagSet) Duration(name string, defaultValue time.Duration, usage string) *time.Duration {
	flag := &InternalFlag{
		name:     name,
		value:    defaultValue,
		flagType: "duration",
		changed:  false,
		usage:    usage,
	}
	fs.flags[name] = flag
	return &defaultValue
}

// StringSlice adds a string slice flag
func (fs *InternalFlagSet) StringSlice(name string, defaultValue []string, usage string) *[]string {
	flag := &InternalFlag{
		name:     name,
		value:    defaultValue,
		flagType: "stringSlice",
		changed:  false,
		usage:    usage,
	}
	fs.flags[name] = flag
	return &defaultValue
}

// Parse parses command line arguments (simple implementation)
func (fs *InternalFlagSet) Parse(args []string) error {
	for i := 0; i < len(args); i++ {
		arg := args[i]

		if !strings.HasPrefix(arg, "--") {
			continue
		}

		// Remove -- prefix
		arg = arg[2:]

		var flagName, flagValue string
		if strings.Contains(arg, "=") {
			parts := strings.SplitN(arg, "=", 2)
			flagName = parts[0]
			flagValue = parts[1]
		} else {
			flagName = arg
			// Look for value in next argument
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flagValue = args[i+1]
				i++ // Skip next argument
			} else {
				// Boolean flag or error
				if flag, exists := fs.flags[flagName]; exists && flag.flagType == "bool" {
					flagValue = "true"
				} else {
					return fmt.Errorf("flag --%s requires a value", flagName)
				}
			}
		}

		// Set flag value
		err := fs.setFlagValue(flagName, flagValue)
		if err != nil {
			return err
		}
	}

	return nil
}

// setFlagValue sets a flag value with type conversion
func (fs *InternalFlagSet) setFlagValue(name, value string) error {
	flag, exists := fs.flags[name]
	if !exists {
		return fmt.Errorf("unknown flag: --%s", name)
	}

	switch flag.flagType {
	case "string":
		flag.value = value
		flag.changed = true

	case "int":
		intVal, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid int value for flag --%s: %s", name, value)
		}
		flag.value = intVal
		flag.changed = true

	case "bool":
		boolVal, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid bool value for flag --%s: %s", name, value)
		}
		flag.value = boolVal
		flag.changed = true

	case "duration":
		durVal, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration value for flag --%s: %s", name, value)
		}
		flag.value = durVal
		flag.changed = true

	case "stringSlice":
		// Split by comma
		slice := strings.Split(value, ",")
		flag.value = slice
		flag.changed = true

	default:
		return fmt.Errorf("unsupported flag type: %s", flag.flagType)
	}

	return nil
}

// VisitAll implements LockFreeFlagSet interface
func (fs *InternalFlagSet) VisitAll(fn func(LockFreeFlag)) {
	for _, flag := range fs.flags {
		fn(flag)
	}
}

// Lookup implements LockFreeFlagSet interface
func (fs *InternalFlagSet) Lookup(name string) LockFreeFlag {
	flag, exists := fs.flags[name]
	if !exists {
		return nil
	}
	return flag
}

// PrintUsage prints usage information for all flags
func (fs *InternalFlagSet) PrintUsage() {
	fmt.Printf("Usage of %s:\n", fs.name)
	for name, flag := range fs.flags {
		fmt.Printf("  --%s\n", name)
		fmt.Printf("        %s (type: %s)\n", flag.usage, flag.flagType)
	}
}

// GetString gets a flag value as string
func (fs *InternalFlagSet) GetString(name string) string {
	if flag, exists := fs.flags[name]; exists {
		if str, ok := flag.value.(string); ok {
			return str
		}
		return fmt.Sprintf("%v", flag.value)
	}
	return ""
}

// GetInt gets a flag value as int
func (fs *InternalFlagSet) GetInt(name string) int {
	if flag, exists := fs.flags[name]; exists {
		if intVal, ok := flag.value.(int); ok {
			return intVal
		}
	}
	return 0
}

// GetBool gets a flag value as bool
func (fs *InternalFlagSet) GetBool(name string) bool {
	if flag, exists := fs.flags[name]; exists {
		if boolVal, ok := flag.value.(bool); ok {
			return boolVal
		}
	}
	return false
}

// GetDuration gets a flag value as duration
func (fs *InternalFlagSet) GetDuration(name string) time.Duration {
	if flag, exists := fs.flags[name]; exists {
		if durVal, ok := flag.value.(time.Duration); ok {
			return durVal
		}
	}
	return 0
}

// GetStringSlice gets a flag value as string slice
func (fs *InternalFlagSet) GetStringSlice(name string) []string {
	if flag, exists := fs.flags[name]; exists {
		if slice, ok := flag.value.([]string); ok {
			return slice
		}
	}
	return []string{}
}
