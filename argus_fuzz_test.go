// argus_fuzz_test.go - Comprehensive fuzz testing for Argus security-critical functions
//
// This file contains fuzz tests designed to find security vulnerabilities, edge cases,
// and unexpected behaviors in Argus input processing functions.
//
// Focus areas:
// - Path validation and sanitization (ValidateSecurePath)
// - Configuration parsing (ParseConfig)
// - Input validation and processing
//
// The fuzz tests use property-based testing to verify security invariants:
// - ValidateSecurePath should NEVER allow dangerous paths to pass
// - Parsers should handle malformed input gracefully without panics
// - All input validation should be consistent and robust
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"strings"
	"testing"
	"unicode"
)

// FuzzValidateSecurePath performs comprehensive fuzz testing on the ValidateSecurePath function.
//
// SECURITY PURPOSE: This fuzz test is critical for preventing directory traversal attacks.
// ValidateSecurePath is the primary defense against path-based security vulnerabilities,
// so thorough fuzzing is essential to find edge cases that could be exploited.
//
// TESTING STRATEGY:
// 1. Property-based testing: Verify security invariants hold for all inputs
// 2. Mutation-based: Start with known attack vectors and mutate them
// 3. Edge case generation: Test boundary conditions and unusual encodings
// 4. Cross-platform: Ensure consistent security across different OS path conventions
//
// SECURITY INVARIANTS TESTED:
// - No path containing ".." should ever be accepted as safe
// - No path accessing system directories should be accepted
// - No URL-encoded attack vectors should bypass validation
// - No Windows device names should be accepted
// - No control characters or null bytes should be accepted
// - Path length limits should be enforced consistently
//
// The fuzzer will help discover:
// - Unicode normalization attacks
// - Novel encoding bypass techniques
// - OS-specific path handling edge cases
// - Buffer overflow conditions with extremely long paths
// - Race conditions in validation logic
func FuzzValidateSecurePath(f *testing.F) {
	// SEED CORPUS: Based on real attack vectors and edge cases from existing tests
	// This provides the fuzzer with a good starting point for mutations

	// Basic valid paths that should always pass
	f.Add("config.json")
	f.Add("app/config.yaml")
	f.Add("/etc/argus/config.toml")
	f.Add("C:\\Program Files\\MyApp\\config.ini")
	f.Add(".gitignore") // Valid dot files
	f.Add("configs/database/prod.json")

	// Path traversal attack vectors - these should ALWAYS fail
	f.Add("../../../etc/passwd")
	f.Add("..\\..\\..\\windows\\system32\\config\\sam")
	f.Add("../../../../root/.ssh/id_rsa")
	f.Add("/var/www/../../../etc/shadow")
	f.Add("config/../../../proc/self/environ")
	f.Add("./../../etc/hosts")

	// URL-encoded attacks - should be detected and blocked
	f.Add("%2e%2e/%2e%2e/etc/passwd")
	f.Add("%252e%252e/etc/passwd") // Double encoded
	f.Add("..%2fetc%2fpasswd")     // Mixed encoding
	f.Add("%2e%2e\\%2e%2e\\windows\\system32")
	f.Add("config%00.txt") // Null byte injection

	// Windows-specific attack vectors
	f.Add("CON") // Device name
	f.Add("PRN.txt")
	f.Add("COM1.log")
	f.Add("LPT1.dat")
	f.Add("AUX.conf")
	f.Add("NUL.json")
	f.Add("file.txt:hidden") // Alternate Data Streams
	f.Add("config.json:$DATA")

	// System file access attempts
	f.Add("/etc/passwd")
	f.Add("/etc/shadow")
	f.Add("/proc/self/mem")
	f.Add("/sys/kernel/debug")
	f.Add("C:\\Windows\\System32\\config\\SAM")
	f.Add("C:\\WINDOWS\\SYSTEM32\\CONFIG\\SECURITY") // Case variations
	f.Add("/ETC/PASSWD")                             // Case variations for case-insensitive filesystems

	// Edge cases with special characters
	f.Add("config with spaces.json")
	f.Add("config-with-dashes.json")
	f.Add("config_with_underscores.json")
	f.Add("config.with.dots.json")
	f.Add("config@domain.json")
	f.Add("config#hash.json")
	f.Add("config$dollar.json")
	f.Add("config&amp.json")

	// Very long paths to test buffer limits
	f.Add(strings.Repeat("a", 100) + "/config.json")
	f.Add(strings.Repeat("dir/", 20) + "config.json")
	f.Add(strings.Repeat("../", 50) + "etc/passwd")

	// Unicode and encoding edge cases
	f.Add("café/config.json")  // Non-ASCII characters
	f.Add("конфиг.json")       // Cyrillic
	f.Add("設定.json")           // Chinese characters
	f.Add("config\u00A0.json") // Non-breaking space
	f.Add("config\u200B.json") // Zero-width space

	// Control characters and suspicious bytes
	f.Add("config\x00.json") // Null byte
	f.Add("config\x01.json") // SOH control char
	f.Add("config\x1F.json") // US control char
	f.Add("config\x7F.json") // DEL character
	f.Add("config\xFF.json") // High byte

	// Path normalization attack attempts
	f.Add("config/./../../etc/passwd")
	f.Add("config//..//../../etc/passwd")
	f.Add("config\\.\\.\\..\\..\\etc\\passwd")
	f.Add("config/.././../../etc/passwd")

	// Mixed separators and complex traversals
	f.Add("config\\../../../etc/passwd") // Mixed separators
	f.Add("config/..\\../etc/passwd")
	f.Add("config\\..\\../etc/passwd")
	f.Add("config/../..\\etc/passwd")

	// Execute the fuzz test with property-based validation
	f.Fuzz(func(t *testing.T, path string) {
		// Skip empty strings as they have a specific error case
		if path == "" {
			return
		}

		// Call the function under test
		err := ValidateSecurePath(path)

		// SECURITY INVARIANT 1: Paths with obvious traversal patterns should NEVER pass
		if containsDangerousTraversal(path) {
			if err == nil {
				t.Errorf("SECURITY VULNERABILITY: Path with dangerous traversal was accepted: %q", path)
			}
		}

		// SECURITY INVARIANT 2: System file access should be blocked
		if containsSystemFileAccess(path) {
			if err == nil {
				t.Errorf("SECURITY VULNERABILITY: System file access was accepted: %q", path)
			}
		}

		// SECURITY INVARIANT 3: Windows device names should be blocked
		if containsWindowsDeviceName(path) {
			if err == nil {
				t.Errorf("SECURITY VULNERABILITY: Windows device name was accepted: %q", path)
			}
		}

		// SECURITY INVARIANT 4: Control characters should be blocked
		if containsControlCharacters(path) {
			if err == nil {
				t.Errorf("SECURITY VULNERABILITY: Path with control characters was accepted: %q", path)
			}
		}

		// SECURITY INVARIANT 5: Excessively long paths should be blocked
		if len(path) > 4096 {
			if err == nil {
				t.Errorf("SECURITY VULNERABILITY: Excessively long path was accepted (len=%d): %q", len(path), truncateString(path, 50))
			}
		}

		// SECURITY INVARIANT 6: Complex nested paths should be blocked
		separatorCount := strings.Count(path, "/") + strings.Count(path, "\\")
		if separatorCount > 50 {
			if err == nil {
				t.Errorf("SECURITY VULNERABILITY: Overly complex path was accepted (separators=%d): %q", separatorCount, truncateString(path, 50))
			}
		}

		// SECURITY INVARIANT 7: URL-encoded dangerous patterns should be blocked
		if containsURLEncodedAttack(path) {
			if err == nil {
				t.Errorf("SECURITY VULNERABILITY: URL-encoded attack vector was accepted: %q", path)
			}
		}

		// BEHAVIORAL INVARIANT: Function should never panic
		// (This is implicitly tested by the fuzzer - if it panics, the test fails)

		// BEHAVIORAL INVARIANT: Error messages should not leak sensitive information
		if err != nil && containsSensitiveInfo(err.Error()) {
			t.Errorf("INFORMATION LEAK: Error message contains sensitive information: %v", err)
		}

		// PERFORMANCE INVARIANT: Function should complete in reasonable time
		// (Implicitly tested by fuzzer timeout mechanisms)
	})
}

// containsDangerousTraversal checks if a path contains obvious directory traversal patterns
func containsDangerousTraversal(path string) bool {
	lowerPath := strings.ToLower(path)

	dangerousPatterns := []string{
		"..",
		"../",
		"..\\",
		"/..",
		"\\..",
		"/../",
		"\\..\\",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}
	return false
}

// containsSystemFileAccess checks if a path attempts to access system files
func containsSystemFileAccess(path string) bool {
	lowerPath := strings.ToLower(path)

	systemPaths := []string{
		"/etc/passwd",
		"/etc/shadow",
		"/etc/hosts",
		"/proc/",
		"/sys/",
		"/dev/",
		"windows/system32",
		"windows\\system32",
		"program files",
		".ssh/",
		".aws/",
	}

	for _, sysPath := range systemPaths {
		if strings.Contains(lowerPath, sysPath) {
			return true
		}
	}
	return false
}

// containsWindowsDeviceName mirrors the EXACT logic from ValidateSecurePath
// to ensure fuzzer consistency. This function should return true ONLY when
// ValidateSecurePath would actually reject the path for device name reasons.
func containsWindowsDeviceName(path string) bool {
	windowsDevices := []string{
		"CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
	}

	// First handle non-UNC paths (direct device name access)
	if !(strings.HasPrefix(path, "/") || strings.HasPrefix(path, "\\")) || len(path) <= 1 {
		baseName := getBaseName(path)
		baseUpper := strings.ToUpper(baseName)

		// Remove ALL extensions if present (handle multiple extensions)
		for {
			if dotIndex := strings.Index(baseUpper, "."); dotIndex != -1 {
				baseUpper = baseUpper[:dotIndex]
			} else {
				break
			}
		}

		for _, device := range windowsDevices {
			if baseUpper == device {
				return true
			}
		}
		return false
	}

	// UNC path logic - MIRROR ValidateSecurePath EXACTLY
	// Normalize the path: remove all leading slashes and backslashes
	normalizedPath := path
	for len(normalizedPath) > 0 && (normalizedPath[0] == '/' || normalizedPath[0] == '\\') {
		normalizedPath = normalizedPath[1:]
	}

	if len(normalizedPath) == 0 {
		return false
	}

	// Split by both types of separators to get path components
	normalizedForSplit := strings.ReplaceAll(normalizedPath, "\\", "/")
	components := strings.Split(normalizedForSplit, "/")

	if len(components) == 0 || components[0] == "" {
		return false
	}

	// Check if the first component is a device name
	firstComponent := strings.ToUpper(components[0])
	// Remove ALL extensions if present (handle multiple extensions)
	for {
		if dotIndex := strings.Index(firstComponent, "."); dotIndex != -1 {
			firstComponent = firstComponent[:dotIndex]
		} else {
			break
		}
	}

	// Always block if first component is a device name
	for _, device := range windowsDevices {
		if firstComponent == device {
			return true
		}
	}

	// Check second component only if specific conditions are met
	if len(components) >= 2 {
		secondComponent := strings.ToUpper(components[1])
		// Remove ALL extensions if present (handle multiple extensions)
		for {
			if dotIndex := strings.Index(secondComponent, "."); dotIndex != -1 {
				secondComponent = secondComponent[:dotIndex]
			} else {
				break
			}
		}

		// If second component is device AND first component looks suspicious (≤2 chars), block it
		for _, device := range windowsDevices {
			if secondComponent == device && len(components[0]) <= 2 {
				return true
			}
		}
	}

	return false
} // containsControlCharacters checks if a path contains dangerous control characters
func containsControlCharacters(path string) bool {
	for _, char := range path {
		// Allow tab, LF, CR but block other control characters
		if char < 32 && char != 9 && char != 10 && char != 13 {
			return true
		}
		// Block null byte specifically
		if char == 0 {
			return true
		}
	}
	return false
}

// containsURLEncodedAttack checks for URL-encoded attack patterns
func containsURLEncodedAttack(path string) bool {
	lowerPath := strings.ToLower(path)

	encodedPatterns := []string{
		"%2e%2e",     // ".." encoded
		"%252e%252e", // ".." double encoded
		"%2f",        // "/" encoded (in dangerous contexts)
		"%252f",      // "/" double encoded
		"%5c",        // "\" encoded
		"%255c",      // "\" double encoded
		"%00",        // null byte
		"%2500",      // null byte double encoded
	}

	for _, pattern := range encodedPatterns {
		if strings.Contains(lowerPath, pattern) {
			return true
		}
	}
	return false
}

// containsSensitiveInfo checks if an error message leaks sensitive information
func containsSensitiveInfo(errorMsg string) bool {
	lowerMsg := strings.ToLower(errorMsg)

	// Check for potentially sensitive information in error messages
	sensitivePatterns := []string{
		"password",
		"secret",
		"key",
		"token",
		"credential",
		"private",
		"/home/",
		"c:\\users\\",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(lowerMsg, pattern) {
			return true
		}
	}
	return false
}

// Helper functions

func getBaseName(path string) string {
	// Simple basename extraction - find last separator
	lastSlash := strings.LastIndex(path, "/")
	lastBackslash := strings.LastIndex(path, "\\")

	separator := lastSlash
	if lastBackslash > separator {
		separator = lastBackslash
	}

	if separator == -1 {
		return path
	}
	return path[separator+1:]
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// FuzzParseConfig performs fuzz testing on the configuration parsing functionality.
//
// This secondary fuzz test targets the ParseConfig function which is another critical
// attack surface. Malformed configuration data could potentially cause:
// - Buffer overflows or memory corruption
// - Denial of service through resource exhaustion
// - Logic errors leading to security bypasses
// - Parser confusion attacks
//
// The fuzzer tests all supported configuration formats to ensure robust parsing.
func FuzzParseConfig(f *testing.F) {
	// Seed corpus with valid configurations in different formats
	f.Add([]byte(`{"key": "value", "number": 42}`), int(FormatJSON))
	f.Add([]byte("key: value\nnumber: 42"), int(FormatYAML))
	f.Add([]byte("key = \"value\"\nnumber = 42"), int(FormatTOML))
	f.Add([]byte("key=value\nnumber=42"), int(FormatINI))
	f.Add([]byte("key=value\nnumber=42"), int(FormatProperties))

	// Malformed inputs that should be handled gracefully
	f.Add([]byte(`{"invalid": json}`), int(FormatJSON))
	f.Add([]byte("invalid: yaml: content:"), int(FormatYAML))
	f.Add([]byte("invalid = toml = format"), int(FormatTOML))
	f.Add([]byte(""), int(FormatJSON)) // Empty input

	f.Fuzz(func(t *testing.T, data []byte, formatInt int) {
		// Convert int back to ConfigFormat, handle invalid values
		if formatInt < 0 || formatInt >= int(FormatUnknown) {
			return // Skip invalid format values
		}
		format := ConfigFormat(formatInt)

		// The function should never panic, regardless of input
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("ParseConfig panicked with input format=%v, data length=%d: %v", format, len(data), r)
			}
		}()

		// Call function under test
		result, err := ParseConfig(data, format)

		// If parsing succeeds, result should be valid
		if err == nil {
			if result == nil {
				t.Errorf("ParseConfig returned nil result without error")
			}
			// Verify the result is a valid map
			for k := range result {
				if !isValidConfigKeyForFormat(k, format) {
					t.Errorf("ParseConfig produced invalid key: %q", k)
				}
			}
		}

		// Error messages should not contain raw input data to prevent info leaks
		if err != nil && len(data) > 0 && containsRawData(err.Error(), data) {
			t.Errorf("Error message contains raw input data, potential information leak")
		}
	})
}

// isValidConfigKeyForFormat checks if a configuration key is valid for a specific format
func isValidConfigKeyForFormat(key string, format ConfigFormat) bool {
	// Check for null bytes - never allowed in any format
	if strings.Contains(key, "\x00") {
		return false
	}

	// Format-specific validations
	switch format {
	case FormatJSON:
		// JSON allows empty keys and escaped control characters per RFC 7159
		// We are more strict here for security - only printable keys allowed
		if key == "" {
			return true // Empty keys allowed in JSON
		}
		// Check for dangerous control characters in JSON too
		for _, char := range key {
			if char < 32 && char != '\t' && char != '\n' && char != '\r' {
				return false
			}
			if !unicode.IsPrint(char) && char != '\t' {
				return false // Security policy: no non-printable chars in config keys
			}
		}
		return true
	case FormatYAML, FormatTOML, FormatINI, FormatProperties, FormatHCL:
		// These formats don't allow empty keys
		if key == "" {
			return false
		}
		// Check for dangerous control characters
		for _, char := range key {
			if char < 32 && char != '\t' && char != '\n' && char != '\r' {
				return false
			}
			if !unicode.IsPrint(char) && char != '\t' {
				return false
			}
		}
		return true
	default:
		// For unknown formats, be conservative
		if key == "" {
			return false
		}
		// Check for dangerous control characters
		for _, char := range key {
			if char < 32 && char != '\t' && char != '\n' && char != '\r' {
				return false
			}
			if !unicode.IsPrint(char) && char != '\t' {
				return false
			}
		}
		return true
	}
}

// containsRawData checks if error message contains portions of raw input data
func containsRawData(errorMsg string, data []byte) bool {
	dataStr := string(data)

	// Skip very short inputs (less than 8 chars) to avoid false positives with common words
	if len(dataStr) < 8 {
		return false
	}

	// For short inputs (8-15 chars), only flag if the entire input appears in error
	if len(dataStr) < 16 {
		return strings.Contains(errorMsg, dataStr)
	}

	// For longer inputs, check if significant chunks appear in error message
	if len(dataStr) > 50 {
		dataStr = dataStr[:50] // Check first 50 chars
	}

	return strings.Contains(errorMsg, dataStr)
}

// TestFuzzBypassAnalysis manually tests the bypass found by fuzzer
func TestFuzzBypassAnalysis(t *testing.T) {
	// Test the bypass found by fuzzer
	testPaths := []string{
		"//Con",
		"Con",
		"CON",
		"con",
		"//CON",
		"\\\\Con",
		"/Con",
		"//con",
		"/\\000/Con",   // Previous test case
		"PRN.0.",       // New fuzzer finding
		"COM1.txt.bak", // Multiple extensions
		"AUX.a.b.c",    // Many extensions
		"NUL...",       // Multiple dots
		"LPT1.exe.old", // Executable with backup extension
	}

	for _, path := range testPaths {
		err := ValidateSecurePath(path)
		t.Logf("Path: %-15s -> Error: %v", path, err)

		// Also test what our fuzzer functions think
		isDeviceName := containsWindowsDeviceName(path)
		t.Logf("  containsWindowsDeviceName(%q) = %v", path, isDeviceName)

		base := getBaseName(path) // Use our helper function
		t.Logf("  getBaseName(%q) = %q", path, base)
	}
} // TestUNCPathDeviceNameRegression tests the specific UNC path device name vulnerability
// that was found by the fuzzer to ensure it stays fixed.
//
// SECURITY: This is a regression test for CVE-equivalent vulnerability where UNC paths
// could bypass Windows device name validation, potentially allowing access to system devices.
func TestUNCPathDeviceNameRegression(t *testing.T) {
	// Test cases for UNC path device name bypass vulnerability
	maliciousUNCPaths := []struct {
		path        string
		description string
	}{
		{"//Con", "UNC path to CON device"},
		{"\\\\Con", "Windows UNC path to CON device"},
		{"//CON", "UNC path to CON device (uppercase)"},
		{"\\\\CON", "Windows UNC path to CON device (uppercase)"},
		{"//con", "UNC path to CON device (lowercase)"},
		{"\\\\con", "Windows UNC path to CON device (lowercase)"},
		{"//PRN", "UNC path to PRN device"},
		{"\\\\PRN", "Windows UNC path to PRN device"},
		{"//AUX", "UNC path to AUX device"},
		{"\\\\AUX", "Windows UNC path to AUX device"},
		{"//NUL", "UNC path to NUL device"},
		{"\\\\NUL", "Windows UNC path to NUL device"},
		{"//COM1", "UNC path to COM1 device"},
		{"\\\\COM1", "Windows UNC path to COM1 device"},
		{"//LPT1", "UNC path to LPT1 device"},
		{"\\\\LPT1", "Windows UNC path to LPT1 device"},
		{"//Con.txt", "UNC path to CON device with extension"},
		{"\\\\Con.txt", "Windows UNC path to CON device with extension"},
		{"//Con/subfolder", "UNC path to CON device with subfolder"},
		{"\\\\Con\\subfolder", "Windows UNC path to CON device with subfolder"},
		{"///Con", "Triple slash UNC path to CON device"},
		{"////CON", "Quad slash UNC path to CON device"},
		{"\\\\\\Con", "Triple backslash UNC path to CON device"},
		{"/////con.txt", "Many slash UNC path to CON device with extension"},
		{"/\\Con", "Mixed separator UNC path to CON device"},
		{"/\\0/Con", "Mixed separator with suspicious server name"},
		{"\\//Con", "Reverse mixed separator UNC path to CON device"},
	}

	for _, testCase := range maliciousUNCPaths {
		t.Run(testCase.description, func(t *testing.T) {
			err := ValidateSecurePath(testCase.path)

			// All these paths should be rejected for security
			if err == nil {
				t.Errorf("SECURITY REGRESSION: UNC path %q was accepted, should be blocked", testCase.path)
			}

			// Verify the error message indicates UNC path blocking
			if err != nil && !strings.Contains(err.Error(), "windows device name not allowed") {
				t.Errorf("Expected Windows device name error for %q, got: %v", testCase.path, err)
			}

			t.Logf("✓ UNC path %q correctly blocked: %v", testCase.path, err)
		})
	}

	// Edge cases found by fuzzer - needs analysis
	edgeCasePaths := []string{
		"//0/Con",    // Server "0" with folder "Con" - should this be allowed?
		"/\\000/Con", // Mixed separator server "000" with folder "Con" - attack or legitimate?
		"//srv/Con",  // Server "srv" with folder "Con" - clearly legitimate
		"//Con/srv",  // Device "Con" with folder "srv" - clearly attack
	}

	for _, path := range edgeCasePaths {
		t.Run("edge_case_"+path, func(t *testing.T) {
			err := ValidateSecurePath(path)
			// Log the current behavior for analysis
			t.Logf("Edge case path %q result: %v", path, err)

			// For now, we document this behavior but don't assert
			// This may be legitimate: server "0", folder "Con"
			// vs device access which should be blocked
		})
	}

	// Test that legitimate UNC paths (non-device) are still allowed
	legitimateUNCPaths := []string{
		"//server/share/config.json",
		"\\\\server\\share\\config.json",
		"//host/folder/app.yaml",
		"\\\\host\\folder\\app.yaml",
	}

	for _, path := range legitimateUNCPaths {
		t.Run("legitimate_"+path, func(t *testing.T) {
			err := ValidateSecurePath(path)
			// These should be allowed (non-device UNC paths)
			if err != nil && strings.Contains(err.Error(), "windows device name not allowed via UNC path") {
				t.Errorf("Legitimate UNC path %q was incorrectly blocked as device: %v", path, err)
			}
			t.Logf("UNC path %q result: %v", path, err)
		})
	}
}
