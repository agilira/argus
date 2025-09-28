// parser_yaml_comprehensive_test.go
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
)

func TestYAMLParserComprehensive(t *testing.T) {
	t.Run("production_config_example", func(t *testing.T) {
		// production config example
		productionYAML := `
# Production configuration for Argus service
app:
  name: "argus-service"
  version: "2.1.0"
  environment: production
  features:
    auth: true
    audit: true
    metrics: true

server:
  host: "0.0.0.0"
  port: 8080
  ssl:
    enabled: true
    cert_file: "/etc/ssl/certs/argus.pem"
    key_file: "/etc/ssl/private/argus.key"
  timeouts:
    read: 30s
    write: 30s
    idle: 60s

database:
  primary:
    host: "db-primary.company.com"
    port: 5432
    database: "argus_prod"
    ssl_mode: require
  replica:
    host: "db-replica.company.com"
    port: 5432
    database: "argus_prod"
    ssl_mode: require

logging:
  level: info
  format: json
  outputs: [stdout, file]
  file_config:
    path: "/var/log/argus/service.log"
    max_size: "100MB"
    rotate: true

# Flat configuration mixed with nested
debug: false
max_connections: 100
allowed_origins: ["https://app.company.com", "https://admin.company.com"]`

		result, err := parseYAML([]byte(productionYAML))
		if err != nil {
			t.Fatalf("Production YAML should parse successfully: %v", err)
		}

		// Verify top-level app config
		app := result["app"].(map[string]interface{})
		if app["name"] != "argus-service" {
			t.Errorf("Expected app.name=argus-service, got %v", app["name"])
		}

		// Verify nested SSL configuration
		server := result["server"].(map[string]interface{})
		ssl := server["ssl"].(map[string]interface{})
		if ssl["enabled"] != true {
			t.Errorf("Expected server.ssl.enabled=true, got %v", ssl["enabled"])
		}

		// Verify deeply nested database config
		database := result["database"].(map[string]interface{})
		primary := database["primary"].(map[string]interface{})
		if primary["host"] != "db-primary.company.com" {
			t.Errorf("Expected primary host, got %v", primary["host"])
		}

		// Verify array parsing
		allowedOrigins := result["allowed_origins"].([]interface{})
		if len(allowedOrigins) != 2 || allowedOrigins[0] != "https://app.company.com" {
			t.Errorf("Expected 2 allowed origins, got %v", allowedOrigins)
		}

		// Verify mixed flat values
		if result["debug"] != false {
			t.Errorf("Expected debug=false, got %v", result["debug"])
		}

		if result["max_connections"] != 100 {
			t.Errorf("Expected max_connections=100, got %v", result["max_connections"])
		}

		t.Logf("Successfully parsed complex production config with %d top-level keys", len(result))
	})

	t.Run("backwards_compatibility", func(t *testing.T) {
		// esnsure flat YAML still works
		flatYAML := `
host: localhost  
port: 8080
debug: true
name: test-service
timeout: 30.5`

		result, err := parseYAML([]byte(flatYAML))
		if err != nil {
			t.Fatalf("Flat YAML should still work: %v", err)
		}

		if result["host"] != "localhost" || result["port"] != 8080 {
			t.Errorf("Flat YAML parsing broken: host=%v, port=%v", result["host"], result["port"])
		}

		t.Logf("Backwards compatibility maintained")
	})

	t.Run("error_validation_comprehensive", func(t *testing.T) {
		errorCases := []struct {
			name          string
			yaml          string
			shouldContain string
		}{
			{
				"malformed_structure",
				"key: value\ninvalid line without colon at all",
				"missing colon",
			},
			{
				"invalid_characters",
				"key: value\n[invalid bracket line",
				"unexpected character",
			},
			{
				"empty_key",
				": value_without_key",
				"key cannot be empty",
			},
		}

		for _, tc := range errorCases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := parseYAML([]byte(tc.yaml))
				if err == nil {
					t.Errorf("Expected error for %s but parsing succeeded", tc.name)
				} else if !contains(err.Error(), tc.shouldContain) {
					t.Errorf("Expected error to contain '%s', got: %v", tc.shouldContain, err.Error())
				} else {
					t.Logf("Correctly caught error: %v", err.Error())
				}
			})
		}
	})
}
