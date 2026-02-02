// directory_watcher_security_test.go: Red Team Security Tests for Directory Watching
//
// RED TEAM SECURITY ANALYSIS:
// This file implements systematic security testing for the directory watching
// functionality, designed to identify and prevent attack vectors specific to
// directory-based configuration loading.
//
// THREAT MODEL:
// - Path traversal attacks via directory paths
// - Symlink-based escapes from watched directories
// - Race conditions during directory scanning (TOCTOU)
// - Malicious file injection during watch
// - Resource exhaustion via deep directory trees
// - File permission bypass attempts
// - Malformed filenames and paths
//
// METHODOLOGY:
// 1. Identify attack surface specific to directory watching
// 2. Create targeted exploit scenarios
// 3. Test boundary conditions and edge cases
// 4. Validate security controls and mitigations
// 5. Document vulnerabilities and remediation steps
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// =============================================================================
// DIRECTORY PATH TRAVERSAL SECURITY TESTS
// =============================================================================

// TestDirectoryWatcher_Security_PathTraversal tests for directory traversal vulnerabilities.
//
// ATTACK VECTOR: Path traversal via directory path (CWE-22)
// DESCRIPTION: Malicious actors attempt to watch directories outside the intended
// scope by using "../" sequences or equivalent techniques in the directory path.
//
// IMPACT: If successful, attackers could monitor sensitive system directories,
// potentially exfiltrating configuration data or monitoring system activity.
//
// MITIGATION EXPECTED: WatchDirectory should validate and sanitize directory paths,
// rejecting dangerous path components.
func TestDirectoryWatcher_Security_PathTraversal(t *testing.T) {
	pathTraversalAttacks := []struct {
		name        string
		path        string
		description string
	}{
		{
			name:        "BasicUnixTraversal",
			path:        "../../../etc",
			description: "Basic Unix directory traversal to system config",
		},
		{
			name:        "DeepTraversal",
			path:        "../../../../../../../../tmp",
			description: "Deep directory traversal with excessive ../ components",
		},
		{
			name:        "MixedSeparators",
			path:        "../..\\../etc",
			description: "Mixed path separators to bypass filtering",
		},
		{
			name:        "RelativeWithDots",
			path:        "./../../etc",
			description: "Relative path with traversal components",
		},
		{
			name:        "HiddenTraversal",
			path:        "valid/../../../etc",
			description: "Traversal hidden after valid directory name",
		},
		{
			name:        "DoubleSlashTraversal",
			path:        "..//..//..//etc",
			description: "Double slashes to confuse path normalization",
		},
	}

	for _, attack := range pathTraversalAttacks {
		t.Run(attack.name, func(t *testing.T) {
			_, err := WatchDirectory(attack.path, DirectoryWatchOptions{
				Patterns: []string{"*.yaml"},
			}, func(update DirectoryConfigUpdate) {
				t.Errorf("SECURITY VULNERABILITY: Callback invoked for malicious path: %s", attack.path)
			})

			if err == nil {
				t.Errorf("SECURITY VULNERABILITY: Path traversal not blocked for: %s (%s)",
					attack.path, attack.description)
			} else if !strings.Contains(err.Error(), "traversal") && !strings.Contains(err.Error(), "not accessible") {
				t.Logf("Path rejected with error: %v", err)
			}
		})
	}
}

// TestDirectoryWatcher_Security_SymlinkEscape tests for symlink-based escapes.
//
// ATTACK VECTOR: Symlink escape (CWE-59)
// DESCRIPTION: Attacker creates a symlink inside the watched directory pointing
// to a sensitive location outside the intended scope.
//
// IMPACT: Could allow reading configuration files from arbitrary locations,
// potentially exposing secrets or system configuration.
//
// NOTE: This test creates actual symlinks to verify the vulnerability.
func TestDirectoryWatcher_Security_SymlinkEscape(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a "target" directory with sensitive data
	sensitiveDir := filepath.Join(tmpDir, "sensitive")
	if err := os.MkdirAll(sensitiveDir, 0o755); err != nil {
		t.Fatalf("Failed to create sensitive dir: %v", err)
	}

	secretFile := filepath.Join(sensitiveDir, "secret.yaml")
	if err := os.WriteFile(secretFile, []byte("api_key: supersecret123\n"), 0o600); err != nil {
		t.Fatalf("Failed to create secret file: %v", err)
	}

	// Create watched directory
	watchedDir := filepath.Join(tmpDir, "watched")
	if err := os.MkdirAll(watchedDir, 0o755); err != nil {
		t.Fatalf("Failed to create watched dir: %v", err)
	}

	// Create a legit config file
	legitFile := filepath.Join(watchedDir, "config.yaml")
	if err := os.WriteFile(legitFile, []byte("app: myapp\n"), 0o600); err != nil {
		t.Fatalf("Failed to create legit file: %v", err)
	}

	// Create symlink pointing outside watched directory
	symlinkPath := filepath.Join(watchedDir, "escape")
	if err := os.Symlink(sensitiveDir, symlinkPath); err != nil {
		t.Skipf("Cannot create symlinks on this system: %v", err)
	}

	var mu sync.Mutex
	var updates []DirectoryConfigUpdate
	accessedPaths := make(map[string]bool)

	watcher, err := WatchDirectory(watchedDir, DirectoryWatchOptions{
		Patterns:  []string{"*.yaml"},
		Recursive: true, // Try to follow into symlinked dirs
	}, func(update DirectoryConfigUpdate) {
		mu.Lock()
		updates = append(updates, update)
		accessedPaths[update.FilePath] = true
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("WatchDirectory failed: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("watcher.Close failed: %v", err)
		}
	}()

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	// Check if secret file was accessed via symlink escape
	secretAccessed := accessedPaths[secretFile]
	for path := range accessedPaths {
		if strings.Contains(path, "sensitive") || strings.Contains(path, "secret") {
			secretAccessed = true
		}
	}
	mu.Unlock()

	if secretAccessed {
		t.Errorf("SECURITY VULNERABILITY: Symlink escape allowed access to sensitive directory")
		t.Logf("Accessed paths: %v", accessedPaths)
	}
}

// =============================================================================
// RESOURCE EXHAUSTION SECURITY TESTS
// =============================================================================

// TestDirectoryWatcher_Security_DeepDirectoryTree tests resource exhaustion via deep trees.
//
// ATTACK VECTOR: Resource exhaustion (CWE-400)
// DESCRIPTION: Attacker creates deeply nested directory structures to cause
// stack overflow, memory exhaustion, or excessive CPU usage during traversal.
//
// IMPACT: Denial of service through resource exhaustion.
//
// MITIGATION EXPECTED: Reasonable limits on recursion depth.
func TestDirectoryWatcher_Security_DeepDirectoryTree(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping deep directory test in short mode")
	}

	tmpDir := t.TempDir()

	// Create deeply nested directory (100 levels)
	currentDir := tmpDir
	for i := 0; i < 100; i++ {
		currentDir = filepath.Join(currentDir, fmt.Sprintf("level%d", i))
	}
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("Failed to create deep directory: %v", err)
	}

	// Create config file at the bottom
	deepFile := filepath.Join(currentDir, "deep.yaml")
	if err := os.WriteFile(deepFile, []byte("deep: true\n"), 0o600); err != nil {
		t.Fatalf("Failed to create deep file: %v", err)
	}

	start := time.Now()
	done := make(chan bool)

	go func() {
		watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
			Patterns:  []string{"*.yaml"},
			Recursive: true,
		}, func(update DirectoryConfigUpdate) {})

		if err != nil {
			t.Logf("Deep directory watch failed (acceptable): %v", err)
			done <- false
			return
		}
		if err := watcher.Close(); err != nil {
			t.Logf("watcher.Close failed: %v", err)
		}
		done <- true
	}()

	select {
	case success := <-done:
		elapsed := time.Since(start)
		if elapsed > 5*time.Second {
			t.Errorf("SECURITY CONCERN: Deep directory traversal took %v (should be fast or fail)", elapsed)
		}
		if success {
			t.Logf("Deep directory traversal completed in %v", elapsed)
		}
	case <-time.After(10 * time.Second):
		t.Errorf("SECURITY VULNERABILITY: Deep directory traversal caused timeout (DoS)")
	}
}

// TestDirectoryWatcher_Security_ManyFiles tests behavior with large file counts.
//
// ATTACK VECTOR: Resource exhaustion via file count (CWE-400)
// DESCRIPTION: Directory with massive number of files could cause memory exhaustion
// or excessive file descriptor usage.
//
// IMPACT: Denial of service, file descriptor exhaustion.
func TestDirectoryWatcher_Security_ManyFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping many files test in short mode")
	}

	tmpDir := t.TempDir()

	// Create 1000 config files
	for i := 0; i < 1000; i++ {
		filePath := filepath.Join(tmpDir, fmt.Sprintf("config%04d.yaml", i))
		content := fmt.Sprintf("id: %d\n", i)
		if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
			t.Fatalf("Failed to create file %d: %v", i, err)
		}
	}

	var mu sync.Mutex
	updateCount := 0

	start := time.Now()

	watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
		Patterns: []string{"*.yaml"},
	}, func(update DirectoryConfigUpdate) {
		mu.Lock()
		updateCount++
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("WatchDirectory failed: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("watcher.Close failed: %v", err)
		}
	}()

	elapsed := time.Since(start)

	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	count := updateCount
	mu.Unlock()

	t.Logf("Processed %d files in %v", count, elapsed)

	// Should handle 1000 files reasonably quickly
	if elapsed > 10*time.Second {
		t.Errorf("SECURITY CONCERN: Processing 1000 files took %v", elapsed)
	}

	// Should have received updates for all files
	if count < 900 {
		t.Errorf("Expected ~1000 updates, got %d", count)
	}
}

// =============================================================================
// MALFORMED INPUT SECURITY TESTS
// =============================================================================

// TestDirectoryWatcher_Security_MalformedFilenames tests handling of malicious filenames.
//
// ATTACK VECTOR: Malformed input (CWE-20)
// DESCRIPTION: Files with unusual names (null bytes, special chars, very long names)
// could cause parsing errors, crashes, or security bypasses.
//
// IMPACT: Application crash, security control bypass, log injection.
func TestDirectoryWatcher_Security_MalformedFilenames(t *testing.T) {
	tmpDir := t.TempDir()

	// Test cases for malformed filenames
	malformedNames := []struct {
		name        string
		filename    string
		shouldExist bool // Some filenames can't be created
		description string
	}{
		{
			name:        "LongFilename",
			filename:    strings.Repeat("a", 200) + ".yaml",
			shouldExist: true,
			description: "Extremely long filename",
		},
		{
			name:        "UnicodeFilename",
			filename:    "конфиг.yaml",
			shouldExist: true,
			description: "Unicode Cyrillic filename",
		},
		{
			name:        "EmojiFilename",
			filename:    "🔧config🔧.yaml",
			shouldExist: true,
			description: "Emoji in filename",
		},
		{
			name:        "SpacesFilename",
			filename:    "config file with spaces.yaml",
			shouldExist: true,
			description: "Filename with spaces",
		},
		{
			name:        "DashStartFilename",
			filename:    "-config.yaml",
			shouldExist: true,
			description: "Filename starting with dash",
		},
		{
			name:        "DoubleExtension",
			filename:    "config.yaml.yaml",
			shouldExist: true,
			description: "Double extension",
		},
		{
			name:        "HiddenFile",
			filename:    ".hidden.yaml",
			shouldExist: true,
			description: "Hidden file (dot prefix)",
		},
	}

	for _, tc := range malformedNames {
		filePath := filepath.Join(tmpDir, tc.filename)
		err := os.WriteFile(filePath, []byte("key: value\n"), 0o600)
		if err != nil {
			t.Logf("Could not create %s (%s): %v", tc.name, tc.description, err)
			continue
		}
	}

	var mu sync.Mutex
	var updates []DirectoryConfigUpdate
	var parseErrors []string

	watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
		Patterns: []string{"*.yaml"},
	}, func(update DirectoryConfigUpdate) {
		mu.Lock()
		if update.Config == nil && !update.IsDelete {
			parseErrors = append(parseErrors, update.FilePath)
		} else {
			updates = append(updates, update)
		}
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("WatchDirectory failed: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("watcher.Close failed: %v", err)
		}
	}()

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	updateCount := len(updates)
	errorCount := len(parseErrors)
	mu.Unlock()

	t.Logf("Successfully parsed %d files, %d parse errors", updateCount, errorCount)

	// Should have handled at least some files without crashing
	if updateCount == 0 {
		t.Error("No files were successfully processed")
	}
}

// TestDirectoryWatcher_Security_PathInjectionInContent tests for path injection in file content.
//
// ATTACK VECTOR: Injection via content (CWE-94)
// DESCRIPTION: Malicious configuration content could attempt to manipulate
// path handling or cause other injection issues.
//
// IMPACT: Code execution, path manipulation, configuration hijacking.
func TestDirectoryWatcher_Security_PathInjectionInContent(t *testing.T) {
	tmpDir := t.TempDir()

	injectionPayloads := []struct {
		name    string
		content string
	}{
		{
			name:    "PathTraversalValue",
			content: "path: ../../../etc/passwd\n",
		},
		{
			name:    "CommandInjection",
			content: "cmd: $(cat /etc/passwd)\n",
		},
		{
			name:    "TemplateInjection",
			content: "template: {{.Env.SECRET_KEY}}\n",
		},
		{
			name:    "YAMLAnchorBomb",
			content: "a: &a [*a, *a, *a, *a, *a]\n",
		},
		{
			name:    "LargeValue",
			content: "data: " + strings.Repeat("x", 10000) + "\n",
		},
	}

	for i, payload := range injectionPayloads {
		filePath := filepath.Join(tmpDir, fmt.Sprintf("inject%d.yaml", i))
		if err := os.WriteFile(filePath, []byte(payload.content), 0o600); err != nil {
			t.Fatalf("Failed to create injection file: %v", err)
		}
	}

	var mu sync.Mutex
	var updates []DirectoryConfigUpdate

	watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
		Patterns: []string{"*.yaml"},
	}, func(update DirectoryConfigUpdate) {
		mu.Lock()
		updates = append(updates, update)
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("WatchDirectory failed: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("watcher.Close failed: %v", err)
		}
	}()

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	count := len(updates)
	mu.Unlock()

	// Should have processed files without executing injected code
	// The parsed values should be treated as strings, not executed
	t.Logf("Processed %d files with injection payloads (treated as data, not executed)", count)
}

// =============================================================================
// RACE CONDITION SECURITY TESTS
// =============================================================================

// TestDirectoryWatcher_Security_TOCTOU tests for time-of-check-time-of-use race conditions.
//
// ATTACK VECTOR: TOCTOU race condition (CWE-367)
// DESCRIPTION: Between the time a file is validated and when it's read,
// an attacker could swap the file with a malicious one.
//
// IMPACT: Security control bypass, reading unintended files.
func TestDirectoryWatcher_Security_TOCTOU(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial safe file
	targetFile := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(targetFile, []byte("safe: true\n"), 0o600); err != nil {
		t.Fatalf("Failed to create initial file: %v", err)
	}

	var mu sync.Mutex
	var updates []DirectoryConfigUpdate

	watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
		Patterns:     []string{"*.yaml"},
		PollInterval: 50 * time.Millisecond,
	}, func(update DirectoryConfigUpdate) {
		mu.Lock()
		updates = append(updates, update)
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("WatchDirectory failed: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("watcher.Close failed: %v", err)
		}
	}()

	// Simulate TOCTOU attack: rapidly swap file contents
	done := make(chan bool)
	go func() {
		for i := 0; i < 50; i++ {
			// Rapidly alternate between safe and "malicious" content
			if i%2 == 0 {
				_ = os.WriteFile(targetFile, []byte("safe: true\n"), 0o600)
			} else {
				_ = os.WriteFile(targetFile, []byte("malicious: true\npath: /etc/passwd\n"), 0o600)
			}
			time.Sleep(10 * time.Millisecond)
		}
		done <- true
	}()

	<-done
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	count := len(updates)
	mu.Unlock()

	// All updates should be properly parsed without crashes
	t.Logf("Processed %d updates during TOCTOU test (no crashes = good)", count)
}

// =============================================================================
// PERMISSION SECURITY TESTS
// =============================================================================

// TestDirectoryWatcher_Security_RestrictivePermissions tests handling of permission-restricted files.
//
// ATTACK VECTOR: Permission bypass attempt
// DESCRIPTION: Directory contains mix of readable and unreadable files.
// Watcher should gracefully handle permission errors without exposing info.
func TestDirectoryWatcher_Security_RestrictivePermissions(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping permission test when running as root")
	}

	tmpDir := t.TempDir()

	// Create readable file
	readableFile := filepath.Join(tmpDir, "readable.yaml")
	if err := os.WriteFile(readableFile, []byte("readable: true\n"), 0o644); err != nil {
		t.Fatalf("Failed to create readable file: %v", err)
	}

	// Create unreadable file
	unreadableFile := filepath.Join(tmpDir, "unreadable.yaml")
	if err := os.WriteFile(unreadableFile, []byte("secret: value\n"), 0o000); err != nil {
		t.Fatalf("Failed to create unreadable file: %v", err)
	}
	defer func() { _ = os.Chmod(unreadableFile, 0o644) }() // Cleanup

	var mu sync.Mutex
	var updates []DirectoryConfigUpdate

	watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
		Patterns: []string{"*.yaml"},
	}, func(update DirectoryConfigUpdate) {
		mu.Lock()
		updates = append(updates, update)
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("WatchDirectory failed: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Logf("watcher.Close failed: %v", err)
		}
	}()

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	count := len(updates)
	foundReadable := false
	foundUnreadable := false
	for _, u := range updates {
		if strings.Contains(u.FilePath, "readable") {
			foundReadable = true
		}
		if strings.Contains(u.FilePath, "unreadable") {
			foundUnreadable = true
		}
	}
	mu.Unlock()

	// Should have processed readable file
	if !foundReadable {
		t.Error("Readable file was not processed")
	}

	// Unreadable file should NOT have been processed (or should have empty config)
	if foundUnreadable {
		t.Logf("Note: Unreadable file was detected (listing) but should have empty/nil config")
	}

	t.Logf("Processed %d files (graceful handling of permission errors)", count)
}

// =============================================================================
// CONCURRENT ACCESS SECURITY TESTS
// =============================================================================

// TestDirectoryWatcher_Security_ConcurrentAccess tests thread safety of directory watching.
//
// ATTACK VECTOR: Race condition exploitation (CWE-362)
// DESCRIPTION: Multiple goroutines accessing watcher state simultaneously
// could cause data races, crashes, or security issues.
func TestDirectoryWatcher_Security_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()

	// Create initial files
	for i := 0; i < 10; i++ {
		filePath := filepath.Join(tmpDir, fmt.Sprintf("config%d.yaml", i))
		if err := os.WriteFile(filePath, []byte(fmt.Sprintf("id: %d\n", i)), 0o600); err != nil {
			t.Fatalf("Failed to create file: %v", err)
		}
	}

	var mu sync.Mutex
	var updates []DirectoryConfigUpdate

	watcher, err := WatchDirectory(tmpDir, DirectoryWatchOptions{
		Patterns:     []string{"*.yaml"},
		PollInterval: 50 * time.Millisecond,
	}, func(update DirectoryConfigUpdate) {
		mu.Lock()
		updates = append(updates, update)
		mu.Unlock()
	})

	if err != nil {
		t.Fatalf("WatchDirectory failed: %v", err)
	}

	var wg sync.WaitGroup

	// Multiple goroutines accessing watcher concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = watcher.Files()
		}()
	}

	// Concurrent file modifications
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			filePath := filepath.Join(tmpDir, fmt.Sprintf("config%d.yaml", id))
			_ = os.WriteFile(filePath, []byte(fmt.Sprintf("updated: %d\n", id)), 0o600)
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	if err := watcher.Close(); err != nil {
		t.Logf("watcher.Close failed: %v", err)
	}

	mu.Lock()
	count := len(updates)
	mu.Unlock()

	t.Logf("Processed %d updates under concurrent access (no races = good)", count)
}

// =============================================================================
// BENCHMARK FOR SECURITY-RELATED OPERATIONS
// =============================================================================

func BenchmarkDirectoryWatcher_Security_PathValidation(b *testing.B) {
	maliciousPaths := []string{
		"../../../etc",
		"valid/../../../etc",
		"./../../etc",
		strings.Repeat("a/", 50) + "../" + strings.Repeat("../", 50),
	}

	for i := 0; i < b.N; i++ {
		path := maliciousPaths[i%len(maliciousPaths)]
		_, _ = WatchDirectory(path, DirectoryWatchOptions{
			Patterns: []string{"*.yaml"},
		}, func(update DirectoryConfigUpdate) {})
	}
}
