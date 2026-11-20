// settings_bind.go: binding a revision onto a struct
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// The rule this file exists for: a reload delivers a new value. Argus never
// writes into a struct the application is already reading — that is a data
// race inside user code, and no amount of locking on our side removes it.
// Value() hands back an immutable snapshot of one revision; the next revision
// is a different allocation.
//
// The field plan is built once, from the type, at Bind. Filling a value then
// costs one reflect.Set per field, once per revision. ConfigBinder's
// unsafe.Pointer machinery buys ~100 ns per field, which matters on a path
// that runs per read and not on one that runs per reload; what matters here is
// that the offsets cannot be wrong.

package argus

import (
	"reflect"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agilira/go-errors"
)

// durationType is time.Duration, which is an int64 with a meaning.
var durationType = reflect.TypeOf(time.Duration(0))

// stringSliceType is []string.
var stringSliceType = reflect.TypeOf([]string(nil))

// boundField is one struct field and the key it reads.
type boundField struct {
	index    []int // the reflect path to the field, nested structs included
	key      string
	path     string // the Go field path, for error messages
	kind     reflect.Type
	required bool
	composed bool // a map, a list or a pointer: decoded from the whole subtree
}

// bindPlan is everything Bind works out once, from the type alone.
type bindPlan struct {
	fields []boundField
	keys   *keyTree // what T names, for Strict
	strict bool
}

// Bound is a struct kept in step with the configuration.
//
// Value returns the struct for the revision in force. Each revision is a new
// allocation, so a value already handed out never changes underneath its
// reader.
type Bound[T any] struct {
	settings *Settings
	plan     *bindPlan

	current atomic.Pointer[boundRevision[T]]
	pending *T // built while a candidate is being validated, published on swap
}

// boundRevision ties a value to the revision it came from, so that Value and
// Revision cannot disagree.
type boundRevision[T any] struct {
	value    *T
	revision uint64
}

// Bind maps the configuration onto the struct type T and keeps it in step.
//
//	type Config struct {
//	    Model   string        `argus:"model,required"`
//	    Timeout time.Duration `argus:"timeout"`
//	    Server  struct {
//	        Port int `argus:"port"`
//	    } `argus:"server"`
//	}
//
//	bound, err := argus.Bind[Config](settings)
//	cfg := bound.Value()
//
// Only tagged fields are bound; everything else is left as it is. A nested
// struct is read under its own tag, so Server.Port above reads "server.port".
// Supported field types are string, int, int64, bool, float64, time.Duration,
// structs, and maps with string keys, lists and pointers of any of those,
// nested as deep as the configuration goes:
//
//	type Config struct {
//	    Servers map[string]Server `argus:"servers"`
//	    Ports   []int             `argus:"ports"`
//	    Tools   *[]string         `argus:"tools"` // nil when absent, empty when []
//	}
//
// A map, a list or a pointer is read whole from the file or remote layer,
// nested or written flat ("servers.files.command=...", as properties and INI
// files write it). The environment may give a list of single values as one
// comma-separated string, and a flag may set a pointer to a single value. A
// key absent leaves it nil. A null too, except where YAML reads it as {}, as
// it always has: a map or a pointer to a struct set to null is an empty one.
// Inside one, the struct tags mean what they mean at the top, dotted and
// required included, and errors name the key down to the element:
// "servers.files.timeout", "owners[0].turns".
//
// With Strict, a key the configuration holds and T does not name is an
// error, so that a typo is refused rather than ignored. When *T has a
// Validate() error method, it is called on every value built; its error
// refuses the revision like any other.
//
// From the moment a type is bound, a revision that does not satisfy it is not
// published: the candidate is refused, the error handler is told, and the last
// good value keeps serving.
func Bind[T any](s *Settings, opts ...BindOption) (*Bound[T], error) {
	var zero T

	plan, err := newBindPlan(reflect.TypeOf(zero))
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt(plan)
	}

	bound := &Bound[T]{settings: s, plan: plan}

	// The cycle is held still for these three steps: read the revision in
	// force, build the value, register for the next one. A swap landing in
	// between would leave the value a revision behind with nothing to correct
	// it.
	err = s.core.reloader.pause(func() error {
		view := s.core.res.view()
		value, err := bound.build(s.readerOf(view))
		if err != nil {
			return err
		}
		bound.current.Store(&boundRevision[T]{value: value, revision: view.revision})
		s.core.addBinder(bound)

		return nil
	})
	if err != nil {
		return nil, err
	}

	return bound, nil
}

// Value returns the configuration as of the revision in force.
func (b *Bound[T]) Value() *T { return b.current.Load().value }

// Revision returns the revision the current value came from.
func (b *Bound[T]) Revision() uint64 { return b.current.Load().revision }

// build fills a new value from one revision.
func (b *Bound[T]) build(r reader) (*T, error) {
	value := new(T)
	target := reflect.ValueOf(value).Elem()

	for _, field := range b.plan.fields {
		if field.required && !r.has(field.key) {
			return nil, errors.New(ErrCodeInvalidConfig,
				"required configuration key "+field.key+" is missing, for field "+field.path)
		}
		if err := setBoundField(target.FieldByIndex(field.index), field, r); err != nil {
			return nil, err
		}
	}
	if b.plan.strict {
		if err := b.plan.keys.check(r); err != nil {
			return nil, err
		}
	}
	if v, ok := any(value).(Validator); ok {
		if err := v.Validate(); err != nil {
			return nil, err
		}
	}

	return value, nil
}

// prepare builds the value a candidate revision would produce. An error here
// refuses the candidate.
func (b *Bound[T]) prepare(view *snapshot) error {
	value, err := b.build(b.settings.readerOf(view))
	if err != nil {
		return err
	}
	b.pending = value

	return nil
}

// commit publishes the value prepared for a candidate that was accepted.
func (b *Bound[T]) commit(revision uint64) {
	if b.pending == nil {
		return
	}
	b.current.Store(&boundRevision[T]{value: b.pending, revision: revision})
	b.pending = nil
}

// discard drops a value prepared for a candidate that was refused.
func (b *Bound[T]) discard() { b.pending = nil }

// setBoundField writes one value into the struct being built, or says why the
// value a source supplies cannot go there.
//
// A key nobody supplies leaves the field at its zero value; whether that is
// acceptable is what the required option answers. A key somebody supplies
// badly is an error, which is the difference between this and the getters.
func setBoundField(target reflect.Value, field boundField, r reader) error {
	value, fromFlag, found := r.raw(field.key)
	if field.composed {
		return setComposed(target, field, r, value, fromFlag, found)
	}
	if !found {
		return nil
	}
	if fromFlag {
		// A flag already holds its value in the type it was declared with.
		setFieldFromFlags(target, field, r)
		return nil
	}
	if err := convertInto(target, value); err != nil {
		return bindError(field, value, err)
	}

	return nil
}

// setComposed writes a map, a list or a pointer: from the subtree a layer
// holds, or rebuilt from its flat keys; a pointer to a single value also
// from a flag.
func setComposed(target reflect.Value, field boundField, r reader, value interface{}, fromFlag, found bool) error {
	if fromFlag {
		if field.kind.Kind() != reflect.Pointer || !bindable(field.kind.Elem()) {
			return errors.New(ErrCodeInvalidConfig, "argus: a flag cannot set "+field.key+" ("+field.kind.String()+")")
		}
		elem := reflect.New(field.kind.Elem())
		setFieldFromFlags(elem.Elem(), boundField{key: field.key, kind: field.kind.Elem()}, r)
		target.Set(elem)
		return nil
	}
	if !found {
		for _, layer := range []map[string]interface{}{r.view.file, r.view.remote} {
			if tree := flatSubtree(layer, r.key(field.key)); tree != nil {
				return decodeComposed(target, tree, field.key)
			}
		}
		return nil
	}
	return decodeComposed(target, value, field.key)
}

// convertInto writes a single value into a field of a scalar type, or a
// []string.
func convertInto(target reflect.Value, value interface{}) error {
	kind := target.Type()
	switch {
	case kind == durationType:
		converted, err := convertToDuration(value)
		if err == nil {
			target.SetInt(int64(converted))
		}
		return err
	case kind == stringSliceType:
		converted, err := convertToStringSlice(value)
		if err == nil {
			target.Set(reflect.ValueOf(converted))
		}
		return err
	case kind.Kind() == reflect.String:
		target.SetString(convertToString(value))
		return nil
	}
	return convertNumber(target, value)
}

// convertNumber writes an int, int64, bool or float64.
func convertNumber(target reflect.Value, value interface{}) error {
	switch target.Kind() {
	case reflect.Int:
		converted, err := convertToInt(value)
		target.SetInt(int64(converted))
		return err
	case reflect.Int64:
		converted, err := convertToInt64(value)
		target.SetInt(converted)
		return err
	case reflect.Bool:
		converted, err := convertToBool(value)
		target.SetBool(converted)
		return err
	case reflect.Float64:
		converted, err := convertToFloat64(value)
		target.SetFloat(converted)
		return err
	default:
		// Unreachable: newBindPlan refuses a field bindable does not know.
		// Here so that adding a type to one and not the other is an error
		// rather than a field that silently stays zero.
		return errors.New(ErrCodeInvalidConfig, "no binding for type "+target.Type().String())
	}
}

// setFieldFromFlags reads a value the flag layer supplies, in the type the
// flag was declared with.
func setFieldFromFlags(target reflect.Value, field boundField, r reader) {
	switch {
	case field.kind == durationType:
		target.SetInt(int64(r.getDuration(field.key)))
	case field.kind == stringSliceType:
		target.Set(reflect.ValueOf(r.getStringSlice(field.key)))
	case field.kind.Kind() == reflect.String:
		target.SetString(r.getString(field.key))
	case field.kind.Kind() == reflect.Int:
		target.SetInt(int64(r.getInt(field.key)))
	case field.kind.Kind() == reflect.Int64:
		target.SetInt(r.getInt64(field.key))
	case field.kind.Kind() == reflect.Bool:
		target.SetBool(r.getBool(field.key))
	case field.kind.Kind() == reflect.Float64:
		target.SetFloat(r.getFloat64(field.key))
	}
}

// convertToStringSlice accepts a list, or the comma-separated string an
// environment variable has to carry one as.
func convertToStringSlice(value interface{}) ([]string, error) {
	switch typed := value.(type) {
	case []string:
		return typed, nil
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			result = append(result, convertToString(item))
		}
		return result, nil
	case string:
		if typed == "" {
			return nil, nil
		}
		parts := strings.Split(typed, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts, nil
	default:
		return nil, errors.New(ErrCodeInvalidConfig, "not a list")
	}
}

// bindError says which field, which key and which value did not go together.
func bindError(field boundField, value interface{}, cause error) error {
	return errors.Wrap(cause, ErrCodeInvalidConfig,
		"cannot bind "+field.key+" to field "+field.path+
			" ("+field.kind.String()+"): value "+convertToString(value))
}

// newBindPlan works out, from the type alone, which fields are bound and to
// which keys. Everything that can be wrong about a binding is wrong here,
// before the application is running.
func newBindPlan(structType reflect.Type) (*bindPlan, error) {
	if structType == nil || structType.Kind() != reflect.Struct {
		return nil, errors.New(ErrCodeInvalidConfig,
			"argus: only a struct type can be bound")
	}

	plan := &bindPlan{keys: &keyTree{}}
	if err := plan.walk(structType, "", nil, ""); err != nil {
		return nil, err
	}
	if err := checkComposable(structType, map[reflect.Type]bool{}); err != nil {
		return nil, err
	}

	return plan, nil
}

// walk descends one struct type, carrying the key prefix and the field path.
func (p *bindPlan) walk(structType reflect.Type, keyPrefix string, index []int, path string) error {
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		key, required, tagged := parseBindTag(field.Tag.Get("argus"))
		if !tagged || !field.IsExported() {
			continue
		}
		f := boundField{
			index:    append(append([]int{}, index...), i),
			key:      join(keyPrefix, key),
			path:     join(path, field.Name),
			kind:     field.Type,
			required: required,
		}
		if err := p.add(f); err != nil {
			return err
		}
	}

	return nil
}

// add plans one field: a nested struct is walked, anything else is bound.
func (p *bindPlan) add(f boundField) error {
	if f.kind.Kind() == reflect.Struct && f.kind != durationType {
		p.keys.branch(f.key)
		return p.walk(f.kind, f.key, f.index, f.path)
	}
	f.composed = composite(f.kind)
	if !f.composed && !bindable(f.kind) {
		return errors.New(ErrCodeInvalidConfig,
			"argus: field "+f.path+" has no binding for type "+f.kind.String())
	}
	p.keys.leaf(f.key, f.kind)
	p.fields = append(p.fields, f)
	return nil
}

// parseBindTag reads an argus struct tag: the key, and whether the application
// says it cannot run without it.
func parseBindTag(tag string) (key string, required, tagged bool) {
	if tag == "" || tag == "-" {
		return "", false, false
	}

	parts := strings.Split(tag, ",")
	key = strings.TrimSpace(parts[0])
	if key == "" {
		return "", false, false
	}
	for _, option := range parts[1:] {
		if strings.TrimSpace(option) == "required" {
			required = true
		}
	}

	return key, required, true
}

// bindable reports whether a field type can be read from a configuration
// value.
func bindable(fieldType reflect.Type) bool {
	if fieldType == durationType || fieldType == stringSliceType {
		return true
	}

	switch fieldType.Kind() {
	case reflect.String, reflect.Int, reflect.Int64, reflect.Bool, reflect.Float64:
		return true
	default:
		return false
	}
}
