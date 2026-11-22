// config_writer_formats.go: TOML, HCL, INI and Properties, written
//
// Copyright (c) 2026 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// The text formats are written from the values, as argus's parsers read
// them back: keys in order (a map has none, so the same configuration
// came out in a different order at each write), every level of nesting
// (TOML and HCL wrote one level and printed the rest as Go's map
// syntax), numbers exact (%.6g turned 1234567.5 into 1.23457e+06).

func sortedConfigKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// formatNumber is v exactly, as Go reads it back.
func formatNumber(v interface{}) (string, bool) {
	switch n := v.(type) {
	case int:
		return strconv.Itoa(n), true
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", n), true
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", n), true
	case float32:
		return formatFloat(float64(n), 32), true
	case float64:
		return formatFloat(n, 64), true
	}
	return "", false
}

func formatFloat(f float64, bits int) string {
	switch {
	case math.IsInf(f, 1):
		return "inf"
	case math.IsInf(f, -1):
		return "-inf"
	case math.IsNaN(f):
		return "nan"
	}
	s := strconv.FormatFloat(f, 'g', -1, bits)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0" // still a float when read back
	}
	return s
}

// serializeTOML writes the top-level values, then a table per mapping,
// [a.b] for nested ones.
func serializeTOML(config map[string]interface{}, buffer []byte) ([]byte, error) {
	var lines []string
	writeTOMLTable(&lines, "", config)
	return reuse(buffer, []byte(strings.Join(lines, "\n")+"\n")), nil
}

func writeTOMLTable(lines *[]string, name string, table map[string]interface{}) {
	if name != "" {
		if len(*lines) > 0 {
			*lines = append(*lines, "")
		}
		*lines = append(*lines, "["+name+"]")
	}
	keys := sortedConfigKeys(table)
	for _, k := range keys {
		if _, isTable := table[k].(map[string]interface{}); !isTable {
			*lines = append(*lines, k+" = "+formatTOMLValue(table[k]))
		}
	}
	for _, k := range keys {
		if sub, isTable := table[k].(map[string]interface{}); isTable {
			full := k
			if name != "" {
				full = name + "." + k
			}
			writeTOMLTable(lines, full, sub)
		}
	}
}

// formatTOMLValue is a value as argus's TOML parser reads it.
func formatTOMLValue(value interface{}) string {
	if n, ok := formatNumber(value); ok {
		return n
	}
	switch v := value.(type) {
	case nil:
		return `""`
	case bool:
		return strconv.FormatBool(v)
	case string:
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	case []interface{}:
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = formatTOMLValue(item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	return fmt.Sprintf(`"%v"`, value)
}

// serializeHCL writes attributes, then a block per mapping, nested.
func serializeHCL(config map[string]interface{}, buffer []byte) ([]byte, error) {
	var lines []string
	writeHCLBody(&lines, "", config)
	return reuse(buffer, []byte(strings.Join(lines, "\n")+"\n")), nil
}

func writeHCLBody(lines *[]string, indent string, body map[string]interface{}) {
	keys := sortedConfigKeys(body)
	for _, k := range keys {
		if _, isBlock := body[k].(map[string]interface{}); !isBlock {
			*lines = append(*lines, indent+k+" = "+formatHCLValue(body[k]))
		}
	}
	for _, k := range keys {
		if sub, isBlock := body[k].(map[string]interface{}); isBlock {
			*lines = append(*lines, indent+k+" {")
			writeHCLBody(lines, indent+"  ", sub)
			*lines = append(*lines, indent+"}")
		}
	}
}

// formatHCLValue is a value as argus's HCL parser reads it.
func formatHCLValue(value interface{}) string {
	if n, ok := formatNumber(value); ok {
		return n
	}
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(v)
	case string:
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	case []interface{}:
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = formatHCLValue(item)
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	return fmt.Sprintf(`"%v"`, value)
}

// serializeINI writes the top-level values, then a [section] per mapping;
// a mapping inside one is [section.sub], as INI has one level.
func serializeINI(config map[string]interface{}, buffer []byte) ([]byte, error) {
	var lines []string
	writeINISection(&lines, "", config)
	return reuse(buffer, []byte(strings.Join(lines, "\n")+"\n")), nil
}

func writeINISection(lines *[]string, name string, section map[string]interface{}) {
	if name != "" {
		if len(*lines) > 0 {
			*lines = append(*lines, "")
		}
		*lines = append(*lines, "["+name+"]")
	}
	keys := sortedConfigKeys(section)
	for _, k := range keys {
		if _, isSection := section[k].(map[string]interface{}); !isSection {
			*lines = append(*lines, k+"="+formatPlainValue(section[k]))
		}
	}
	for _, k := range keys {
		if sub, isSection := section[k].(map[string]interface{}); isSection {
			full := k
			if name != "" {
				full = name + "." + k
			}
			writeINISection(lines, full, sub)
		}
	}
}

// serializeProperties writes key=value, nested keys in dot notation,
// sorted.
func serializeProperties(config map[string]interface{}, buffer []byte) ([]byte, error) {
	flat := flattenConfig(config, "")
	var lines []string
	for _, k := range sortedConfigKeys(flat) {
		lines = append(lines, k+"="+formatPlainValue(flat[k]))
	}
	return reuse(buffer, []byte(strings.Join(lines, "\n")+"\n")), nil
}

// formatPlainValue is a value of INI and Properties, unquoted.
func formatPlainValue(value interface{}) string {
	if n, ok := formatNumber(value); ok {
		return n
	}
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

// flattenConfig converts nested maps to flat key-value pairs using dot notation
func flattenConfig(config map[string]interface{}, prefix string) map[string]interface{} {
	result := make(map[string]interface{})
	for key, value := range config {
		fullKey := key
		if prefix != "" {
			fullKey = prefix + "." + key
		}
		if nested, ok := value.(map[string]interface{}); ok {
			for nestedKey, nestedValue := range flattenConfig(nested, fullKey) {
				result[nestedKey] = nestedValue
			}
			continue
		}
		result[fullKey] = value
	}
	return result
}

// reuse puts data in buffer when it fits.
func reuse(buffer, data []byte) []byte {
	if cap(buffer) >= len(data) {
		buffer = buffer[:len(data)]
		copy(buffer, data)
		return buffer
	}
	return data
}
