// settings_bench_test.go - what a quiet cycle and a read actually cost
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"path/filepath"
	"strconv"
	"testing"
)

// benchSources builds a temporary application with n documents beside its
// configuration file.
func benchSources(b *testing.B, n int) *configSources {
	b.Helper()
	dir := b.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := writeBenchFile(config, `{"port": 8080}`); err != nil {
		b.Fatalf("write: %v", err)
	}
	prompts := filepath.Join(dir, "prompts")
	for i := 0; i < n; i++ {
		path := filepath.Join(prompts, "doc"+strconv.Itoa(i)+".md")
		if err := writeBenchFile(path, "content "+strconv.Itoa(i)); err != nil {
			b.Fatalf("write: %v", err)
		}
	}

	return &configSources{
		files: []fileSource{{path: config}},
		docs: &documentStore{
			sources: []documentSource{{group: "prompts", pattern: filepath.Join(prompts, "*.md")}},
			limits:  defaultDocumentLimits(),
		},
	}
}

// BenchmarkFingerprint is the cheap half of a cycle: what runs at every poll
// when nothing has changed.
func BenchmarkFingerprint(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(strconv.Itoa(n)+"-documents", func(b *testing.B) {
			sources := benchSources(b, n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = sources.fingerprint()
			}
		})
	}
}

// BenchmarkSettingsGetString is the hot path: an application reading a value
// while the configuration reloads underneath it.
func BenchmarkSettingsGetString(b *testing.B) {
	dir := b.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := writeBenchFile(config, `{"model": "claude-opus-5", "nested": {"key": "value"}}`); err != nil {
		b.Fatalf("write: %v", err)
	}

	settings, err := Setup("bench").File(config).Start()
	if err != nil {
		b.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	b.Run("flat", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = settings.GetString("model")
		}
	})
	b.Run("nested", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = settings.GetString("nested.key")
		}
	})
	b.Run("document", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = settings.Doc("prompts", "system")
		}
	})
}

// BenchmarkBind separates the two costs: reading a bound value, which happens
// per request, and building one, which happens per revision.
func BenchmarkBind(b *testing.B) {
	dir := b.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := writeBenchFile(config, agentJSON); err != nil {
		b.Fatalf("write: %v", err)
	}

	settings, err := Setup("bench").File(config).Start()
	if err != nil {
		b.Fatalf("Start: %v", err)
	}
	defer func() { _ = settings.Close() }()

	bound, err := Bind[agentConfig](settings)
	if err != nil {
		b.Fatalf("Bind: %v", err)
	}

	b.Run("Value", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = bound.Value().Model
		}
	})

	b.Run("rebuild-per-revision", func(b *testing.B) {
		b.ReportAllocs()
		view := settings.core.res.view()
		for i := 0; i < b.N; i++ {
			if _, err := bound.build(settings.readerOf(view)); err != nil {
				b.Fatalf("build: %v", err)
			}
		}
	})
}
