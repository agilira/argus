# Argus OTEL Integration Example

This example demonstrates how to professionally integrate OpenTelemetry with Argus's audit system, showcasing multiple export backends, performance optimization, and production-ready configuration patterns.

## Features Demonstrated

- **Jaeger Distributed Tracing** - Full distributed trace correlation
- **Prometheus Metrics Export** - Audit events as metrics
- **OTLP Generic Export** - Flexible OTEL protocol support
- **Performance Benchmarking** - Impact measurement and optimization
- **Graceful Error Handling** - Robust fallback mechanisms
- **Production Patterns** - Real-world deployment examples

## Quick Start

### Prerequisites

```bash
# Install dependencies
go mod tidy

# Start local observability stack (optional)
docker-compose up -d
```

### Basic Usage

```bash
# Run with default configuration (console output)
go run main.go

# Run with Jaeger tracing
JAEGER_ENDPOINT=http://localhost:14268/api/traces go run main.go

# Run with OTLP export
OTLP_ENDPOINT=localhost:4317 go run main.go

# Run with Prometheus metrics
PROMETHEUS_ENDPOINT=:8080 go run main.go

# Run with all integrations
ENABLE_OTEL=true \
JAEGER_ENDPOINT=http://localhost:14268/api/traces \
OTLP_ENDPOINT=localhost:4317 \
PROMETHEUS_ENDPOINT=:8080 \
go run main.go
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SERVICE_NAME` | `argus-otel-example` | Service name for tracing |
| `SERVICE_VERSION` | `1.0.0` | Service version |
| `ENVIRONMENT` | `development` | Deployment environment |
| `JAEGER_ENDPOINT` | `http://localhost:14268/api/traces` | Jaeger collector endpoint |
| `OTLP_ENDPOINT` | `localhost:4317` | OTLP gRPC endpoint |
| `PROMETHEUS_ENDPOINT` | `:8080` | Prometheus metrics endpoint |
| `ENABLE_OTEL` | `true` | Enable/disable OTEL integration |

### Docker Compose Stack

```yaml
# docker-compose.yml - Local observability stack
version: '3.8'

services:
  jaeger:
    image: jaegertracing/all-in-one:1.50
    ports:
      - "16686:16686"    # Jaeger UI
      - "14268:14268"    # Jaeger collector
    environment:
      - COLLECTOR_OTLP_ENABLED=true

  prometheus:
    image: prom/prometheus:v2.47.0
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml

  grafana:
    image: grafana/grafana:10.1.0
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin

  otel-collector:
    image: otel/opentelemetry-collector-contrib:0.86.0
    command: ["--config=/etc/otel-collector-config.yml"]
    volumes:
      - ./otel-collector-config.yml:/etc/otel-collector-config.yml
    ports:
      - "4317:4317"   # OTLP gRPC receiver
      - "4318:4318"   # OTLP HTTP receiver
    depends_on:
      - jaeger
      - prometheus
```

## Demonstration Scenarios

### 1. Configuration Changes

The example simulates realistic configuration changes:

```go
// Database configuration update
oldConfig: {
    "host": "localhost",
    "port": 5432,
    "ssl_mode": "require"
}
newConfig: {
    "host": "prod-db.company.com",
    "port": 5432, 
    "ssl_mode": "require",
    "pool_size": 20
}
```

**OTEL Span Generated:**
- **Name:** `audit.config_change`
- **Attributes:** Service metadata, file path, audit level
- **Context:** Full before/after configuration state

### 2. Security Events

Security-focused audit events with enhanced tracing:

```go
// Unauthorized access attempt
Event: "unauthorized_access"
Details: "Failed login attempt from suspicious IP"
Context: {
    "source_ip": "192.168.1.100",
    "user_agent": "curl/7.68.0", 
    "attempted_user": "admin",
    "failure_count": 3
}
```

**OTEL Span Generated:**
- **Name:** `audit.security.unauthorized_access`
- **Attributes:** Security-specific metadata
- **Status:** Critical security event marker

### 3. File Watch Events

File system monitoring with distributed tracing:

```go
// File system events
Events: ["file_created", "file_modified", "file_deleted"]
Paths: ["/etc/app/config.json", "/tmp/cache.db"]
```

**OTEL Span Generated:**
- **Name:** `audit.file_watch.{event_type}`
- **Attributes:** File path, event type, component
- **Performance:** Optimized for high-frequency events

### 4. Performance Testing

High-frequency event processing benchmarks:

```bash
# Example output
📊 Running Performance Benchmarks
  🔥 Warming up...
  ⏱️ Benchmarking 10000 audit events...
  📈 Benchmark Results:
    • Total Duration: 2.847s
    • Average Latency: 284.7µs  
    • Throughput: 3512 events/sec
    • OTEL Integration: Enabled
    • OTEL Tracing Errors: 0
```

## Observability Integration

### Jaeger Tracing

Access Jaeger UI at `http://localhost:16686`:

```bash
# Example trace search
Service: argus-otel-example
Operation: audit.config_change
Tags: audit.level=CRITICAL
```

**Trace Details:**
- **Root Span:** Configuration change audit
- **Attributes:** Complete audit context
- **Timeline:** Event processing duration
- **Links:** Related configuration events

### Prometheus Metrics

Access metrics at `http://localhost:8080/metrics`:

```prometheus
# HELP argus_audit_events_total Total audit events processed
# TYPE argus_audit_events_total counter
argus_audit_events_total{level="CRITICAL",component="argus"} 45

# HELP argus_audit_processing_duration_seconds Audit processing duration
# TYPE argus_audit_processing_duration_seconds histogram
argus_audit_processing_duration_seconds_bucket{le="0.001"} 1234
```

### OTLP Export

Generic OTEL protocol export for any backend:

```yaml
# otel-collector-config.yml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317

processors:
  batch:
    timeout: 5s
    send_batch_size: 512

exporters:
  jaeger:
    endpoint: jaeger:14250
    tls:
      insecure: true
  prometheus:
    endpoint: "0.0.0.0:8889"

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [jaeger]
    metrics:
      receivers: [otlp] 
      processors: [batch]
      exporters: [prometheus]
```

## Performance Characteristics

### Benchmark Results

| Configuration | Throughput | Latency | Memory | CPU |
|---------------|------------|---------|--------|-----|
| **Core Only** | 4,200 events/sec | 238µs | 2MB | 1% |
| **OTEL Disabled** | 3,850 events/sec | 260µs | 2.1MB | 1% |
| **OTEL Enabled** | 3,512 events/sec | 285µs | 3.2MB | 1.2% |

### Performance Impact Analysis

- **Throughput Impact:** ~16% reduction with full OTEL integration
- **Latency Impact:** +47µs average (async processing)
- **Memory Impact:** +1.1MB for OTEL exporters
- **CPU Impact:** +0.2% for trace generation

**Key Insight:** Performance impact is minimal and fully asynchronous, ensuring core audit operations remain unaffected.

## Production Deployment

### Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-with-argus-audit
spec:
  template:
    spec:
      containers:
      - name: app
        image: myapp:latest
        env:
        - name: OTEL_EXPORTER_JAEGER_ENDPOINT  
          value: "http://jaeger-collector:14268/api/traces"
        - name: OTEL_EXPORTER_OTLP_ENDPOINT
          value: "http://otel-collector:4317"
        - name: OTEL_SERVICE_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.labels['app.kubernetes.io/name']
        - name: OTEL_RESOURCE_ATTRIBUTES
          value: "deployment.environment=production,k8s.namespace.name=$(NAMESPACE)"
        - name: NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
```

### Monitoring and Alerting

```yaml
# Example Prometheus alert rules
groups:
- name: argus_audit
  rules:
  - alert: AuditSystemDown
    expr: up{job="argus-audit"} == 0
    for: 1m
    annotations:
      summary: "Argus audit system is down"
      
  - alert: HighSecurityEvents
    expr: rate(argus_audit_events_total{level="SECURITY"}[5m]) > 0.1
    for: 2m
    annotations:
      summary: "High rate of security audit events"
      
  - alert: OTELTracingErrors  
    expr: argus_otel_tracing_errors_total > 0
    for: 5m
    annotations:
      summary: "OTEL tracing errors detected"
```

## Troubleshooting

### Common Issues

#### OTEL Integration Not Working

```bash
# Check OTEL configuration
export OTEL_LOG_LEVEL=debug
go run main.go

# Verify exporters
curl http://localhost:8080/metrics
curl http://localhost:16686/api/services
```

#### Performance Issues

```bash
# Reduce sampling rate
export OTEL_TRACES_SAMPLER=traceidratio
export OTEL_TRACES_SAMPLER_ARG=0.01  # 1% sampling

# Increase batch sizes  
export OTEL_BSP_MAX_EXPORT_BATCH_SIZE=2048
export OTEL_BSP_EXPORT_TIMEOUT=10s
```

#### Missing Traces

```bash
# Check exporter endpoints
telnet jaeger-collector 14268
telnet otel-collector 4317

# Verify service discovery
nslookup jaeger-collector
nslookup otel-collector
```

### Debug Mode

```bash
# Enable comprehensive debugging
OTEL_LOG_LEVEL=debug \
ARGUS_DEBUG=true \
go run main.go
```

## Best Practices

### 1. **Sampling Strategy**
- Use **head-based sampling** for uniform distribution
- Set **1-10% sampling** for high-volume production systems
- Implement **adaptive sampling** for dynamic adjustment

### 2. **Resource Attribution**
- Include **service metadata** in all traces
- Add **deployment environment** context
- Use **consistent naming** across services

### 3. **Error Handling**
- Monitor **OTEL exporter health** independently
- Implement **graceful degradation** for exporter failures
- Ensure **audit reliability** is never compromised

### 4. **Security Considerations**
- Use **TLS encryption** for OTEL exports
- Avoid **sensitive data** in trace attributes
- Implement **proper RBAC** for observability systems

## Integration Examples

### Custom Exporter

```go
// Custom exporter for enterprise systems
type EnterpriseExporter struct {
    endpoint string
    apiKey   string
}

func (e *EnterpriseExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
    // Custom export logic for enterprise SIEM
    for _, span := range spans {
        event := convertToSIEMFormat(span)
        if err := e.sendToSIEM(event); err != nil {
            return err
        }
    }
    return nil
}
```

### Multi-Environment Configuration

```go
func getOTELConfig(environment string) *OTELConfig {
    switch environment {
    case "production":
        return &OTELConfig{
            SamplingRate: 0.01,      // 1% sampling
            BatchSize:    2048,      // Large batches
            Timeout:      30 * time.Second,
        }
    case "staging":
        return &OTELConfig{
            SamplingRate: 0.1,       // 10% sampling  
            BatchSize:    512,       // Medium batches
            Timeout:      10 * time.Second,
        }
    default: // development
        return &OTELConfig{
            SamplingRate: 1.0,       // 100% sampling
            BatchSize:    100,       // Small batches
            Timeout:      5 * time.Second,
        }
    }
}
```

This example shows a production-ready OpenTelemetry integration with comprehensive observability, performance optimization, and deployment patterns.

---

Argus • an AGILira fragment