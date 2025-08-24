# Argus Parser Plugin System

Argus provides a flexible parser plugin system that allows you to use production-ready parsers while maintaining zero dependencies for basic use cases.

## How It Works

Go compiles static binaries, so "plugins" work through **compile-time registration**:

### Method 1: Import-Based Auto-Registration (Recommended)

Third-party parser libraries can auto-register themselves when imported:

```go
package main

import (
    "github.com/agilira/argus"
    
    // These imports auto-register advanced parsers
    _ "github.com/your-org/argus-yaml-pro"   // Full YAML spec compliance
    _ "github.com/your-org/argus-toml-pro"   // Advanced TOML features  
    _ "github.com/your-org/argus-hcl-pro"    // Complete HCL support
)

func main() {
    // Argus automatically uses advanced parsers when available
    watcher, err := argus.UniversalConfigWatcher("config.yaml", func(config map[string]interface{}) {
        // This will use the advanced YAML parser if registered
        fmt.Printf("Config: %+v\n", config)
    })
}
```

### Method 2: Manual Registration

For explicit control, register parsers manually:

```go
package main

import "github.com/agilira/argus"

func main() {
    // Register your custom parser
    argus.RegisterParser(&MyAdvancedYAMLParser{})
    
    // Now Argus will use your parser for YAML files
    watcher, err := argus.UniversalConfigWatcher("config.yaml", handleConfig)
}
```

### Method 3: Build Tags (Advanced)

Use build tags for conditional compilation:

```go
//go:build yaml_pro
// +build yaml_pro

package main

import _ "github.com/your-org/argus-yaml-pro"
```

Build with: `go build -tags "yaml_pro,toml_pro" ./...`

## Creating Custom Parsers

Implement the `ConfigParser` interface:

```go
package main

import "github.com/agilira/argus"

type MyAdvancedYAMLParser struct{}

func (p *MyAdvancedYAMLParser) Parse(data []byte) (map[string]interface{}, error) {
    // Use your favorite YAML library here
    // e.g., gopkg.in/yaml.v3, github.com/goccy/go-yaml
    var result map[string]interface{}
    err := yaml.Unmarshal(data, &result)
    return result, err
}

func (p *MyAdvancedYAMLParser) Supports(format argus.ConfigFormat) bool {
    return format == argus.FormatYAML
}

func (p *MyAdvancedYAMLParser) Name() string {
    return "Advanced YAML Parser"
}

// Auto-register in init() for import-based registration
func init() {
    argus.RegisterParser(&MyAdvancedYAMLParser{})
}
```

## Parser Priority

1. **Custom registered parsers** (tried first)
2. **Built-in lightweight parsers** (fallback)

This ensures production features when needed, zero dependencies by default.

## Built-in vs Production Parsers

| Format | Built-in Parser | Production Parser Benefits |
|--------|----------------|---------------------------|
| JSON | Full support | Better error messages |
| YAML | Simple key:value | Full spec, anchors, multi-doc |
| TOML | Basic parsing | Complete TOML spec |
| HCL | Key=value only | Terraform compatibility |
| INI | Flat key=value | Section support, includes |
| Properties | Java-style | Advanced escaping, Unicode |

## Example: YAML Evolution

**Built-in** (zero dependencies):
```yaml
key: value
port: 8080
debug: true
```

**Production** (with gopkg.in/yaml.v3):
```yaml
app: &app
  name: myapp
  version: 1.0

environments:
  dev:
    <<: *app
    debug: true
  prod:
    <<: *app
    debug: false
```

## Best Practices

1. **Start simple**: Use built-in parsers for development
2. **Add production parsers**: When you need advanced features
3. **Use import-based registration**: For clean dependency management
4. **Test both paths**: Ensure fallback works without custom parsers

## Available Third-Party Parsers

*Note: These are examples - actual packages would be maintained by the community*

- `github.com/argus-plugins/yaml-pro` - Full YAML spec with gopkg.in/yaml.v3
- `github.com/argus-plugins/toml-pro` - Complete TOML with github.com/BurntSushi/toml
- `github.com/argus-plugins/hcl-pro` - Terraform HCL with github.com/hashicorp/hcl/v2

## Debug: List Registered Parsers

```go
parsers := argus.ListRegisteredParsers()
fmt.Printf("Registered parsers: %v\n", parsers)
```
---

Argus • an AGILira fragment
