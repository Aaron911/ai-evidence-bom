# Changelog

All notable changes to this experimental project are documented here.

## [Unreleased]

## [0.3.0] - 2026-08-04

### Added

- OTLP/HTTP binary protobuf ingestion at `/v1/traces` with protocol-matched success and error responses.
- OTLP/gRPC `TraceService/Export` ingestion on the standard local port 4317.
- Shared normalization for OTLP JSON and protobuf resource, scope, span, identifier, timestamp, and attribute data.
- Bearer-token authentication, retry deduplication, and metadata-only filtering across both transports.
- Graceful dual-server startup and shutdown with independently disableable HTTP and gRPC listeners.
- Ready-to-use OpenTelemetry Collector exporter examples for `otlp_http` and `otlp_grpc`.

### Security

- The same allowlist controls JSON and protobuf output; raw messages, prompts, model responses, tool arguments, and tool results are not persisted.
- Non-loopback HTTP or gRPC listeners require a bearer-token file.
- The minimum Go patch release is 1.26.5 so release builds include current standard-library security fixes.
- CI now rejects reachable Go standard-library and dependency vulnerabilities with `govulncheck`.

### Known gaps

- The receiver accepts traces only; OTLP metrics, logs, and profiles are not supported.
- Built-in TLS and persisted retry-deduplication state are not yet provided.

## [0.2.0] - 2026-08-04

### Added

- Live OTLP/HTTP JSON trace receiver at `/v1/traces`.
- Continuously merged evidence graph and CycloneDX snapshots.
- Atomic snapshot replacement and restart from an existing graph.
- Bounded request handling before and after gzip decompression.
- Optional bearer-token authentication and safe loopback default.
- Bounded retry deduplication by trace and span ID.
- Live evidence, BOM, health, and receiver-stat endpoints.
- OpenTelemetry instrumentation scope and schema provenance.
- Fallback normalization for standard `invoke_agent` and `execute_tool` span names.
- Installable Go module path at `github.com/Aaron911/ai-evidence-bom`.

### Security

- Raw live OTLP payloads are processed in memory and never written to disk.
- Sensitive GenAI message, tool argument, and tool result attributes remain excluded from output.

### Known gaps

- The live receiver does not yet accept OTLP binary protobuf or OTLP/gRPC.
- Deduplication state is not persisted across restarts.

## [0.1.0] - 2026-08-04

- Initial evidence graph, OTLP JSON file scan, CycloneDX export, diff, policy, prompt HMAC, and Ed25519 signing prototype.

[Unreleased]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Aaron911/ai-evidence-bom/releases/tag/v0.1.0
