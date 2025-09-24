# Custom Parser Example

This example demonstrates how to implement and register a custom configuration parser with Argus.

## Features
- Custom parser registration
- Integration with Argus configuration watcher
- Error handling and live reload demonstration

## How to Run
```bash
cd examples/custom_parser
go run main.go
```

## What the Example Does
1. **Parses a YAML config file** using the built-in parser.
2. **Registers a custom YAML parser** and parses the same file again, showing extended metadata.
3. **Compares built-in and custom parser behaviors**.
4. **Demonstrates live reload** by updating the config file and observing changes.

## Key Concepts
- **Custom parser registration**: Extend Argus by implementing the `ConfigParser` interface and registering your parser.
- **Minimal dependencies**: The built-in parser is simple; custom parsers can add advanced features as needed.
- **Live reload**: Argus automatically reloads configuration when the file changes.

## Example Output
```
Argus Custom Parser Example
============================
Step 1: Parsing with built-in parser
   (Simple, minimal dependencies)
Step 2: Registering custom parser
   (Demonstrates extensibility and advanced features)
   Custom parser registered.
   Custom parser result:
      ...
Step 3: Key differences
   Built-in: Fast, simple, minimal dependencies
   Custom:   Full spec compliance, advanced features
   Priority: Custom parsers are tried first, built-in as fallback
Step 4: Testing live reload
Demo completed.
Key points:
   • Import-based registration: import _ "github.com/your-org/argus-yaml-pro"
   • Manual registration: argus.RegisterParser(&MyParser{})
   • Build tags: go build -tags "yaml_pro,toml_pro"
   • Minimal dependencies by default; advanced features available via custom parsers.
```

## Further Reading
- [Parsers](../../docs/PARSERS.md)
- [Parser Plugins](../../docs/PARSER_PLUGINS.md)

---

Argus • an AGILira fragment
