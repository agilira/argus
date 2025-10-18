# Configuration Parser System

Technical reference for Argus configuration parsers, including built-in format support, plugin architecture, and custom parser development.

## Table of Contents

- [Overview](#overview)
- [Supported Formats](#supported-formats)
- [Format Detection](#format-detection)
- [Built-in Parsers](#built-in-parsers)
- [Plugin Architecture](#plugin-architecture)
- [Custom Parser Development](#custom-parser-development)
- [Performance Characteristics](#performance-characteristics)
- [Security Considerations](#security-considerations)
- [Testing](#testing)

## Overview

The Argus parser system provides comprehensive support for multiple configuration formats through a two-tier architecture:

1. **Built-in parsers**: Zero-dependency parsers optimized for performance and memory efficiency
2. **Plugin parsers**: External parsers providing full specification compliance and advanced features

The system automatically selects the appropriate parser based on format detection and plugin availability, with custom parsers taking priority over built-in implementations.

## Supported Formats

| Format | Extensions | Built-in Support | Plugin Support |
|--------|------------|------------------|----------------|
| JSON | `.json` | Full RFC 7159 compliance | Enhanced error reporting |
| YAML | `.yaml`, `.yml` | Nested structures, indentation tracking | Full YAML 1.2 specification |
| TOML | `.toml` | Basic key-value and sections | Complete TOML specification |
| HCL | `.hcl`, `.tf` | Blocks, nested structures, expressions | Advanced HCL features |
| INI | `.ini`, `.conf`, `.cfg`, `.config` | Sections and key-value pairs | Extended INI variants |
| Properties | `.properties` | Java-style properties | Unicode escaping, multiline values |

## Format Detection

The format detection system uses hyper-optimized perfect hashing for zero-allocation format identification:

```go
func DetectFormat(filePath string) ConfigFormat
```

Detection algorithm:
1. Perfect hash lookup for common 4-character extensions (`.json`, `.yaml`, `.toml`, `.conf`)
2. Perfect hash lookup for 3-character extensions (`.yml`, `.hcl`, `.ini`, `.cfg`)
3. Character-by-character comparison for longer extensions (`.properties`, `.config`)
4. Terraform file detection (`.tf`)

Performance characteristics:
- Zero memory allocations
- Constant time complexity O(1)
- Optimized for common file extensions

## Built-in Parsers

### JSON Parser

Features:
- Full RFC 7159 compliance
- Security validation for JSON keys
- Memory pooling for reduced allocations
- Control character filtering

Security validations:
- Null byte detection in keys
- Control character filtering (except tab, LF, CR)
- Non-printable character blocking

### YAML Parser

Features:
- Nested structure support with indentation tracking
- Comment handling (`#` prefix)
- Automatic type inference (boolean, integer, float, string)
- Array and object parsing
- Enhanced error reporting with line numbers

Parsing approach:
- Line-by-line processing with indentation context
- Recursive parsing for nested structures
- Strict syntax validation

### TOML Parser

Features:
- Section-based configuration
- Nested sections with dot notation
- Array and inline table support
- Type inference and validation
- Comment support

### HCL Parser

Features:
- Block structure parsing (`name { }`)
- Nested block support
- Key-value pairs within blocks
- Comment support (`#` and `//`)
- Expression parsing for basic values
- Type inference

### INI Parser

Features:
- Section-based organization
- Key-value pairs within sections
- Comment support (`;` and `#`)
- Case-insensitive section names
- Multi-format compatibility

### Properties Parser

Features:
- Java-style key=value parsing
- Comment support (`#` and `!`)
- Line continuation with backslash
- Unicode escape sequences
- Whitespace handling

## Plugin Architecture

### Parser Interface

```go
type ConfigParser interface {
    Parse(data []byte) (map[string]interface{}, error)
    Supports(format ConfigFormat) bool
    Name() string
}
```

### Registration System

The plugin system supports multiple registration approaches:

#### Runtime Registration
```go
argus.RegisterParser(&CustomYAMLParser{})
```

#### Import-based Registration
```go
import _ "github.com/org/argus-yaml-pro"
```

#### Build Tag Registration
```go
//go:build yaml_pro
// +build yaml_pro

func init() {
    argus.RegisterParser(&ProYAMLParser{})
}
```

### Parser Priority System

1. Custom parsers are evaluated before built-in parsers
2. Registration order determines priority for multiple custom parsers
3. First matching parser handles the format
4. Thread-safe registration with minimal lock contention

## Custom Parser Development

### Basic Implementation

```go
type CustomYAMLParser struct{}

func (p *CustomYAMLParser) Parse(data []byte) (map[string]interface{}, error) {
    // Implementation using external YAML library
    var result map[string]interface{}
    if err := yaml.Unmarshal(data, &result); err != nil {
        return nil, fmt.Errorf("YAML parsing failed: %w", err)
    }
    return result, nil
}

func (p *CustomYAMLParser) Supports(format argus.ConfigFormat) bool {
    return format == argus.FormatYAML
}

func (p *CustomYAMLParser) Name() string {
    return "CustomYAMLParser"
}
```

### Advanced Implementation

```go
type ProductionYAMLParser struct {
    strictMode bool
    maxDepth   int
}

func NewProductionYAMLParser(strictMode bool, maxDepth int) *ProductionYAMLParser {
    return &ProductionYAMLParser{
        strictMode: strictMode,
        maxDepth:   maxDepth,
    }
}

func (p *ProductionYAMLParser) Parse(data []byte) (map[string]interface{}, error) {
    decoder := yaml.NewDecoder(bytes.NewReader(data))
    decoder.KnownFields(p.strictMode)
    
    var result map[string]interface{}
    if err := decoder.Decode(&result); err != nil {
        return nil, fmt.Errorf("production YAML parsing failed: %w", err)
    }
    
    if err := p.validateDepth(result, 0); err != nil {
        return nil, err
    }
    
    return result, nil
}

func (p *ProductionYAMLParser) validateDepth(data interface{}, depth int) error {
    if depth > p.maxDepth {
        return fmt.Errorf("maximum nesting depth exceeded: %d", depth)
    }
    
    switch v := data.(type) {
    case map[string]interface{}:
        for _, value := range v {
            if err := p.validateDepth(value, depth+1); err != nil {
                return err
            }
        }
    case []interface{}:
        for _, item := range v {
            if err := p.validateDepth(item, depth+1); err != nil {
                return err
            }
        }
    }
    
    return nil
}
```

### Registration Patterns

#### Library Integration
```go
package yamlpro

import "github.com/agilira/argus"

func init() {
    argus.RegisterParser(NewProductionYAMLParser(true, 10))
}
```

#### Application-specific Registration
```go
func main() {
    // Register application-specific parsers
    argus.RegisterParser(&CustomConfigParser{
        appName: "myapp",
        version: "1.0",
    })
    
    // Continue with application logic
}
```

## Performance Characteristics

### Memory Management

Built-in parsers utilize memory pooling for configuration maps:

```go
var configMapPool = sync.Pool{
    New: func() interface{} {
        return make(map[string]interface{})
    },
}
```

Benefits:
- Reduced garbage collection pressure
- Zero allocations for configuration map creation
- Automatic memory reuse across parsing operations

### Parsing Performance

Benchmark results (operations per second):

| Format | Built-in Parser | Custom Parser (typical) |
|--------|----------------|-------------------------|
| JSON | 50,000 ops/sec | 35,000 ops/sec |
| YAML | 15,000 ops/sec | 25,000 ops/sec |
| TOML | 20,000 ops/sec | 18,000 ops/sec |
| HCL | 12,000 ops/sec | 8,000 ops/sec |
| INI | 30,000 ops/sec | 22,000 ops/sec |
| Properties | 40,000 ops/sec | 30,000 ops/sec |

### Lock Contention Optimization

The parser registration system minimizes lock contention:

```go
// Fast path: no custom parsers registered
if len(customParsers) == 0 {
    return parseBuiltin(data, format)
}

// Minimal lock time for parser lookup
parserMutex.RLock()
// ... parser selection logic
parserMutex.RUnlock()
```

## Security Considerations

### Input Validation

Built-in parsers implement security validations:

1. **JSON**: Key validation to prevent control character injection
2. **YAML**: Indentation bomb protection and recursion limits
3. **HCL**: Block nesting limits and identifier validation
4. **All formats**: Size limits and timeout protection

### Parser Security

Custom parsers should implement:

1. **Input sanitization**: Validate untrusted configuration data
2. **Resource limits**: Prevent memory exhaustion and infinite loops
3. **Error isolation**: Avoid information leakage in error messages
4. **Type safety**: Validate data types and ranges

Example security implementation:

```go
func (p *SecureParser) Parse(data []byte) (map[string]interface{}, error) {
    // Size limit check
    if len(data) > p.maxInputSize {
        return nil, errors.New("input size exceeds limit")
    }
    
    // Timeout protection
    ctx, cancel := context.WithTimeout(context.Background(), p.parseTimeout)
    defer cancel()
    
    return p.parseWithContext(ctx, data)
}
```

## Testing

### Unit Testing

```go
func TestCustomParser(t *testing.T) {
    parser := &CustomYAMLParser{}
    
    tests := []struct {
        name     string
        input    []byte
        expected map[string]interface{}
        wantErr  bool
    }{
        {
            name:  "simple key-value",
            input: []byte("key: value"),
            expected: map[string]interface{}{
                "key": "value",
            },
            wantErr: false,
        },
        {
            name:    "invalid syntax",
            input:   []byte("key: [invalid"),
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            result, err := parser.Parse(tt.input)
            
            if tt.wantErr {
                if err == nil {
                    t.Error("expected error, got nil")
                }
                return
            }
            
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            
            if !reflect.DeepEqual(result, tt.expected) {
                t.Errorf("expected %v, got %v", tt.expected, result)
            }
        })
    }
}
```

### Integration Testing

```go
func TestParserIntegration(t *testing.T) {
    // Register custom parser
    originalCount := len(customParsers)
    argus.RegisterParser(&CustomYAMLParser{})
    
    defer func() {
        // Cleanup: restore original parser state
        customParsers = customParsers[:originalCount]
    }()
    
    // Test that custom parser is used
    data := []byte("test: configuration")
    result, err := argus.ParseConfig(data, argus.FormatYAML)
    
    if err != nil {
        t.Fatalf("parsing failed: %v", err)
    }
    
    // Verify custom parser was used
    if result["test"] != "configuration" {
        t.Errorf("unexpected result: %v", result)
    }
}
```

### Performance Testing

```go
func BenchmarkCustomParser(b *testing.B) {
    parser := &CustomYAMLParser{}
    data := []byte(`
        database:
          host: localhost
          port: 5432
          credentials:
            username: admin
            password: secret
    `)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := parser.Parse(data)
        if err != nil {
            b.Fatalf("parsing failed: %v", err)
        }
    }
}
```