// benchmark_test.go - Argus Benchmark Tests
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Benchmark for the hyper-optimized DetectFormat function
func BenchmarkDetectFormatOptimized(b *testing.B) {
	testFiles := []string{
		"config.json",            // Common case
		"app.yml",                // 3-char extension
		"docker-compose.yaml",    // 4-char extension
		"Cargo.toml",             // Different format
		"terraform.hcl",          // HCL format
		"main.tf",                // Short HCL
		"app.ini",                // INI format
		"system.conf",            // CONF format
		"server.cfg",             // CFG format
		"application.properties", // Long extension
		"service.config",         // CONFIG format
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, file := range testFiles {
			DetectFormat(file)
		}
	}
}

// Benchmark single file format detection (most common case)
func BenchmarkDetectFormatSingleOptimized(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectFormat("config.json") // Most common case
	}
}

// Benchmark ParseConfig without custom parsers (built-in only) - OPTIMIZED
func BenchmarkParseConfigBuiltinOnlyOptimized(b *testing.B) {
	// Test different JSON sizes to verify scalability
	testCases := []struct {
		name string
		data []byte
	}{
		{"small", []byte(`{"service": "test", "port": 8080, "enabled": true}`)},
		{"medium", []byte(`{"service": "test", "port": 8080, "enabled": true, "database": {"host": "localhost", "port": 5432, "name": "testdb"}, "features": ["auth", "logging", "metrics"]}`)},
		{"large", []byte(`{"service": "test", "port": 8080, "enabled": true, "database": {"host": "localhost", "port": 5432, "name": "testdb", "pool": {"min": 5, "max": 100}}, "features": ["auth", "logging", "metrics", "tracing"], "config": {"timeout": 30, "retries": 3, "backoff": 1.5}, "servers": [{"name": "server1", "host": "10.0.0.1"}, {"name": "server2", "host": "10.0.0.2"}]}`)},
	}

	// Ensure no custom parsers are registered
	originalParsers := snapshotParsers()
	restoreParsers(nil)
	defer func() {
		restoreParsers(originalParsers)
	}()

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ParseConfig(tc.data, FormatJSON)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Benchmark ParseConfig with custom parser registered (but not used)
func BenchmarkParseConfigWithCustomParser(b *testing.B) {
	jsonContent := []byte(`{"service": "test", "port": 8080, "enabled": true}`)

	// Save original state
	originalParsers := snapshotParsers()
	restoreParsers(nil) // Clear for clean test
	defer func() {
		restoreParsers(originalParsers)
	}()

	// Register a custom YAML parser (won't be used for JSON)
	testParser := &testParserForBenchmark{}
	RegisterParser(testParser)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseConfig(jsonContent, FormatJSON)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark ParseConfig with custom parser being used
func BenchmarkParseConfigCustomParserUsed(b *testing.B) {
	yamlContent := []byte(`service: test
port: 8080
enabled: true`)

	// Save original state
	originalParsers := snapshotParsers()
	restoreParsers(nil) // Clear for clean test
	defer func() {
		restoreParsers(originalParsers)
	}()

	// Register a custom YAML parser
	testParser := &testParserForBenchmark{}
	RegisterParser(testParser)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseConfig(yamlContent, FormatYAML)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark parser registration (thread safety overhead)
func BenchmarkParserRegistration(b *testing.B) {
	// Save original state
	originalParsers := snapshotParsers()
	defer func() {
		restoreParsers(originalParsers)
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clear and re-register to test registration performance
		restoreParsers(nil)

		RegisterParser(&testParserForBenchmark{})
	}
}

// Benchmark core Watcher operations (moved from other files for consolidation)
func BenchmarkWatcherGetStatOptimized(b *testing.B) {
	tmpFile, err := os.CreateTemp("", "argus_bench_")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			b.Errorf("Failed to remove tmpFile: %v", err)
		}
	}()
	if err := tmpFile.Close(); err != nil {
		b.Errorf("Failed to close tmpFile: %v", err)
	}

	watcher := New(Config{CacheTTL: time.Hour}) // Long TTL for cache hit testing
	if err := watcher.Start(); err != nil {
		b.Fatalf("Failed to start watcher: %v", err)
	}
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Errorf("Failed to stop watcher: %v", err)
		}
	}()

	// Prime the cache
	if _, err := watcher.getStat(tmpFile.Name()); err != nil {
		b.Logf("Failed to get stat: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := watcher.getStat(tmpFile.Name()); err != nil {
			b.Logf("Failed to get stat: %v", err)
		} // Should be cache hit
	}
}

// BenchmarkWatcherStatFresh measures an uncached stat: os.Stat plus the cache
// write. This is what a poll cycle does for every watched file.
//
// Note that a short CacheTTL would not produce this through getStat: cache
// ages are measured with go-timecache, whose clock does not advance inside a
// tight loop, so every lookup there reads as fresh.
func BenchmarkWatcherStatFresh(b *testing.B) {
	tmpFile, err := os.CreateTemp("", "argus_bench_")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			b.Errorf("Failed to remove tmpFile: %v", err)
		}
	}()
	if err := tmpFile.Close(); err != nil {
		b.Errorf("Failed to close tmpFile: %v", err)
	}

	watcher := New(Config{})
	if err := watcher.Start(); err != nil {
		b.Fatalf("Failed to start watcher: %v", err)
	}
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Errorf("Failed to stop watcher: %v", err)
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := watcher.statFresh(tmpFile.Name()); err != nil {
			b.Logf("Failed to get stat: %v", err)
		}
	}
}

func BenchmarkWatcherPollFiles(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "argus_bench_")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			b.Errorf("Failed to remove tmpDir: %v", err)
		}
	}()

	// Create test files
	for i := 0; i < 5; i++ {
		testFile := filepath.Join(tmpDir, fmt.Sprintf("test%d.json", i))
		if err := os.WriteFile(testFile, []byte(`{"test": true}`), 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}

	watcher := New(Config{PollInterval: time.Millisecond})
	if err := watcher.Start(); err != nil {
		b.Fatalf("Failed to start watcher: %v", err)
	}
	defer func() {
		if err := watcher.Stop(); err != nil {
			b.Errorf("Failed to stop watcher: %v", err)
		}
	}()

	// Add some files to watch
	files, _ := os.ReadDir(tmpDir)
	for _, file := range files {
		if !file.IsDir() {
			if err := watcher.Watch(tmpDir+"/"+file.Name(), func(event ChangeEvent) {}); err != nil {
				b.Logf("Failed to watch file: %v", err)
			}
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		watcher.pollFiles()
	}
}

// Test parser for benchmarks
type testParserForBenchmark struct{}

func (p *testParserForBenchmark) Parse(data []byte) (map[string]interface{}, error) {
	// Simple fast parser for benchmarking
	return map[string]interface{}{
		"benchmark": "test",
		"data_size": len(data),
	}, nil
}

func (p *testParserForBenchmark) Supports(format ConfigFormat) bool {
	return format == FormatYAML
}

func (p *testParserForBenchmark) Name() string {
	return "Benchmark Test Parser"
}

// benchFiles creates n config files in a temporary directory.
func benchFiles(b *testing.B, n int) []string {
	b.Helper()
	dir := b.TempDir()
	paths := make([]string, n)
	for i := range paths {
		p := filepath.Join(dir, fmt.Sprintf("config_%05d.json", i))
		if err := os.WriteFile(p, []byte(`{"a":1}`), 0o600); err != nil {
			b.Fatalf("WriteFile: %v", err)
		}
		paths[i] = p
	}
	return paths
}

// BenchmarkWatcherSetup measures what registering a file costs: Watch
// validates the path, stats it once and stores it. Reported per file.
func BenchmarkWatcherSetup(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("files=%d", n), func(b *testing.B) {
			paths := benchFiles(b, n)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				w := New(Config{
					PollInterval:    time.Hour,
					MaxWatchedFiles: n + 10,
					DisableAudit:    true,
				})
				b.StartTimer()

				for _, p := range paths {
					if err := w.Watch(p, func(ChangeEvent) {}); err != nil {
						b.Fatalf("Watch: %v", err)
					}
				}

				b.StopTimer()
				_ = w.Close()
				b.StartTimer()
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/file")
		})
	}
}

// BenchmarkWatcherPollCycle measures one complete poll of n watched files:
// one os.Stat each, spread over the worker pool. Reported per file.
func BenchmarkWatcherPollCycle(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("files=%d", n), func(b *testing.B) {
			paths := benchFiles(b, n)
			w := New(Config{
				PollInterval:    time.Hour,
				MaxWatchedFiles: n + 10,
				DisableAudit:    true,
			})
			defer func() { _ = w.Close() }()
			for _, p := range paths {
				if err := w.Watch(p, func(ChangeEvent) {}); err != nil {
					b.Fatalf("Watch: %v", err)
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w.pollFiles()
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*n), "ns/file")
		})
	}
}

// BenchmarkParseConfigByFormat parses one equivalent document per supported
// format with the built-in parsers, so the per-format cost is comparable.
func BenchmarkParseConfigByFormat(b *testing.B) {
	cases := []struct {
		name   string
		format ConfigFormat
		data   []byte
	}{
		{"JSON", FormatJSON, []byte(`{"service":"api","port":8080,"debug":true,"database":{"host":"localhost","port":5432}}`)},
		{"YAML", FormatYAML, []byte("service: api\nport: 8080\ndebug: true\ndatabase:\n  host: localhost\n  port: 5432\n")},
		{"TOML", FormatTOML, []byte("service = \"api\"\nport = 8080\ndebug = true\n\n[database]\nhost = \"localhost\"\nport = 5432\n")},
		{"HCL", FormatHCL, []byte("service = \"api\"\nport = 8080\ndebug = true\n\ndatabase {\n  host = \"localhost\"\n  port = 5432\n}\n")},
		{"INI", FormatINI, []byte("service = api\nport = 8080\ndebug = true\n\n[database]\nhost = localhost\nport = 5432\n")},
		{"Properties", FormatProperties, []byte("service=api\nport=8080\ndebug=true\ndatabase.host=localhost\ndatabase.port=5432\n")},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			if _, err := ParseConfig(tc.data, tc.format); err != nil {
				b.Fatalf("ParseConfig(%s): %v", tc.name, err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := ParseConfig(tc.data, tc.format); err != nil {
					b.Fatalf("ParseConfig(%s): %v", tc.name, err)
				}
			}
		})
	}
}
