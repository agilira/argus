// config_binder_test.go - Tests for ultra-fast configuration binding system
//
// Copyright (c) 2025 AGILira - A. Giordano
// Series: an AGILira fragment
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"fmt"
	"testing"
	"time"
)

func TestConfigBinder_BasicTypes(t *testing.T) {
	// Sample configuration
	config := map[string]interface{}{
		"app_name":    "test-app",
		"port":        8080,
		"enabled":     true,
		"timeout":     "30s",
		"retry_count": int64(5),
		"rate_limit":  99.5,
	}

	// Variables to bind to
	var (
		appName    string
		port       int
		enabled    bool
		timeout    time.Duration
		retryCount int64
		rateLimit  float64
	)

	// Perform binding
	err := BindFromConfig(config).
		BindString(&appName, "app_name").
		BindInt(&port, "port").
		BindBool(&enabled, "enabled").
		BindDuration(&timeout, "timeout").
		BindInt64(&retryCount, "retry_count").
		BindFloat64(&rateLimit, "rate_limit").
		Apply()

	if err != nil {
		t.Fatalf("Binding failed: %v", err)
	}

	// Verify results
	if appName != "test-app" {
		t.Errorf("Expected appName='test-app', got '%s'", appName)
	}
	if port != 8080 {
		t.Errorf("Expected port=8080, got %d", port)
	}
	if !enabled {
		t.Errorf("Expected enabled=true, got %t", enabled)
	}
	if timeout != 30*time.Second {
		t.Errorf("Expected timeout=30s, got %v", timeout)
	}
	if retryCount != 5 {
		t.Errorf("Expected retryCount=5, got %d", retryCount)
	}
	if rateLimit != 99.5 {
		t.Errorf("Expected rateLimit=99.5, got %f", rateLimit)
	}

	t.Logf("✅ All basic types bound correctly")
}

func TestConfigBinder_WithDefaults(t *testing.T) {
	// Minimal configuration
	config := map[string]interface{}{
		"name": "test-service",
	}

	// Variables with defaults
	var (
		name    string
		port    int
		debug   bool
		timeout time.Duration
	)

	// Bind with defaults
	err := BindFromConfig(config).
		BindString(&name, "name", "default-service").
		BindInt(&port, "port", 3000).                     // Missing key, will use default
		BindBool(&debug, "debug", true).                  // Missing key, will use default
		BindDuration(&timeout, "timeout", 5*time.Second). // Missing key, will use default
		Apply()

	if err != nil {
		t.Fatalf("Binding with defaults failed: %v", err)
	}

	// Verify results
	if name != "test-service" {
		t.Errorf("Expected name='test-service', got '%s'", name)
	}
	if port != 3000 {
		t.Errorf("Expected port=3000 (default), got %d", port)
	}
	if !debug {
		t.Errorf("Expected debug=true (default), got %t", debug)
	}
	if timeout != 5*time.Second {
		t.Errorf("Expected timeout=5s (default), got %v", timeout)
	}

	t.Logf("✅ Defaults work correctly")
}

func TestConfigBinder_NestedKeys(t *testing.T) {
	// Nested configuration
	config := map[string]interface{}{
		"database": map[string]interface{}{
			"host":     "localhost",
			"port":     5432,
			"ssl_mode": "require",
			"pool": map[string]interface{}{
				"max_connections": 20,
				"idle_timeout":    "5m",
			},
		},
		"server": map[string]interface{}{
			"bind_address": "0.0.0.0",
			"port":         8080,
		},
	}

	// Variables for nested values
	var (
		dbHost        string
		dbPort        int
		sslMode       string
		maxConns      int
		idleTimeout   time.Duration
		serverAddress string
		serverPort    int
	)

	// Bind nested keys
	err := BindFromConfig(config).
		BindString(&dbHost, "database.host").
		BindInt(&dbPort, "database.port").
		BindString(&sslMode, "database.ssl_mode").
		BindInt(&maxConns, "database.pool.max_connections").
		BindDuration(&idleTimeout, "database.pool.idle_timeout").
		BindString(&serverAddress, "server.bind_address").
		BindInt(&serverPort, "server.port").
		Apply()

	if err != nil {
		t.Fatalf("Nested binding failed: %v", err)
	}

	// Verify nested values
	if dbHost != "localhost" {
		t.Errorf("Expected dbHost='localhost', got '%s'", dbHost)
	}
	if dbPort != 5432 {
		t.Errorf("Expected dbPort=5432, got %d", dbPort)
	}
	if sslMode != "require" {
		t.Errorf("Expected sslMode='require', got '%s'", sslMode)
	}
	if maxConns != 20 {
		t.Errorf("Expected maxConns=20, got %d", maxConns)
	}
	if idleTimeout != 5*time.Minute {
		t.Errorf("Expected idleTimeout=5m, got %v", idleTimeout)
	}
	if serverAddress != "0.0.0.0" {
		t.Errorf("Expected serverAddress='0.0.0.0', got '%s'", serverAddress)
	}
	if serverPort != 8080 {
		t.Errorf("Expected serverPort=8080, got %d", serverPort)
	}

	t.Logf("✅ Nested key binding works correctly")
}

func TestConfigBinder_ErrorHandling(t *testing.T) {
	config := map[string]interface{}{
		"invalid_int":      "not-a-number",
		"invalid_bool":     "maybe",
		"invalid_duration": "not-a-duration",
		"invalid_float":    "not-a-float",
	}

	var (
		invalidInt      int
		invalidBool     bool
		invalidDuration time.Duration
		invalidFloat    float64
	)

	// Test invalid int
	err := BindFromConfig(config).
		BindInt(&invalidInt, "invalid_int").
		Apply()
	if err == nil {
		t.Error("Expected error for invalid int, got none")
	}

	// Test invalid bool
	err = BindFromConfig(config).
		BindBool(&invalidBool, "invalid_bool").
		Apply()
	if err == nil {
		t.Error("Expected error for invalid bool, got none")
	}

	// Test invalid duration
	err = BindFromConfig(config).
		BindDuration(&invalidDuration, "invalid_duration").
		Apply()
	if err == nil {
		t.Error("Expected error for invalid duration, got none")
	}

	// Test invalid float
	err = BindFromConfig(config).
		BindFloat64(&invalidFloat, "invalid_float").
		Apply()
	if err == nil {
		t.Error("Expected error for invalid float, got none")
	}

	t.Logf("✅ Error handling works correctly")
}

func TestConfigBinder_TypeConversions(t *testing.T) {
	// Configuration with various type representations
	config := map[string]interface{}{
		"string_int":   "42",
		"float_to_int": 42.7,
		"int_to_bool":  1,
		"zero_to_bool": 0,
		"string_bool":  "true",
		"int_to_float": 42,
		"string_float": "3.14159",
		"int_duration": int64(5000000000), // 5 seconds in nanoseconds
	}

	var (
		stringInt   int
		floatToInt  int
		intToBool   bool
		zeroToBool  bool
		stringBool  bool
		intToFloat  float64
		stringFloat float64
		intDuration time.Duration
	)

	err := BindFromConfig(config).
		BindInt(&stringInt, "string_int").
		BindInt(&floatToInt, "float_to_int").
		BindBool(&intToBool, "int_to_bool").
		BindBool(&zeroToBool, "zero_to_bool").
		BindBool(&stringBool, "string_bool").
		BindFloat64(&intToFloat, "int_to_float").
		BindFloat64(&stringFloat, "string_float").
		BindDuration(&intDuration, "int_duration").
		Apply()

	if err != nil {
		t.Fatalf("Type conversion binding failed: %v", err)
	}

	// Verify conversions
	if stringInt != 42 {
		t.Errorf("Expected stringInt=42, got %d", stringInt)
	}
	if floatToInt != 42 {
		t.Errorf("Expected floatToInt=42, got %d", floatToInt)
	}
	if !intToBool {
		t.Errorf("Expected intToBool=true, got %t", intToBool)
	}
	if zeroToBool {
		t.Errorf("Expected zeroToBool=false, got %t", zeroToBool)
	}
	if !stringBool {
		t.Errorf("Expected stringBool=true, got %t", stringBool)
	}
	if intToFloat != 42.0 {
		t.Errorf("Expected intToFloat=42.0, got %f", intToFloat)
	}
	if stringFloat != 3.14159 {
		t.Errorf("Expected stringFloat=3.14159, got %f", stringFloat)
	}
	if intDuration != 5*time.Second {
		t.Errorf("Expected intDuration=5s, got %v", intDuration)
	}

	t.Logf("✅ Type conversions work correctly")
}

func BenchmarkConfigBinder_Apply(b *testing.B) {
	// Large configuration for benchmarking
	config := map[string]interface{}{
		"string1": "value1", "string2": "value2", "string3": "value3",
		"int1": 1, "int2": 2, "int3": 3,
		"bool1": true, "bool2": false, "bool3": true,
		"float1": 1.1, "float2": 2.2, "float3": 3.3,
		"duration1": "1s", "duration2": "2m", "duration3": "3h",
	}

	// Variables to bind
	var (
		s1, s2, s3 string
		i1, i2, i3 int
		b1, b2, b3 bool
		f1, f2, f3 float64
		d1, d2, d3 time.Duration
	)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		err := BindFromConfig(config).
			BindString(&s1, "string1").BindString(&s2, "string2").BindString(&s3, "string3").
			BindInt(&i1, "int1").BindInt(&i2, "int2").BindInt(&i3, "int3").
			BindBool(&b1, "bool1").BindBool(&b2, "bool2").BindBool(&b3, "bool3").
			BindFloat64(&f1, "float1").BindFloat64(&f2, "float2").BindFloat64(&f3, "float3").
			BindDuration(&d1, "duration1").BindDuration(&d2, "duration2").BindDuration(&d3, "duration3").
			Apply()

		if err != nil {
			b.Fatalf("Benchmark binding failed: %v", err)
		}
	}
}

func TestConfigBinder_RealWorldExample(t *testing.T) {
	// Real-world configuration example
	config := map[string]interface{}{
		"app": map[string]interface{}{
			"name":        "argus-service",
			"version":     "1.2.3",
			"environment": "production",
			"debug":       false,
		},
		"server": map[string]interface{}{
			"host":             "0.0.0.0",
			"port":             8080,
			"read_timeout":     "30s",
			"write_timeout":    "30s",
			"shutdown_timeout": "5s",
		},
		"database": map[string]interface{}{
			"host":         "db.example.com",
			"port":         5432,
			"name":         "argus_db",
			"ssl_mode":     "require",
			"max_conns":    25,
			"max_idle":     5,
			"conn_timeout": "10s",
		},
		"cache": map[string]interface{}{
			"enabled": true,
			"ttl":     "1h",
			"size":    1000,
		},
	}

	// Application configuration structure using individual variables
	var (
		// App config
		appName        string
		appVersion     string
		appEnvironment string
		appDebug       bool

		// Server config
		serverHost            string
		serverPort            int
		serverReadTimeout     time.Duration
		serverWriteTimeout    time.Duration
		serverShutdownTimeout time.Duration

		// Database config
		dbHost        string
		dbPort        int
		dbName        string
		dbSSLMode     string
		dbMaxConns    int
		dbMaxIdle     int
		dbConnTimeout time.Duration

		// Cache config
		cacheEnabled bool
		cacheTTL     time.Duration
		cacheSize    int
	)

	// Bind all configuration in one fluent call
	err := BindFromConfig(config).
		// App bindings
		BindString(&appName, "app.name").
		BindString(&appVersion, "app.version").
		BindString(&appEnvironment, "app.environment").
		BindBool(&appDebug, "app.debug").
		// Server bindings
		BindString(&serverHost, "server.host").
		BindInt(&serverPort, "server.port").
		BindDuration(&serverReadTimeout, "server.read_timeout").
		BindDuration(&serverWriteTimeout, "server.write_timeout").
		BindDuration(&serverShutdownTimeout, "server.shutdown_timeout").
		// Database bindings
		BindString(&dbHost, "database.host").
		BindInt(&dbPort, "database.port").
		BindString(&dbName, "database.name").
		BindString(&dbSSLMode, "database.ssl_mode").
		BindInt(&dbMaxConns, "database.max_conns").
		BindInt(&dbMaxIdle, "database.max_idle").
		BindDuration(&dbConnTimeout, "database.conn_timeout").
		// Cache bindings
		BindBool(&cacheEnabled, "cache.enabled").
		BindDuration(&cacheTTL, "cache.ttl").
		BindInt(&cacheSize, "cache.size").
		Apply()

	if err != nil {
		t.Fatalf("Real-world binding failed: %v", err)
	}

	// Verify some key values
	if appName != "argus-service" {
		t.Errorf("Expected appName='argus-service', got '%s'", appName)
	}
	if serverPort != 8080 {
		t.Errorf("Expected serverPort=8080, got %d", serverPort)
	}
	if dbMaxConns != 25 {
		t.Errorf("Expected dbMaxConns=25, got %d", dbMaxConns)
	}
	if !cacheEnabled {
		t.Errorf("Expected cacheEnabled=true, got %t", cacheEnabled)
	}
	if cacheTTL != time.Hour {
		t.Errorf("Expected cacheTTL=1h, got %v", cacheTTL)
	}

	t.Logf("✅ Real-world configuration binding successful!")
	t.Logf("   App: %s v%s (%s)", appName, appVersion, appEnvironment)
	t.Logf("   Server: %s:%d", serverHost, serverPort)
	t.Logf("   Database: %s:%d/%s", dbHost, dbPort, dbName)
	t.Logf("   Cache: enabled=%t, ttl=%v, size=%d", cacheEnabled, cacheTTL, cacheSize)
}

// TestConfigBinder_ErrorConditions tests error handling paths for higher coverage
func TestConfigBinder_ErrorConditions(t *testing.T) {
	// Test BindInt64 with existing error
	var target int64
	cb := NewConfigBinder(map[string]interface{}{})
	cb.err = fmt.Errorf("pre-existing error")

	result := cb.BindInt64(&target, "test", 42)
	if result != cb {
		t.Error("BindInt64 should return self even with error")
	}
	if len(cb.bindings) != 0 {
		t.Error("BindInt64 should not add bindings when error exists")
	}

	// Test BindFloat64 with existing error
	var targetFloat float64
	result = cb.BindFloat64(&targetFloat, "test", 3.14)
	if result != cb {
		t.Error("BindFloat64 should return self even with error")
	}
	if len(cb.bindings) != 0 { // Should still be 0 from previous
		t.Error("BindFloat64 should not add bindings when error exists")
	}

	// Test Apply with existing error
	err := cb.Apply()
	if err == nil {
		t.Error("Apply should return pre-existing error")
	}
	if err.Error() != "pre-existing error" {
		t.Errorf("Expected pre-existing error, got %v", err)
	}
}

// TestConfigBinder_TypeConversionEdgeCases tests edge cases in type conversion
func TestConfigBinder_TypeConversionEdgeCases(t *testing.T) {
	cb := NewConfigBinder(map[string]interface{}{
		"int_invalid":      "not-a-number",
		"duration_invalid": "not-a-duration",
		"nested_invalid": map[string]interface{}{
			"intermediate": "not-a-map",
			"value":        "test",
		},
	})

	// Test toInt with invalid string
	val, exists := cb.getValue("int_invalid")
	if !exists {
		t.Fatal("int_invalid should exist")
	}
	_, err := cb.toInt(val)
	if err == nil {
		t.Error("toInt should fail with invalid string")
	}

	// Test toDuration with invalid string
	val, exists = cb.getValue("duration_invalid")
	if !exists {
		t.Fatal("duration_invalid should exist")
	}
	_, err = cb.toDuration(val)
	if err == nil {
		t.Error("toDuration should fail with invalid string")
	}

	// Test toInt with unsupported type
	_, err = cb.toInt(make(chan int))
	if err == nil {
		t.Error("toInt should fail with unsupported type")
	}

	// Test toDuration with unsupported type
	_, err = cb.toDuration(make(chan int))
	if err == nil {
		t.Error("toDuration should fail with unsupported type")
	}

	// Test getValue with invalid nested structure
	_, exists = cb.getValue("nested_invalid.intermediate.value")
	if exists {
		t.Error("getValue should return false for invalid nested structure")
	}
}

// TestConfigBinder_GetValueEdgeCases tests getValue edge cases
func TestConfigBinder_GetValueEdgeCases(t *testing.T) {
	config := map[string]interface{}{
		"simple": "value",
		"nested": map[string]interface{}{
			"deep": map[string]interface{}{
				"key": "deep-value",
			},
		},
		"invalid": map[string]interface{}{
			"intermediate": "not-a-map",
		},
	}

	cb := NewConfigBinder(config)

	// Test simple key
	val, exists := cb.getValue("simple")
	if !exists || val != "value" {
		t.Errorf("Expected simple key to return 'value', got %v, exists=%v", val, exists)
	}

	// Test deep nested key
	val, exists = cb.getValue("nested.deep.key")
	if !exists || val != "deep-value" {
		t.Errorf("Expected deep nested key to return 'deep-value', got %v, exists=%v", val, exists)
	}

	// Test non-existent key
	_, exists = cb.getValue("nonexistent")
	if exists {
		t.Error("Non-existent key should return exists=false")
	}

	// Test partial nested path that doesn't exist
	_, exists = cb.getValue("nested.missing.key")
	if exists {
		t.Error("Partial missing nested path should return exists=false")
	}

	// Test invalid intermediate type
	_, exists = cb.getValue("invalid.intermediate.key")
	if exists {
		t.Error("Invalid intermediate type should return exists=false")
	}
}

// TestConfigBinder_TypeConversionToInt tests toInt function specifically
func TestConfigBinder_TypeConversionToInt(t *testing.T) {
	cb := NewConfigBinder(map[string]interface{}{})

	// Test int to int conversion
	result, err := cb.toInt(42)
	if err != nil || result != 42 {
		t.Errorf("toInt(42) failed: got %d, %v", result, err)
	}

	// Test int64 to int conversion
	result, err = cb.toInt(int64(123))
	if err != nil || result != 123 {
		t.Errorf("toInt(int64(123)) failed: got %d, %v", result, err)
	}

	// Test float64 to int conversion
	result, err = cb.toInt(45.67)
	if err != nil || result != 45 {
		t.Errorf("toInt(45.67) failed: got %d, %v", result, err)
	}

	// Test string to int conversion
	result, err = cb.toInt("789")
	if err != nil || result != 789 {
		t.Errorf("toInt(\"789\") failed: got %d, %v", result, err)
	}

	// Test invalid string to int conversion
	_, err = cb.toInt("not-a-number")
	if err == nil {
		t.Error("toInt(\"not-a-number\") should fail")
	}

	// Test unsupported type to int conversion
	_, err = cb.toInt(true)
	if err == nil {
		t.Error("toInt(true) should fail")
	}

	// Test nil to int conversion
	_, err = cb.toInt(nil)
	if err == nil {
		t.Error("toInt(nil) should fail")
	}
}

// TestConfigBinder_TypeConversionToInt64 tests toInt64 function specifically
func TestConfigBinder_TypeConversionToInt64(t *testing.T) {
	cb := NewConfigBinder(map[string]interface{}{})

	// Test int64 to int64 conversion
	result, err := cb.toInt64(int64(42))
	if err != nil || result != 42 {
		t.Errorf("toInt64(42) failed: got %d, %v", result, err)
	}

	// Test int to int64 conversion
	result, err = cb.toInt64(123)
	if err != nil || result != 123 {
		t.Errorf("toInt64(123) failed: got %d, %v", result, err)
	}

	// Test float64 to int64 conversion
	result, err = cb.toInt64(45.67)
	if err != nil || result != 45 {
		t.Errorf("toInt64(45.67) failed: got %d, %v", result, err)
	}

	// Test string to int64 conversion
	result, err = cb.toInt64("789")
	if err != nil || result != 789 {
		t.Errorf("toInt64(\"789\") failed: got %d, %v", result, err)
	}

	// Test invalid string to int64 conversion
	_, err = cb.toInt64("not-a-number")
	if err == nil {
		t.Error("toInt64(\"not-a-number\") should fail")
	}

	// Test unsupported type to int64 conversion
	_, err = cb.toInt64(true)
	if err == nil {
		t.Error("toInt64(true) should fail")
	}
}
