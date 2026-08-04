# Runtime OTLP receiver

The `collect` command turns AI Evidence BOM into a small dual-protocol OTLP trace backend. Accepted spans are normalized and merged into a durable evidence graph and optional CycloneDX snapshots. A child whose parent has not arrived yet waits in a bounded metadata-only queue so identity can be correlated across export requests. JSON and protobuf inputs share one metadata allowlist and produce the same graph contract.

## Start locally

```bash
go build -o ./bin/aiebom ./cmd/aiebom

./bin/aiebom collect \
  --listen 127.0.0.1:4318 \
  --grpc-listen 127.0.0.1:4317 \
  --graph-out work/live.evidence.json \
  --bom-out work/live.cdx.json
```

The default trace endpoints are:

| Transport | Endpoint | Payload |
|---|---|---|
| OTLP/HTTP | `POST http://127.0.0.1:4318/v1/traces` | `application/json` or `application/x-protobuf` |
| OTLP/gRPC | `127.0.0.1:4317` | `opentelemetry.proto.collector.trace.v1.TraceService/Export` |

HTTP responses use the request's representation: JSON requests receive an empty JSON `ExportTraceServiceResponse` (`{}`), and protobuf requests receive its empty binary encoding. HTTP JSON and protobuf requests may use `Content-Encoding: gzip`. The gRPC server supports the compression negotiated by the client library.

Set `--listen ''` or `--grpc-listen ''` to disable one transport. At least one listener is required.

## Connect an OpenTelemetry Collector

Create a token file and start the receiver:

```bash
umask 077
printf '%s\n' 'replace-with-a-long-random-token' > work/receiver.token
export AIEBOM_TOKEN="$(tr -d '\r\n' < work/receiver.token)"

./bin/aiebom collect \
  --auth-token-file work/receiver.token \
  --graph-out work/live.evidence.json \
  --bom-out work/live.cdx.json
```

Then start a Collector with either:

```bash
otelcol --config examples/otel-collector-http.yaml
# or
otelcol --config examples/otel-collector-grpc.yaml
```

The HTTP example uses the Collector's current `otlp_http` component name, binary protobuf encoding, and gzip. The gRPC example uses `otlp_grpc` and explicitly disables client TLS because the built-in local receiver is plaintext. The older `otlphttp` alias is deprecated by the Collector project.

The example Collector listens for application telemetry on local ports 14317 and 14318 so it does not collide with AI Evidence BOM's ports. Point an instrumented application at either Collector input, not directly at both.

When the Collector runs in a container, replace `127.0.0.1` in the exporter endpoint with a reachable host name such as `host.docker.internal`. AI Evidence BOM must then bind to the corresponding non-loopback interface and requires `--auth-token-file`.

## Read live state

| Endpoint | Purpose |
|---|---|
| `GET /healthz` | Liveness and schema version; intentionally public. |
| `GET /v1/evidence` | Current evidence graph. |
| `GET /v1/bom` | Current CycloneDX export. |
| `GET /v1/stats` | Request, accepted span, duplicate, pending-span, and failure counters. |

## Authentication and network exposure

Loopback is the safe default. Binding either listener to a non-loopback address is rejected unless `--auth-token-file` is supplied:

```bash
./bin/aiebom collect \
  --listen 0.0.0.0:4318 \
  --grpc-listen 0.0.0.0:4317 \
  --auth-token-file /secure/path/receiver.token \
  --graph-out work/live.evidence.json
```

HTTP clients send `Authorization: Bearer <token>`. gRPC clients send the same value as the `authorization` metadata entry. The evidence, BOM, and stats endpoints require the token; health remains unauthenticated.

The built-in servers do not terminate TLS. Plaintext is intended only for loopback or a protected sidecar network. Terminate both HTTP and gRPC TLS at a trusted proxy for any remote or shared-network deployment.

## Resource and retry controls

- `--max-request-bytes` defaults to 64 MiB. It applies before and after HTTP gzip decompression and to each received gRPC message.
- `--max-dedupe-items` defaults to 100,000 recent trace/span pairs.
- The same limit bounds unresolved child spans and retained trace-context entries. If the pending queue reaches the limit, the oldest unresolved metadata is normalized without parent identity rather than growing memory without bound.
- A child whose parent never arrives remains visible in the `pendingSpans` gauge until it is released by its parent or by queue pressure. Pending context is in memory and resets on restart.
- Duplicate span retries are acknowledged but do not increase evidence counts, even when a retry changes transport.
- Deduplication state is in memory and resets on restart.
- An existing graph at `--graph-out` is loaded so evidence history continues after restart.
- Raw OTLP requests are never written to disk.

## Protocol scope

v0.6 accepts OTLP trace `ExportTraceServiceRequest` messages over HTTP/JSON, HTTP/protobuf, and gRPC/protobuf. Unknown protobuf fields are discarded for forward compatibility. Resource attributes, instrumentation scope provenance, span attributes, names, timestamps, trace IDs, span IDs, and parent span IDs pass through one normalizer.

Metrics, logs, and profiles are intentionally not registered or exposed. Partial-success responses are not currently generated: a syntactically valid request is accepted as a whole, while malformed input or persistence failure returns the appropriate HTTP status or gRPC status code.
