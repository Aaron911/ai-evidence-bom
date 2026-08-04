# Changelog

All notable changes to this experimental project are documented here.

## [Unreleased]

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

[Unreleased]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Aaron911/ai-evidence-bom/releases/tag/v0.1.0
