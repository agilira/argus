// parser_yaml_improvement_test.go
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
)

func TestYAMLParserImprovement(t *testing.T) {
	t.Run("detailed_error_reporting", func(t *testing.T) {
		// test that the parser provides detailed error information
		malformedYAML := `
app: myapp
this line has no colon and should fail
port: 8080
host: localhost`

		result, err := parseYAML([]byte(malformedYAML))

		if err == nil {
			t.Fatalf("Parser should have failed but returned: %+v", result)
		}

		errorMsg := err.Error()
		t.Logf("Detailed error message: %s", errorMsg)

		// Verify that the error contains useful information
		if !contains(errorMsg, "line 3") {
			t.Errorf("Error should mention line number, got: %s", errorMsg)
		}
		if !contains(errorMsg, "missing colon") {
			t.Errorf("Error should explain the problem, got: %s", errorMsg)
		}
	})

	t.Run("invalid_characters_detected", func(t *testing.T) {
		// Test that the parser detects invalid characters
		malformedYAML := `
app: myapp
port: 8080
[invalid brackets without colon
host: localhost`

		result, err := parseYAML([]byte(malformedYAML))

		if err == nil {
			t.Fatalf("Parser should have failed on invalid characters but returned: %+v", result)
		}

		errorMsg := err.Error()
		t.Logf("Character validation error: %s", errorMsg)

		if !contains(errorMsg, "unexpected character") {
			t.Errorf("Error should mention invalid character, got: %s", errorMsg)
		}
	})

	t.Run("valid_yaml_still_works", func(t *testing.T) {
		// Verify that valid YAML still works
		validYAML := `
app: myapp
port: 8080
host: localhost
debug: true
timeout: 30.5`

		result, err := parseYAML([]byte(validYAML))
		if err != nil {
			t.Fatalf("Valid YAML should parse successfully: %v", err)
		}

		// Verify values
		if result["app"] != "myapp" {
			t.Errorf("Expected app=myapp, got %v", result["app"])
		}
		if result["port"] != int64(8080) {
			t.Logf("Port type: %T, value: %v", result["port"], result["port"])
			// Check if it's the correct number type
			switch v := result["port"].(type) {
			case int64:
				if v != 8080 {
					t.Errorf("Expected port=8080, got %v", v)
				}
			case int:
				if v != 8080 {
					t.Errorf("Expected port=8080, got %v", v)
				}
			default:
				t.Errorf("Expected port to be numeric, got %T: %v", result["port"], result["port"])
			}
		}
		if result["debug"] != true {
			t.Errorf("Expected debug=true, got %v", result["debug"])
		}
		if result["timeout"] != 30.5 {
			t.Errorf("Expected timeout=30.5, got %v", result["timeout"])
		}
	})

	t.Run("empty_key_validation", func(t *testing.T) {
		// Test empty key validation
		malformedYAML := `
app: myapp
: empty_key_should_fail
host: localhost`

		result, err := parseYAML([]byte(malformedYAML))

		if err == nil {
			t.Fatalf("Parser should have failed on empty key but returned: %+v", result)
		}

		errorMsg := err.Error()
		if !contains(errorMsg, "key cannot be empty") {
			t.Errorf("Error should mention empty key, got: %s", errorMsg)
		}
	})

	t.Run("simple_array_support", func(t *testing.T) {
		// Test simple array support
		yamlWithArray := `
app: myapp
servers: [web1, web2, web3]
ports: [8080, 9090, 3000]`

		result, err := parseYAML([]byte(yamlWithArray))
		if err != nil {
			t.Fatalf("YAML with arrays should parse successfully: %v", err)
		}

		// Verify string array
		servers, ok := result["servers"].([]interface{})
		if !ok {
			t.Fatalf("servers should be an array, got %T", result["servers"])
		}
		if len(servers) != 3 || servers[0] != "web1" {
			t.Errorf("Expected servers=[web1, web2, web3], got %v", servers)
		}

		// Verify number array
		ports, ok := result["ports"].([]interface{})
		if !ok {
			t.Fatalf("ports should be an array, got %T", result["ports"])
		}
		if len(ports) != 3 {
			t.Errorf("Expected 3 ports, got %d: %v", len(ports), ports)
		} else {
			// Flexible type checking for numbers
			firstPort := ports[0]
			switch v := firstPort.(type) {
			case int64:
				if v != 8080 {
					t.Errorf("Expected first port=8080, got %v", v)
				}
			case int:
				if v != 8080 {
					t.Errorf("Expected first port=8080, got %v", v)
				}
			default:
				t.Errorf("Expected port to be numeric, got %T: %v", firstPort, firstPort)
			}
		}
	})
}

// contains helper function for string checking
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsAt(s, substr, 1)))
}

func containsAt(s, substr string, start int) bool {
	if start >= len(s) {
		return false
	}
	if start+len(substr) > len(s) {
		return containsAt(s, substr, start+1)
	}
	if s[start:start+len(substr)] == substr {
		return true
	}
	return containsAt(s, substr, start+1)
}
