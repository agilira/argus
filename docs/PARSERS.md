# Parser Plugin Development Guide

Argus includes built-in parsers optimized for the **80% use case**. For complex configurations requiring full specification compliance, develop custom parser plugins.

## Built-in Parser Capabilities

### Fully Compliant Parsers
- **JSON** - Complete RFC 7159 compliance
- **Properties** - Java-style key=value parsing  
- **INI** - Section-based configuration files

### Simplified Parsers (80% Use Case)
- **YAML** - Line-based parser for simple key-value configurations
- **TOML** - Basic parsing for standard use cases
- **HCL** - HashiCorp Configuration Language essentials

## Custom Parser Development

### Parser Interface

```go
type ConfigParser interface {
    // Parse converts configuration data to map[string]interface{}
    Parse(data []byte) (map[string]interface{}, error)
    
    // Supports returns true if this parser can handle the format
    Supports(format ConfigFormat) bool
    
    // Name returns parser identifier for debugging
    Name() string
}
```

### Example: Advanced YAML Parser

```go
package main

import (
    "github.com/agilira/argus"
    "gopkg.in/yaml.v3"
)

type AdvancedYAMLParser struct{}

func (p *AdvancedYAMLParser) Parse(data []byte) (map[string]interface{}, error) {
    var result map[string]interface{}
    err := yaml.Unmarshal(data, &result)
    return result, err
}

func (p *AdvancedYAMLParser) Supports(format argus.ConfigFormat) bool {
    return format == argus.FormatYAML
}

func (p *AdvancedYAMLParser) Name() string {
    return "AdvancedYAMLParser"
}

func init() {
    // Register custom parser - overrides built-in YAML parser
    argus.RegisterParser(&AdvancedYAMLParser{})
}
```

### Parser Registration

```go
// Register parser at package init
func init() {
    argus.RegisterParser(myParser)
}

// Register parser at runtime
argus.RegisterParser(&CustomParser{})
```

### Parser Priority

- **Custom parsers override built-in parsers** for the same format
- Registration order determines priority for conflicting formats
- Use `ConfigFormat` constants to ensure compatibility

## Testing Custom Parsers

```go
func TestCustomParser(t *testing.T) {
    parser := &MyCustomParser{}
    
    testData := []byte(`complex: configuration`)
    result, err := parser.Parse(testData)
    
    if err != nil {
        t.Fatalf("Parse failed: %v", err)
    }
    
    if !parser.Supports(argus.FormatYAML) {
        t.Error("Should support YAML format")
    }
}
```

## Best Practices

1. **Error Handling**: Return descriptive errors for debugging
2. **Format Detection**: Implement robust `Supports()` method
3. **Memory Efficiency**: Reuse objects where possible
4. **Thread Safety**: Parsers may be called concurrently
5. **Testing**: Include edge cases and malformed input

## Complete Example

See [examples/custom_parser/](../examples/custom_parser/) for a working implementation with:
- Custom YAML parser with advanced features
- Error handling patterns
- Integration testing
- Performance benchmarks
