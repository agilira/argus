# Argus Overhead Benchmarks (Isolated)

This directory contains **isolated overhead benchmarks** for measuring Argus CPU usage and memory footprint without interference from other tests.

## Purpose

- **Realistic Overhead**: Precise measurement of Argus overhead in production scenarios
- **Minimal Overhead**: Theoretical minimum overhead measurement
- **Isolated Environment**: No test interference or shared resources

## Benchmarks

### `theoretical_minimal_overhead_test.go`
- **BenchmarkClean_MinimalOverhead**: Theoretical minimum overhead
- **Baseline**: Pure operation without any allocations
- **Result**: ~0.24 ns/op baseline, ~0.30 ns/op with Argus
- **Overhead**: +0.06 ns/op

### `realistic_production_overhead_test.go`  
- **BenchmarkArgus_PerLogEntryOverhead**: Realistic logging scenario
- **Baseline**: String concatenation with allocations (real-world)
- **Result**: ~44.74 ns/op baseline, ~51.34 ns/op with Argus
- **Overhead**: +6.6 ns/op

## Running

```bash
# Theoretical minimal overhead
go test -bench=BenchmarkClean_MinimalOverhead -benchmem -count=3

# Realistic production overhead
go test -bench=BenchmarkArgus_PerLogEntryOverhead -benchmem -count=3

# All overhead benchmarks
go test -bench=. -benchmem -count=3
```

## Results Interpretation

### Theoretical Minimal Overhead Results
- **Use case**: Proving theoretical efficiency
- **Context**: Best-case scenario overhead measurement
- **Expected**: Sub-nanosecond overhead

### Realistic Production Overhead Results
- **Use case**: Real-world production impact assessment
- **Context**: Typical logging operations with string processing
- **Expected**: <20% overhead on realistic workloads

## Professional Metrics

These benchmarks provide data for:
- **Production capacity planning** with realistic impact assessment  
- **Architecture decisions** based on measured performance cost
- **Competitive analysis** against other monitoring solutions

---

Argus • an AGILira fragment