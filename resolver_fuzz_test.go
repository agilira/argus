// resolver_fuzz_test.go - fuzzing the key-to-environment-variable mapping
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"strings"
	"testing"
)

// FuzzEnvKeyName: a configuration key comes from a file, a remote provider or
// an application, so it is outside input. Whatever it holds, the variable name
// it maps to must stay inside the caller's prefix — a key must never be able
// to read a variable belonging to some other application, or to the system.
func FuzzEnvKeyName(f *testing.F) {
	f.Add("APP_", "port")
	f.Add("APP", "server.port")
	f.Add("", "log-level")
	f.Add("app_", "DATABASE_URL")
	f.Add("APP_", "")
	f.Add("APP_", "../../etc/passwd")
	f.Add("APP_", "PATH")
	f.Add("APP_", "a\x00b")

	f.Fuzz(func(t *testing.T, prefix, key string) {
		got := envKeyName(prefix, key)

		// The prefix is a namespace, not a suggestion.
		if prefix != "" {
			want := strings.ToUpper(prefix)
			if !strings.HasSuffix(want, "_") {
				want += "_"
			}
			if !strings.HasPrefix(got, want) {
				t.Fatalf("envKeyName(%q, %q) = %q, which escapes the prefix %q", prefix, key, got, want)
			}
		}

		// Separators are translated, never carried through: a name holding a
		// dot or a dash could not be set from a shell anyway, so a key that
		// kept one would be silently unreachable.
		body := got[len(got)-len(strings.ToUpper(envSeparators.Replace(key))):]
		for _, bad := range []string{".", "-", " "} {
			if strings.Contains(body, bad) {
				t.Fatalf("envKeyName(%q, %q) = %q still holds %q", prefix, key, got, bad)
			}
		}

		// Same input, same name: the mapping is a pure function, or a reload
		// would resolve the same key from a different variable.
		if again := envKeyName(prefix, key); again != got {
			t.Fatalf("envKeyName(%q, %q) is not deterministic: %q then %q", prefix, key, got, again)
		}
	})
}
