// config_writer_formats.go: TOML, HCL, INI and Properties, written
//
// Copyright (c) 2026 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"encoding"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
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

// nestDottedKeys turns INI's and Properties' flat keys ("db.host", as
// their parsers give them) into the nesting the writer addresses with dot
// notation: kept flat, GetValue("db.host") found nothing and SetValue made
// a second db.host beside the first, which one written by chance (audit
// 22/11). A key that is both a value and a group is refused.
func nestDottedKeys(flat map[string]interface{}) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(flat))
	for _, key := range sortedConfigKeys(flat) {
		parts := strings.Split(key, ".")
		m := out
		for _, part := range parts[:len(parts)-1] {
			next, ok := m[part]
			if !ok {
				next = make(map[string]interface{})
				m[part] = next
			}
			nm, isMap := next.(map[string]interface{})
			if !isMap {
				return nil, fmt.Errorf("key %q is both a value and a group of keys", part)
			}
			m = nm
		}
		last := parts[len(parts)-1]
		value := flat[key]
		if nested, isMap := value.(map[string]interface{}); isMap {
			sub, err := nestDottedKeys(nested)
			if err != nil {
				return nil, err
			}
			value = sub
		}
		if _, exists := m[last]; exists { // "a.b" beside a group a holding b
			return nil, fmt.Errorf("key %q is given twice", key)
		}
		m[last] = value
	}
	return out, nil
}

// verifyRoundTrip reads data back with argus's own parser and refuses it
// unless it holds config: the line-based parsers of these formats read no
// escapes, no list of maps, no nested list, no empty INI section, and a
// value written that they read otherwise (a string "true" in INI, a quote
// in a TOML string) was a file that said something else (audit 22/11).
// The error names the key, never the value.
func verifyRoundTrip(data []byte, format ConfigFormat, config map[string]interface{}) error {
	back, err := ParseConfig(data, format)
	if err == nil && (format == FormatINI || format == FormatProperties) {
		back, err = nestDottedKeys(back)
	}
	if err != nil {
		return fmt.Errorf("%s cannot hold this configuration: the text written does not read back", format)
	}
	if path := firstDifference(config, back, ""); path != "" {
		return fmt.Errorf("%s cannot hold the value of %q as it is (quotes, newlines, list of maps or lists, an empty group, a string read as a number or a boolean): write it as YAML or JSON", format, path)
	}
	return nil
}

// firstDifference is the first key, in order, whose value differs
// between want and got, typed; "" when none.
func firstDifference(want, got map[string]interface{}, prefix string) string {
	keys := sortedConfigKeys(want)
	for k := range got {
		if _, ok := want[k]; !ok {
			keys = append(keys, k)
		}
	}
	for _, k := range keys {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		w, wok := want[k]
		g, gok := got[k]
		wm, wIsMap := w.(map[string]interface{})
		gm, gIsMap := g.(map[string]interface{})
		switch {
		case wok && !gok && wIsMap && len(wm) > 0:
			return firstDifference(wm, map[string]interface{}{}, path) // name the key lost, not its group
		case wok != gok:
			return path
		case wIsMap && gIsMap:
			if d := firstDifference(wm, gm, path); d != "" {
				return d
			}
		case hashConfig(map[string]interface{}{"v": w}) != hashConfig(map[string]interface{}{"v": g}):
			return path
		}
	}
	return ""
}

// withOwnEncoding is v, or a pointer to a copy of it when only the pointer
// has the encoding (big.Int by value: written {} otherwise, the encoders
// not calling a pointer method on a value); ok is false for a value
// without one.
func withOwnEncoding(v interface{}) (interface{}, bool) {
	switch v.(type) {
	case yaml.Marshaler, json.Marshaler, encoding.TextMarshaler:
		return v, true
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Pointer {
		return nil, false // checked above, on the pointer itself
	}
	pv := reflect.New(rv.Type())
	pv.Elem().Set(rv)
	switch pv.Interface().(type) {
	case yaml.Marshaler, json.Marshaler, encoding.TextMarshaler:
		return pv.Interface(), true
	}
	return nil, false
}

// maxValueDepth bounds a value's nesting: a value that holds itself (a
// map put inside itself) recursed without end, a crash recover cannot
// catch (audit 22/11).
const maxValueDepth = 1000

// normalizeGoValue holds a Go value of its own types as the parsers give
// values: a slice or an array as []interface{}, a map with string keys as
// map[string]interface{}, an integer of any width as int (uint64 above
// int's range stays uint64), a float32 as the float64 its text reads as,
// a pointer as what it points to, through every level. Others stay.
func normalizeGoValue(v interface{}, depth int) (interface{}, error) {
	if depth > maxValueDepth {
		return nil, fmt.Errorf("the value nests deeper than %d levels (does it hold itself?)", maxValueDepth)
	}
	switch x := v.(type) {
	case nil, string, bool, int, float64, []byte:
		return v, nil
	case *yaml.Node:
		return v, nil // the caller's document: encoded as it is
	case yaml.Node:
		return &x, nil // by value inside a map or a list, the encoder panicked (audit 22/11)
	case float32:
		f, _ := strconv.ParseFloat(strconv.FormatFloat(float64(x), 'g', -1, 32), 64)
		return f, nil
	}
	if own, ok := withOwnEncoding(v); ok {
		return own, nil // big.Int, net.IP, time.Time...: written as they say (audit 22/11: taken apart, an IP became 16 numbers)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if rv.Type().PkgPath() == "" { // int64, not time.Duration: that one keeps its meaning
			return int(rv.Int()), nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if rv.Type().PkgPath() == "" {
			if u := rv.Uint(); u <= math.MaxInt {
				return int(u), nil
			}
		}
	case reflect.Pointer:
		if rv.IsNil() {
			return nil, nil
		}
		return normalizeGoValue(rv.Elem().Interface(), depth+1)
	case reflect.Slice, reflect.Array:
		out := make([]interface{}, rv.Len())
		for i := range out {
			item, err := normalizeGoValue(rv.Index(i).Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			out[i] = item
		}
		return out, nil
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return v, nil
		}
		out := make(map[string]interface{}, rv.Len())
		for _, k := range rv.MapKeys() {
			item, err := normalizeGoValue(rv.MapIndex(k).Interface(), depth+1)
			if err != nil {
				return nil, err
			}
			out[k.String()] = item
		}
		return out, nil
	}
	return v, nil
}
