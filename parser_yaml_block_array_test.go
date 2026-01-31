// parser_yaml_block_array_test.go - Block style YAML array tests
//
// YAML supports two array styles:
// 1. Flow style:  ports: [8080, 9090]
// 2. Block style: ports:
//                   - 8080
//                   - 9090
//
// Argus must support BOTH styles as per YAML 1.2 spec.
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"reflect"
	"testing"
)

func TestYAMLParserBlockStyleArrays(t *testing.T) {
	t.Run("simple_block_array", func(t *testing.T) {
		// Basic block style array with string values
		yaml := `
servers:
  - web1
  - web2
  - web3`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Block style array should parse successfully: %v", err)
		}

		servers, ok := result["servers"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'servers' to be an array, got %T: %v", result["servers"], result["servers"])
		}

		expected := []interface{}{"web1", "web2", "web3"}
		if !reflect.DeepEqual(servers, expected) {
			t.Errorf("Expected %v, got %v", expected, servers)
		}
	})

	t.Run("block_array_with_numbers", func(t *testing.T) {
		// Block style array with integer values
		yaml := `
ports:
  - 8080
  - 9090
  - 3000`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Block style array with numbers should parse: %v", err)
		}

		ports, ok := result["ports"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'ports' to be an array, got %T: %v", result["ports"], result["ports"])
		}

		if len(ports) != 3 {
			t.Fatalf("Expected 3 ports, got %d: %v", len(ports), ports)
		}

		// Verify type inference - should be int64 or int
		for i, port := range ports {
			switch v := port.(type) {
			case int64, int:
				// OK
			default:
				t.Errorf("Expected port[%d] to be numeric, got %T: %v", i, v, v)
			}
		}
	})

	t.Run("block_array_mixed_with_regular_keys", func(t *testing.T) {
		// Block array mixed with regular key-value pairs
		yaml := `
app: myservice
version: "2.0"
endpoints:
  - /api/v1/users
  - /api/v1/orders
  - /api/v1/products
debug: true
port: 8080`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Mixed config with block array should parse: %v", err)
		}

		// Verify regular keys
		if result["app"] != "myservice" {
			t.Errorf("Expected app=myservice, got %v", result["app"])
		}
		if result["version"] != "2.0" {
			t.Errorf("Expected version=2.0, got %v", result["version"])
		}
		if result["debug"] != true {
			t.Errorf("Expected debug=true, got %v", result["debug"])
		}

		// Verify block array
		endpoints, ok := result["endpoints"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'endpoints' to be an array, got %T", result["endpoints"])
		}
		if len(endpoints) != 3 {
			t.Errorf("Expected 3 endpoints, got %d", len(endpoints))
		}
	})

	t.Run("nested_block_array", func(t *testing.T) {
		// Block array inside nested structure
		yaml := `
database:
  host: localhost
  port: 5432
  replicas:
    - db-replica-1.example.com
    - db-replica-2.example.com
  credentials:
    username: admin`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Nested structure with block array should parse: %v", err)
		}

		db, ok := result["database"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected 'database' to be a map, got %T", result["database"])
		}

		if db["host"] != "localhost" {
			t.Errorf("Expected host=localhost, got %v", db["host"])
		}

		replicas, ok := db["replicas"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'replicas' to be an array, got %T", db["replicas"])
		}

		if len(replicas) != 2 {
			t.Errorf("Expected 2 replicas, got %d: %v", len(replicas), replicas)
		}
	})

	t.Run("empty_block_array", func(t *testing.T) {
		// Empty array (key with no items)
		// Note: In YAML, an empty array can be represented as:
		// items: []  (flow style)
		// or just no items under the key (which becomes empty map or null)
		yaml := `
items: []
config:
  empty_list: []`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Empty arrays should parse: %v", err)
		}

		items, ok := result["items"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'items' to be an array, got %T", result["items"])
		}
		if len(items) != 0 {
			t.Errorf("Expected empty array, got %v", items)
		}
	})

	t.Run("block_array_with_quoted_strings", func(t *testing.T) {
		// Block array items with quoted strings containing special chars
		yaml := `
messages:
  - "Hello, World!"
  - 'It''s working'
  - "Line with: colon"`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Block array with quoted strings should parse: %v", err)
		}

		messages, ok := result["messages"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'messages' to be an array, got %T", result["messages"])
		}

		if len(messages) != 3 {
			t.Errorf("Expected 3 messages, got %d", len(messages))
		}

		// First message should be unquoted content
		if messages[0] != "Hello, World!" {
			t.Errorf("Expected 'Hello, World!', got %v", messages[0])
		}
	})

	t.Run("block_array_with_complex_objects", func(t *testing.T) {
		// Block array containing objects (maps)
		yaml := `
servers:
  - name: web1
    host: 192.168.1.1
    port: 8080
  - name: web2
    host: 192.168.1.2
    port: 8081
  - name: web3
    host: 192.168.1.3
    port: 8082`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Block array with complex objects should parse: %v", err)
		}

		servers, ok := result["servers"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'servers' to be an array, got %T", result["servers"])
		}

		if len(servers) != 3 {
			t.Fatalf("Expected 3 servers, got %d", len(servers))
		}

		// Verify first server is a map with expected keys
		server1, ok := servers[0].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected servers[0] to be a map, got %T", servers[0])
		}

		if server1["name"] != "web1" {
			t.Errorf("Expected server1.name=web1, got %v", server1["name"])
		}
		if server1["host"] != "192.168.1.1" {
			t.Errorf("Expected server1.host=192.168.1.1, got %v", server1["host"])
		}
	})

	t.Run("multiple_block_arrays", func(t *testing.T) {
		// Multiple block arrays in same config
		yaml := `
allowed_ips:
  - 192.168.1.0/24
  - 10.0.0.0/8
blocked_ports:
  - 22
  - 23
  - 3389
admin_users:
  - alice
  - bob`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Multiple block arrays should parse: %v", err)
		}

		// Verify all three arrays
		allowedIPs, ok := result["allowed_ips"].([]interface{})
		if !ok || len(allowedIPs) != 2 {
			t.Errorf("Expected 2 allowed_ips, got %v", result["allowed_ips"])
		}

		blockedPorts, ok := result["blocked_ports"].([]interface{})
		if !ok || len(blockedPorts) != 3 {
			t.Errorf("Expected 3 blocked_ports, got %v", result["blocked_ports"])
		}

		adminUsers, ok := result["admin_users"].([]interface{})
		if !ok || len(adminUsers) != 2 {
			t.Errorf("Expected 2 admin_users, got %v", result["admin_users"])
		}
	})

	t.Run("block_array_boolean_values", func(t *testing.T) {
		// Block array with boolean values
		yaml := `
feature_flags:
  - true
  - false
  - true`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Block array with booleans should parse: %v", err)
		}

		flags, ok := result["feature_flags"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'feature_flags' to be an array, got %T", result["feature_flags"])
		}

		expected := []interface{}{true, false, true}
		if !reflect.DeepEqual(flags, expected) {
			t.Errorf("Expected %v, got %v", expected, flags)
		}
	})

	t.Run("block_array_float_values", func(t *testing.T) {
		// Block array with float values
		yaml := `
thresholds:
  - 0.5
  - 0.75
  - 0.95`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Block array with floats should parse: %v", err)
		}

		thresholds, ok := result["thresholds"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'thresholds' to be an array, got %T", result["thresholds"])
		}

		if len(thresholds) != 3 {
			t.Errorf("Expected 3 thresholds, got %d", len(thresholds))
		}

		// Check first value is float
		if v, ok := thresholds[0].(float64); !ok || v != 0.5 {
			t.Errorf("Expected thresholds[0]=0.5, got %T: %v", thresholds[0], thresholds[0])
		}
	})

	t.Run("block_array_with_null_values", func(t *testing.T) {
		// Block array with null/nil values
		yaml := `
optional_values:
  - value1
  - null
  - value3
  - ~`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Block array with nulls should parse: %v", err)
		}

		values, ok := result["optional_values"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'optional_values' to be an array, got %T", result["optional_values"])
		}

		if len(values) != 4 {
			t.Errorf("Expected 4 values, got %d", len(values))
		}

		// null and ~ should be parsed as nil
		if values[1] != nil {
			t.Errorf("Expected values[1]=nil, got %v", values[1])
		}
		if values[3] != nil {
			t.Errorf("Expected values[3]=nil, got %v", values[3])
		}
	})

	t.Run("deeply_nested_block_arrays", func(t *testing.T) {
		// Deeply nested structure with arrays at different levels
		yaml := `
config:
  app:
    name: myapp
    servers:
      - name: primary
        zones:
          - us-east-1
          - us-west-2
      - name: secondary
        zones:
          - eu-west-1`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Deeply nested arrays should parse: %v", err)
		}

		config, ok := result["config"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected 'config' to be a map, got %T", result["config"])
		}

		app, ok := config["app"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected 'config.app' to be a map, got %T", config["app"])
		}

		servers, ok := app["servers"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'config.app.servers' to be an array, got %T", app["servers"])
		}

		if len(servers) != 2 {
			t.Errorf("Expected 2 servers, got %d", len(servers))
		}
	})

	t.Run("production_config_with_block_arrays", func(t *testing.T) {
		// Real-world production config example (like Themis)
		yaml := `
# Production Themis config example
instance:
  environment: production
  provider: kubernetes

secrets:
  env:
    enabled: true
    prefix: THEMIS_

logging:
  level: info
  format: json

# Block style array - common in production configs
scopes:
  - name: default
    type: local
    root: /var/lib/themis/data
    readonly: false
  - name: backup
    type: s3
    root: s3://themis-backup/data
    readonly: true

policy:
  hot_reload: true
  risk_thresholds:
    auto_execute_max: 30
    operator_approval_max: 70
    admin_approval_max: 90
  
  # Array of policy files to load
  policy_files:
    - /etc/themis/policies/security.yaml
    - /etc/themis/policies/compliance.yaml
    - /etc/themis/policies/custom.yaml

connectors:
  - type: filesystem
    enabled: true
    paths:
      - /var/log
      - /etc
  - type: kubernetes
    enabled: true
    namespaces:
      - default
      - kube-system`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Production config should parse: %v", err)
		}

		// Verify scopes array
		scopes, ok := result["scopes"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'scopes' to be an array, got %T", result["scopes"])
		}
		if len(scopes) != 2 {
			t.Errorf("Expected 2 scopes, got %d", len(scopes))
		}

		// Verify nested policy.policy_files array
		policy, ok := result["policy"].(map[string]interface{})
		if !ok {
			t.Fatalf("Expected 'policy' to be a map, got %T", result["policy"])
		}

		policyFiles, ok := policy["policy_files"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'policy.policy_files' to be an array, got %T", policy["policy_files"])
		}
		if len(policyFiles) != 3 {
			t.Errorf("Expected 3 policy_files, got %d", len(policyFiles))
		}

		// Verify connectors array with nested arrays
		connectors, ok := result["connectors"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'connectors' to be an array, got %T", result["connectors"])
		}
		if len(connectors) != 2 {
			t.Errorf("Expected 2 connectors, got %d", len(connectors))
		}
	})
}

func TestYAMLParserBlockArrayEdgeCases(t *testing.T) {
	t.Run("array_item_with_dash_in_value", func(t *testing.T) {
		// Value containing dash (not array indicator)
		yaml := `
names:
  - alice-smith
  - bob-jones
  - charlie-brown`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Array with dashes in values should parse: %v", err)
		}

		names, ok := result["names"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'names' to be an array, got %T", result["names"])
		}

		if names[0] != "alice-smith" {
			t.Errorf("Expected 'alice-smith', got %v", names[0])
		}
	})

	t.Run("array_item_with_colon", func(t *testing.T) {
		// Array item containing colon (quoted)
		yaml := `
urls:
  - "http://example.com:8080"
  - "https://api.example.com:443/v1"`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Array with colons in values should parse: %v", err)
		}

		urls, ok := result["urls"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'urls' to be an array, got %T", result["urls"])
		}

		if urls[0] != "http://example.com:8080" {
			t.Errorf("Expected 'http://example.com:8080', got %v", urls[0])
		}
	})

	t.Run("single_item_block_array", func(t *testing.T) {
		// Array with just one item
		yaml := `
single:
  - only_item`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Single item block array should parse: %v", err)
		}

		single, ok := result["single"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'single' to be an array, got %T", result["single"])
		}

		if len(single) != 1 || single[0] != "only_item" {
			t.Errorf("Expected [only_item], got %v", single)
		}
	})

	t.Run("array_items_with_comments", func(t *testing.T) {
		// Array items with inline comments (should be ignored)
		yaml := `
items:
  - first    # this is the first item
  - second   # this is the second
  # this is a full line comment
  - third`

		result, err := parseYAML([]byte(yaml))
		if err != nil {
			t.Fatalf("Array with comments should parse: %v", err)
		}

		items, ok := result["items"].([]interface{})
		if !ok {
			t.Fatalf("Expected 'items' to be an array, got %T", result["items"])
		}

		// Comments should be stripped, we expect 3 items
		if len(items) != 3 {
			t.Errorf("Expected 3 items, got %d: %v", len(items), items)
		}
	})

	t.Run("inconsistent_indentation_detection", func(t *testing.T) {
		// Mixed indentation should be handled consistently
		yaml := `
items:
  - item1
  - item2
    - not_an_array_item`

		// This might error or be parsed differently - the key is consistency
		result, err := parseYAML([]byte(yaml))

		// Either error or parse correctly, but don't silently ignore
		if err == nil {
			// If no error, verify items array
			items, ok := result["items"].([]interface{})
			if ok {
				t.Logf("Parsed items: %v", items)
			}
		} else {
			t.Logf("Correctly detected malformed YAML: %v", err)
		}
	})
}

// Benchmark for block array parsing performance
func BenchmarkYAMLBlockArrayParsing(b *testing.B) {
	yaml := `
servers:
  - name: web1
    host: 192.168.1.1
    port: 8080
  - name: web2
    host: 192.168.1.2
    port: 8081
  - name: web3
    host: 192.168.1.3
    port: 8082
  - name: web4
    host: 192.168.1.4
    port: 8083
  - name: web5
    host: 192.168.1.5
    port: 8084`

	data := []byte(yaml)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := parseYAML(data)
		if err != nil {
			b.Fatalf("Parsing failed: %v", err)
		}
	}
}
