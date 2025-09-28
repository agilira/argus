# Argus Optimization Strategies Example

This example demonstrates the four optimization strategies available in Argus for different file monitoring scenarios. The example has been fully translated from Italian to professional English and includes comprehensive testing and benchmarking.

## Optimization Strategies

### 1. SingleEvent Strategy
**Best for:** 1-2 critical configuration files  
**Characteristics:** Ultra-low latency, immediate response  
**Use cases:** Critical system configuration, security settings

### 2. SmallBatch Strategy  
**Best for:** 3-20 configuration files  
**Characteristics:** Balanced performance, efficient for moderate loads  
**Use cases:** Microservices configurations, service discovery

### 3. LargeBatch Strategy
**Best for:** 20+ configuration files  
**Characteristics:** High throughput, optimized for bulk operations  
**Use cases:** Container orchestrators, large distributed systems

### 4. Auto Strategy
**Best for:** Dynamic workloads  
**Characteristics:** Automatically adapts between strategies based on file count  
**Use cases:** Development environments, varying workloads

## Files Structure

- `main.go` - Demonstration of all four optimization strategies
- `main_test.go` - Unit tests for each strategy with performance validation
- `integration_test.go` - Integration tests for real-world scenarios
- `benchmark_test.go` - Performance benchmarks with detection rate calculations

## Running the Example

```bash
# Run the demonstration
go run main.go

# Run all tests
go test -v

# Run benchmarks
go test -bench=. -benchmem
```