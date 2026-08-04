# Runtime OTLP/HTTP receiver

The `collect` command turns AI Evidence BOM into a small OTLP/HTTP JSON backend. Each accepted request is normalized immediately and merged into durable evidence graph and optional CycloneDX snapshots.

## Start locally

```bash
go build -o ./bin/aiebom ./cmd/aiebom

./bin/aiebom collect \
  --listen 127.0.0.1:4318 \
  --graph-out work/live.evidence.json \
  --bom-out work/live.cdx.json
```

The default OTLP/HTTP trace endpoint is:

```text
POST http://127.0.0.1:4318/v1/traces
Content-Type: application/json
```

An OTLP success response is HTTP 200 with an empty JSON `ExportTraceServiceResponse` (`{}`). JSON requests may use `Content-Encoding: gzip`.

## Read live state

| Endpoint | Purpose |
|---|---|
| `GET /healthz` | Liveness and schema version; intentionally public. |
| `GET /v1/evidence` | Current evidence graph. |
| `GET /v1/bom` | Current CycloneDX export. |
| `GET /v1/stats` | Request, accepted span, duplicate, and failure counters. |

## Authentication and network exposure

Loopback is the safe default. Binding to a non-loopback address is rejected unless `--auth-token-file` is supplied:

```bash
./bin/aiebom collect \
  --listen 0.0.0.0:4318 \
  --auth-token-file /secure/path/receiver.token \
  --graph-out work/live.evidence.json
```

Clients must then send `Authorization: Bearer <token>` to the trace and read endpoints. The health endpoint remains unauthenticated. The built-in server does not terminate TLS, so place it behind a trusted TLS reverse proxy for any remote or shared-network deployment.

## Resource and retry controls

- `--max-request-bytes` defaults to 64 MiB and applies both before and after gzip decompression.
- `--max-dedupe-items` defaults to 100,000 recent trace/span pairs.
- Duplicate span retries are acknowledged but do not increase evidence counts.
- Deduplication state is in memory and resets on restart.
- An existing graph at `--graph-out` is loaded so evidence history continues after restart.
- Raw OTLP requests are never written to disk.

## Protocol scope

v0.2 accepts the OTLP JSON mapping for `ExportTraceServiceRequest`. It intentionally rejects `application/x-protobuf`, OTLP/gRPC, metrics, logs, and profiles. Many SDKs and OpenTelemetry Collector exporters use binary protobuf by default; they cannot yet point directly at this receiver without a JSON-capable adapter.

Binary protobuf and a first-class Collector integration are the next protocol milestone. The current JSON endpoint is suitable for contract testing, custom adapters, sanitized fixtures, and integrations that can emit OTLP JSON explicitly.
