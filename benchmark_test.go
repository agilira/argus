// benchmark_test.go - Argus Benchmark Tests
//
// Copyright (c) 2025 AGILira
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
	parserMutex.Lock()
	originalParsers := customParsers
	customParsers = nil
	parserMutex.Unlock()
	defer func() {
		parserMutex.Lock()
		customParsers = originalParsers
		parserMutex.Unlock()
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
	parserMutex.Lock()
	originalParsers := make([]ConfigParser, len(customParsers))
	copy(originalParsers, customParsers)
	customParsers = nil
	parserMutex.Unlock()
	defer func() {
		parserMutex.Lock()
		customParsers = originalParsers
		parserMutex.Unlock()
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
	parserMutex.Lock()
	originalParsers := make([]ConfigParser, len(customParsers))
	copy(originalParsers, customParsers)
	customParsers = nil
	parserMutex.Unlock()
	defer func() {
		parserMutex.Lock()
		customParsers = originalParsers
		parserMutex.Unlock()
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
	parserMutex.Lock()
	originalParsers := make([]ConfigParser, len(customParsers))
	copy(originalParsers, customParsers)
	parserMutex.Unlock()
	defer func() {
		parserMutex.Lock()
		customParsers = originalParsers
		parserMutex.Unlock()
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clear and re-register to test registration performance
		parserMutex.Lock()
		customParsers = nil
		parserMutex.Unlock()

		RegisterParser(&testParserForBenchmark{})
	}
}

// Benchmark core Watcher operations (moved from other files for consolidation)
func BenchmarkWatcherGetStatOptimized(b *testing.B) {
	tmpFile, err := os.CreateTemp("", "argus_bench_")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	watcher := New(Config{CacheTTL: time.Hour}) // Long TTL for cache hit testing
	defer watcher.Stop()

	// Prime the cache
	watcher.getStat(tmpFile.Name())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		watcher.getStat(tmpFile.Name()) // Should be cache hit
	}
}

// Benchmark cache miss performance
func BenchmarkWatcherGetStatCacheMiss(b *testing.B) {
	tmpFile, err := os.CreateTemp("", "argus_bench_")
	if err != nil {
		b.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	watcher := New(Config{CacheTTL: 0}) // No caching - always miss
	defer watcher.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		watcher.getStat(tmpFile.Name()) // Always cache miss
	}
}

func BenchmarkWatcherPollFiles(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "argus_bench_")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	for i := 0; i < 5; i++ {
		testFile := filepath.Join(tmpDir, fmt.Sprintf("test%d.json", i))
		if err := os.WriteFile(testFile, []byte(`{"test": true}`), 0644); err != nil {
			b.Fatalf("Failed to create test file: %v", err)
		}
	}

	watcher := New(Config{PollInterval: time.Millisecond})
	defer watcher.Stop()

	// Add some files to watch
	files, _ := os.ReadDir(tmpDir)
	for _, file := range files {
		if !file.IsDir() {
			watcher.Watch(tmpDir+"/"+file.Name(), func(event ChangeEvent) {})
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
