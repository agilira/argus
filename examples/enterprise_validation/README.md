
# Enterprise Validation Example

This example demonstrates advanced configuration validation using Argus.

## Features Demonstrated
- Detailed configuration validation
- Error and warning reporting
- Performance recommendations
- Environment variable validation
- Audit and file path verification

## How to Run
```bash
cd examples/enterprise_validation
go run main.go
```

## What the Example Does
1. **Validates a correct configuration**: shows no errors.
2. **Validates an incorrect configuration**: shows detailed errors and warnings.
3. **Configuration with performance warnings**: highlights possible optimizations.
4. **Environment variable validation**: checks if environment-based config is valid.
5. **Quick validation**: shows only blocking errors.

## Example Output
```
=== Argus 2.0 Enterprise Configuration Validation Demo ===
1. Testing VALID configuration:
   Valid: true
   Errors: 0
   Warnings: 0

2. Testing INVALID configuration:
   Valid: false
   Errors: 5
   Warnings: 1
   🚫 Errors found:
      1. ...
   ⚠️  Warnings found:
      1. ...

3. Testing configuration with PERFORMANCE warnings:
   Valid: true
   Errors: 0
   Warnings: 2
   ⚠️  Performance warnings:
      1. ...
      2. ...

4. Testing ENVIRONMENT configuration validation:
   ✅ Environment configuration is valid

5. Quick validation (errors only):
   ❌ Validation error: ...

=== Enterprise-Grade Validation Complete ===
```

## Further Reading
- [Argus Documentation](../../docs/CONFIG_BINDING.md)
- [API Reference](../../docs/API.md)
