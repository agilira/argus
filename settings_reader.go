// settings_reader.go: reading one revision, one key at a time
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: AGILira fragment
// SPDX-License-Identifier: MPL-2.0
//
// A reader pins the revision it was made from. One Get is one revision by
// construction; several Gets through the same reader — binding a struct,
// explaining an instance — are all the same revision, rather than letting a
// swap land between the third field and the fourth.

package argus

import (
	"strings"
	"time"

	flashflags "github.com/agilira/flash-flags"
)

// reader reads typed values out of one revision.
type reader struct {
	res    *resolver
	view   *snapshot
	flags  *flashflags.FlagSet
	prefix string
}

// reader pins the revision in force.
func (s *Settings) reader() reader {
	return reader{
		res:    s.core.res,
		view:   s.core.res.view(),
		flags:  s.core.flags,
		prefix: s.prefix,
	}
}

// readerOf pins a particular revision, which is how a candidate is bound
// before it is published.
func (s *Settings) readerOf(view *snapshot) reader {
	r := s.reader()
	r.view = view
	return r
}

// key applies the prefix of a Sub view.
func (r reader) key(name string) string {
	if r.prefix == "" {
		return name
	}
	return r.prefix + name
}

// raw returns what a layer supplies for the key, before any conversion.
//
// The typed getters below are lenient by contract: a value that cannot become
// the type asked for reads as the zero value. Binding cannot afford that — a
// port written as "eihgt thousand" would become 0 and the application would
// listen on a port nobody meant — so the binder reads the raw value and
// converts it itself, with the error in hand.
func (r reader) raw(key string) (value interface{}, fromFlag, found bool) {
	got := r.res.resolveIn(r.view, r.key(key))
	return got.value, got.fromFlag, got.found
}

// has reports whether any layer supplies the key.
func (r reader) has(key string) bool {
	return r.res.resolveIn(r.view, r.key(key)).found
}

// getString returns a string value, or "" when no source supplies the key.
func (r reader) getString(key string) string {
	got := r.res.resolveIn(r.view, r.key(key))
	if got.fromFlag || !got.found {
		if r.flags != nil {
			return r.flags.GetString(r.key(key))
		}
		return ""
	}
	return convertToString(got.value)
}

// getInt returns an integer value, or 0 when no source supplies the key.
func (r reader) getInt(key string) int {
	got := r.res.resolveIn(r.view, r.key(key))
	if !got.fromFlag && got.found {
		if converted, err := convertToInt(got.value); err == nil {
			return converted
		}
	}
	if r.flags != nil {
		return r.flags.GetInt(r.key(key))
	}
	return 0
}

// getInt64 returns a 64-bit integer value, or 0.
func (r reader) getInt64(key string) int64 {
	got := r.res.resolveIn(r.view, r.key(key))
	if !got.fromFlag && got.found {
		if converted, err := convertToInt64(got.value); err == nil {
			return converted
		}
	}
	if r.flags != nil {
		return int64(r.flags.GetInt(r.key(key)))
	}
	return 0
}

// getBool returns a boolean value, or false.
func (r reader) getBool(key string) bool {
	got := r.res.resolveIn(r.view, r.key(key))
	if !got.fromFlag && got.found {
		if converted, err := convertToBool(got.value); err == nil {
			return converted
		}
	}
	if r.flags != nil {
		return r.flags.GetBool(r.key(key))
	}
	return false
}

// getDuration returns a duration value, or 0.
func (r reader) getDuration(key string) time.Duration {
	got := r.res.resolveIn(r.view, r.key(key))
	if !got.fromFlag && got.found {
		if converted, err := convertToDuration(got.value); err == nil {
			return converted
		}
	}
	if r.flags != nil {
		return r.flags.GetDuration(r.key(key))
	}
	return 0
}

// getFloat64 returns a floating point value, or 0.
func (r reader) getFloat64(key string) float64 {
	got := r.res.resolveIn(r.view, r.key(key))
	if !got.fromFlag && got.found {
		if converted, err := convertToFloat64(got.value); err == nil {
			return converted
		}
	}
	if r.flags != nil {
		return r.flags.GetFloat64(r.key(key))
	}
	return 0
}

// getStringSlice returns a list value, accepting either a real list or a
// comma-separated string, which is all an environment variable can carry.
func (r reader) getStringSlice(key string) []string {
	got := r.res.resolveIn(r.view, r.key(key))
	if !got.fromFlag && got.found {
		switch typed := got.value.(type) {
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
		}
	}
	if r.flags != nil {
		return r.flags.GetStringSlice(r.key(key))
	}
	return nil
}
