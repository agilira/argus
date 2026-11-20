// settings_bind_compose.go: binding maps, lists and pointers, and Strict
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// A single value is read key by key, through every layer: that is how a flag
// or an environment variable overrides one setting. A map, a list or a
// pointer is a tree, and a tree comes whole from a file or a remote source,
// so it is decoded from the subtree in one go, with the same tags and the same
// strictness at every depth. Each decode allocates its own maps and slices: a
// bound value never shares memory with the revision it came from, nor with
// another bound value.

package argus

import (
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/agilira/go-errors"
)

// BindOption changes how Bind reads the configuration.
type BindOption func(*bindPlan)

// Strict refuses a configuration holding a key the bound type does not name,
// at any depth, in the file and remote layers: a typo is an error, not a
// setting silently left at its default. The environment cannot be listed and
// is not checked.
func Strict() BindOption {
	return func(p *bindPlan) { p.strict = true }
}

// Validator is a bound type that checks itself: Bind calls Validate on every
// value it builds, and an error refuses the revision.
type Validator interface {
	Validate() error
}

// composite: a field type read whole from a subtree.
func composite(t reflect.Type) bool {
	if t == stringSliceType {
		return false
	}
	switch t.Kind() {
	case reflect.Map, reflect.Slice, reflect.Pointer:
		return true
	default:
		return false
	}
}

// checkComposable refuses, before anything runs, a type that holds
// somewhere a value no configuration can produce.
func checkComposable(t reflect.Type, seen map[reflect.Type]bool) error {
	if t == durationType || t == stringSliceType || bindable(t) || seen[t] {
		return nil
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice:
		return checkComposable(t.Elem(), seen)
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return errors.New(ErrCodeInvalidConfig, "argus: "+t.String()+": map keys must be strings")
		}
		return checkComposable(t.Elem(), seen)
	case reflect.Struct:
		return checkFieldsComposable(t, seen)
	default:
		return errors.New(ErrCodeInvalidConfig, "argus: no binding for type "+t.String())
	}
}

func checkFieldsComposable(t reflect.Type, seen map[reflect.Type]bool) error {
	seen[t] = true
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if _, _, tagged := parseBindTag(f.Tag.Get("argus")); !tagged || !f.IsExported() {
			continue
		}
		if err := checkComposable(f.Type, seen); err != nil {
			return errors.New(ErrCodeInvalidConfig, "argus: field "+t.String()+"."+f.Name+": "+err.Error())
		}
	}
	return nil
}

// decodeComposed fills target from raw, the subtree a source produced for
// key. A null leaves a pointer, a map or a list nil; a struct still gets
// its required fields checked.
func decodeComposed(target reflect.Value, raw interface{}, key string) error {
	t := target.Type()
	raw, present := nullAs(t, raw)
	if !present {
		return nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		elem := reflect.New(t.Elem())
		if err := decodeComposed(elem.Elem(), raw, key); err != nil {
			return err
		}
		target.Set(elem)
		return nil
	case reflect.Struct:
		if t != durationType {
			return decodeStruct(target, raw, key)
		}
	case reflect.Map:
		return decodeMap(target, raw, key)
	case reflect.Slice:
		return decodeSlice(target, raw, key)
	}
	return decodeScalar(target, raw, key)
}

// nullAs is what a null means for t: nothing, or for a struct (not a
// pointer to one) an empty map, so that its required fields are checked.
func nullAs(t reflect.Type, raw interface{}) (interface{}, bool) {
	if raw != nil && !nullFromYAML(t, raw) {
		return raw, true
	}
	if t.Kind() != reflect.Struct || t == durationType {
		return nil, false
	}
	return map[string]interface{}{}, true
}

// nullFromYAML: the YAML parser has always turned a null into an empty map,
// so an empty map where no map nor struct can go is the null it was. Where
// one can, a null reads as {}: an empty map, a struct of zero values.
func nullFromYAML(t reflect.Type, raw interface{}) bool {
	m, ok := raw.(map[string]interface{})
	if !ok || len(m) != 0 {
		return false
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() != reflect.Map && (t.Kind() != reflect.Struct || t == durationType)
}

// decodeScalar converts one value. A string field takes a scalar, never a
// map or a list, which would otherwise turn into their Go spelling.
func decodeScalar(target reflect.Value, raw interface{}, key string) error {
	if target.Kind() == reflect.String {
		if k := reflect.ValueOf(raw).Kind(); k == reflect.Map || k == reflect.Slice || k == reflect.Array {
			return errors.New(ErrCodeInvalidConfig, "cannot bind "+key+" ("+target.Type().String()+"): not a single value")
		}
	}
	if err := convertInto(target, raw); err != nil {
		return errors.Wrap(err, ErrCodeInvalidConfig,
			"cannot bind "+key+" ("+target.Type().String()+"): value "+convertToString(raw))
	}
	return nil
}

// asMap reads any map a source may produce (YAML with keys that are not
// strings, a parser's own types, Defaults written in Go) as one with
// string keys.
func asMap(raw interface{}) (map[string]interface{}, bool) {
	if m, ok := raw.(map[string]interface{}); ok {
		return m, true
	}
	v := reflect.ValueOf(raw)
	if v.Kind() != reflect.Map {
		return nil, false
	}
	m := make(map[string]interface{}, v.Len())
	iter := v.MapRange()
	for iter.Next() {
		m[convertToString(iter.Key().Interface())] = iter.Value().Interface()
	}
	return m, true
}

// asList reads any slice or array as a list.
func asList(raw interface{}) ([]interface{}, bool) {
	if l, ok := raw.([]interface{}); ok {
		return l, true
	}
	v := reflect.ValueOf(raw)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		return nil, false
	}
	l := make([]interface{}, v.Len())
	for i := range l {
		l[i] = v.Index(i).Interface()
	}
	return l, true
}

func notA(what, key string) error {
	return errors.New(ErrCodeInvalidConfig, "cannot bind "+key+": not a "+what)
}

// decodeStruct fills a struct from a map. A tag may be dotted, as at the
// top. Whether keys the struct does not name are an error is Strict's
// business, checked separately: here they are skipped.
func decodeStruct(target reflect.Value, raw interface{}, key string) error {
	m, ok := asMap(raw)
	if !ok {
		return notA("map", key)
	}
	t := target.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, required, tagged := parseBindTag(f.Tag.Get("argus"))
		if !tagged || !f.IsExported() {
			continue
		}
		value, found := lookupConfigValue(m, name)
		if !found {
			if required {
				return errors.New(ErrCodeInvalidConfig, "required configuration key "+key+"."+name+" is missing")
			}
			continue
		}
		if err := decodeComposed(target.Field(i), value, key+"."+name); err != nil {
			return err
		}
	}
	return nil
}

func decodeMap(target reflect.Value, raw interface{}, key string) error {
	m, ok := asMap(raw)
	if !ok {
		return notA("map", key)
	}
	t := target.Type()
	out := reflect.MakeMapWithSize(t, len(m))
	for _, name := range sortedKeys(m) {
		elem := reflect.New(t.Elem()).Elem()
		if err := decodeComposed(elem, m[name], key+"."+name); err != nil {
			return err
		}
		out.SetMapIndex(reflect.ValueOf(name).Convert(t.Key()), elem)
	}
	target.Set(out)
	return nil
}

// decodeSlice fills a list. A list of single values may also be written
// as one comma-separated string, as an environment variable carries it.
func decodeSlice(target reflect.Value, raw interface{}, key string) error {
	t := target.Type()
	list, ok := asList(raw)
	if text, isText := raw.(string); isText && bindable(t.Elem()) {
		list, ok = commaList(text), true
	}
	if !ok {
		return notA("list", key)
	}
	out := reflect.MakeSlice(t, len(list), len(list))
	for i, item := range list {
		if err := decodeComposed(out.Index(i), item, key+"["+strconv.Itoa(i)+"]"); err != nil {
			return err
		}
	}
	target.Set(out)
	return nil
}

func commaList(text string) []interface{} {
	parts, _ := convertToStringSlice(text)
	list := make([]interface{}, len(parts))
	for i, p := range parts {
		list[i] = p
	}
	return list
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// flatSubtree rebuilds the subtree of key from a layer written flat, as
// properties and INI files write it ("servers.a.host" for servers:
// {a: {host: ...}}); nil when the layer has no such keys.
func flatSubtree(layer map[string]interface{}, key string) map[string]interface{} {
	var tree map[string]interface{}
	for flat, value := range layer {
		rest, ok := strings.CutPrefix(flat, key+".")
		if !ok || rest == "" {
			continue
		}
		if tree == nil {
			tree = map[string]interface{}{}
		}
		setNested(tree, strings.Split(rest, "."), value)
	}
	return tree
}

// setNested puts value at path in tree, making the maps on the way.
func setNested(tree map[string]interface{}, path []string, value interface{}) {
	for _, part := range path[:len(path)-1] {
		next, ok := tree[part].(map[string]interface{})
		if !ok {
			next = map[string]interface{}{}
			tree[part] = next
		}
		tree = next
	}
	tree[path[len(path)-1]] = value
}

// keyTree is the keys a bound type names: a branch for a nested struct,
// a leaf for a value (a composed one checks its own subtree).
type keyTree struct {
	children map[string]*keyTree
	kind     reflect.Type // a leaf's field type
}

func (k *keyTree) at(key string) *keyTree {
	n := k
	for _, part := range strings.Split(key, ".") {
		if n.children == nil {
			n.children = map[string]*keyTree{}
		}
		next, ok := n.children[part]
		if !ok {
			next = &keyTree{}
			n.children[part] = next
		}
		n = next
	}
	return n
}

func (k *keyTree) branch(key string) { k.at(key) }

func (k *keyTree) leaf(key string, kind reflect.Type) { k.at(key).kind = kind }

// treeOf is the keys struct type t names, as its tags write them.
func treeOf(t reflect.Type) *keyTree {
	tree := &keyTree{}
	var walk func(t reflect.Type, prefix string)
	walk = func(t reflect.Type, prefix string) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name, _, tagged := parseBindTag(f.Tag.Get("argus"))
			if !tagged || !f.IsExported() {
				continue
			}
			if f.Type.Kind() == reflect.Struct && f.Type != durationType {
				tree.branch(join(prefix, name))
				walk(f.Type, join(prefix, name))
				continue
			}
			tree.leaf(join(prefix, name), f.Type)
		}
	}
	walk(t, "")
	return tree
}

// check refuses a key the file or the remote layer holds and the type does
// not name, under the reader's Sub prefix, written nested or flat.
func (k *keyTree) check(r reader) error {
	prefix := strings.TrimSuffix(r.prefix, ".")
	for _, layer := range []map[string]interface{}{r.view.file, r.view.remote} {
		if err := k.checkMap(underPrefix(layer, prefix), prefix); err != nil {
			return err
		}
	}
	return nil
}

// underPrefix is what a layer holds under prefix: its nested map, and its
// flat keys with the prefix taken off.
func underPrefix(layer map[string]interface{}, prefix string) map[string]interface{} {
	if prefix == "" {
		return layer
	}
	out := map[string]interface{}{}
	if tree, found := lookupConfigValue(layer, prefix); found {
		if m, ok := asMap(tree); ok {
			for k, v := range m {
				out[k] = v
			}
		}
	}
	for flat, value := range layer {
		if rest, ok := strings.CutPrefix(flat, prefix+"."); ok && rest != "" {
			out[rest] = value
		}
	}
	return out
}

func join(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// checkMap walks a map against a branch. A dotted key ("server.port", as
// flat formats write them) descends one part at a time; the parts left
// when it reaches a value are that value's own keys.
func (k *keyTree) checkMap(m map[string]interface{}, path string) error {
	for _, key := range sortedKeys(m) {
		n, parts := k, strings.Split(key, ".")
		for len(parts) > 0 && n.kind == nil {
			next, ok := n.children[parts[0]]
			if !ok {
				return errors.New(ErrCodeInvalidConfig, "unknown configuration key "+join(path, key))
			}
			n, parts = next, parts[1:]
		}
		value := m[key]
		if len(parts) > 0 {
			nested := map[string]interface{}{}
			setNested(nested, parts, value)
			value = nested
		}
		if err := n.checkValue(value, join(path, key)); err != nil {
			return err
		}
	}
	return nil
}

// checkValue checks what a key holds: a branch's map, or a leaf's value.
func (k *keyTree) checkValue(raw interface{}, path string) error {
	if k.kind == nil {
		if m, ok := asMap(raw); ok {
			return k.checkMap(m, path)
		}
		return nil // not a map: the bind itself says so
	}
	return checkKeys(k.kind, raw, path)
}

// checkKeys checks the keys of a value decoded into t.
func checkKeys(t reflect.Type, raw interface{}, path string) error {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch {
	case t.Kind() == reflect.Struct && t != durationType:
		if m, ok := asMap(raw); ok {
			return treeOf(t).checkMap(m, path)
		}
	case t.Kind() == reflect.Map:
		m, _ := asMap(raw)
		for _, name := range sortedKeys(m) {
			if err := checkKeys(t.Elem(), m[name], path+"."+name); err != nil {
				return err
			}
		}
	case t.Kind() == reflect.Slice:
		return checkElements(t.Elem(), raw, path)
	}
	return nil
}

func checkElements(t reflect.Type, raw interface{}, path string) error {
	list, _ := asList(raw)
	for i, item := range list {
		if err := checkKeys(t, item, path+"["+strconv.Itoa(i)+"]"); err != nil {
			return err
		}
	}
	return nil
}
