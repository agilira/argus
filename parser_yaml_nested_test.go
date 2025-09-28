// parser_yaml_nested_test.go
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"testing"
)

func TestYAMLParserNested(t *testing.T) {
	t.Run("simple_nested_structure", func(t *testing.T) {
		nestedYAML := `
app:
  name: test-app
  version: 1.0.0
  config:
    debug: true
    port: 8080
database:
  host: localhost
  port: 5432
  credentials:
    username: admin
    password: secret`

		result, err := parseYAML([]byte(nestedYAML))
		if err != nil {
			t.Fatalf("Nested YAML should parse successfully: %v", err)
		}

		// Verify top-level structure
		app, exists := result["app"].(map[string]interface{})
		if !exists {
			t.Fatalf("Expected 'app' to be a nested map, got %T: %v", result["app"], result["app"])
		}

		// Verify nested values
		if app["name"] != "test-app" {
			t.Errorf("Expected app.name=test-app, got %v", app["name"])
		}

		// Verify deeply nested structure
		config, exists := app["config"].(map[string]interface{})
		if !exists {
			t.Fatalf("Expected 'app.config' to be a nested map, got %T: %v", app["config"], app["config"])
		}

		if config["debug"] != true {
			t.Errorf("Expected app.config.debug=true, got %v", config["debug"])
		}

		if config["port"] != 8080 {
			t.Errorf("Expected app.config.port=8080, got %v", config["port"])
		}

		// Verify database credentials
		db, exists := result["database"].(map[string]interface{})
		if !exists {
			t.Fatalf("Expected 'database' to be a nested map, got %T: %v", result["database"], result["database"])
		}

		credentials, exists := db["credentials"].(map[string]interface{})
		if !exists {
			t.Fatalf("Expected 'database.credentials' to be a nested map, got %T: %v", db["credentials"], db["credentials"])
		}

		if credentials["username"] != "admin" {
			t.Errorf("Expected database.credentials.username=admin, got %v", credentials["username"])
		}
	})

	t.Run("mixed_flat_and_nested", func(t *testing.T) {
		mixedYAML := `
# Mixed flat and nested configuration
debug: true
version: "2.0"
server:
  host: 0.0.0.0
  port: 3000
  ssl:
    enabled: true
    cert_file: /path/to/cert.pem
timeout: 30
features: [auth, logging, metrics]`

		result, err := parseYAML([]byte(mixedYAML))
		if err != nil {
			t.Fatalf("Mixed YAML should parse successfully: %v", err)
		}

		// Check flat values
		if result["debug"] != true {
			t.Errorf("Expected debug=true, got %v", result["debug"])
		}

		// Version dovrebbe essere una stringa quotata "2.0"
		if result["version"] != "2.0" {
			t.Errorf("Expected version=\"2.0\", got %T: %v", result["version"], result["version"])
		}

		// Check nested server config
		server, exists := result["server"].(map[string]interface{})
		if !exists {
			t.Fatalf("Expected 'server' to be a nested map, got %T: %v", result["server"], result["server"])
		}

		ssl, exists := server["ssl"].(map[string]interface{})
		if !exists {
			t.Fatalf("Expected 'server.ssl' to be a nested map, got %T: %v", server["ssl"], server["ssl"])
		}

		if ssl["enabled"] != true {
			t.Errorf("Expected server.ssl.enabled=true, got %v", ssl["enabled"])
		}

		// Check array support
		features, exists := result["features"].([]interface{})
		if !exists {
			t.Fatalf("Expected 'features' to be an array, got %T: %v", result["features"], result["features"])
		}

		if len(features) != 3 || features[0] != "auth" {
			t.Errorf("Expected features=[auth, logging, metrics], got %v", features)
		}
	})

	t.Run("empty_nested_objects", func(t *testing.T) {
		emptyNestedYAML := `
app:
  name: test
  empty_config:
  settings:
    debug: false
production: true`

		result, err := parseYAML([]byte(emptyNestedYAML))
		if err != nil {
			t.Fatalf("YAML with empty nested objects should parse: %v", err)
		}

		app, exists := result["app"].(map[string]interface{})
		if !exists {
			t.Fatalf("Expected 'app' to be a nested map")
		}

		// Check empty nested object
		emptyConfig, exists := app["empty_config"].(map[string]interface{})
		if !exists {
			t.Fatalf("Expected 'empty_config' to be an empty map, got %T: %v", app["empty_config"], app["empty_config"])
		}

		if len(emptyConfig) != 0 {
			t.Errorf("Expected empty_config to be empty, got %v", emptyConfig)
		}
	})
}
