# Changelog

All notable changes to this experimental project are documented here.

## [Unreleased]

### Added

- Added `sign --canonical-evidence` for reproducible evidence-graph signatures across JSON whitespace, object-member order, graph collection order, and equivalent timestamp offsets.
- Added a versioned signature-envelope profile that records the canonical payload type and RFC 8785-based canonicalization mode; `verify` selects the declared mode without guessing.

### Security

- Canonical evidence input rejects duplicate JSON members, duplicate graph identities, unknown graph fields, invalid UTF-8, and trailing JSON values before signing.
- Canonical signatures use a mode-specific domain prefix, preventing a canonical v0.2 signature from being interpreted as a legacy raw-byte v0.1 signature.
- Legacy exact-byte signing and verification remain backward compatible; canonical signing does not broaden content collection or provide confidentiality.

### Validation

- Added a CI gate that exports a real BOM, verifies the checksum-pinned official CycloneDX 1.7 JSON Schema, and validates the output against it.
- Added a negative control proving that a deliberately invalid `bomFormat` is rejected, so a no-op validator cannot satisfy the gate.
- Migrated BOM generator metadata from the deprecated legacy `metadata.tools` array to the CycloneDX 1.7 `metadata.tools.components` shape.

### Documentation

- Added an evidence-backed OpenTelemetry GenAI proposal for aligning MCP semantic conventions with the `2026-07-28` stateless lifecycle and protocol-reported server metadata.
- Submitted the proposal as [`open-telemetry/semantic-conventions-genai#437`](https://github.com/open-telemetry/semantic-conventions-genai/issues/437); submission is not acceptance, and no new standard compatibility claim or project alias is introduced.

## [0.7.0] - 2026-08-05

### Added

- A deterministic official MCP Go SDK v1.7.0 client/server live check over stdio, exercising `server/discover`, `tools/list`, and `tools/call` on protocol `2026-07-28`.
- Protocol-derived MCP server identity and content-free declared capability evidence using an explicitly namespaced `aiebom.mcp.*` bridge.
- Bounded directed graph-path policies with typed/evidence-aware node selectors and exact relationship sequences.
- Capability-drift coverage proving that an added but uninvoked `shell.execute` tool is detected by diff and policy without an `invokes` claim.

### Changed

- MCP relationships now use the directed `mcp_server -[provides]-> tool` form, enabling `agent -[connects_to]-> mcp_server -[provides]-> tool` policy paths.
- OTLP evidence defaults to observed but may be downgraded to declared or inferred by `aiebom.evidence.level`; telemetry cannot self-promote to verified.
- MCP standard attributes and project identity extensions are documented separately; legacy `mcp.server.*` aliases remain compatibility inputs only.

### Security

- MCP descriptions, raw schemas, arguments, and results remain excluded from evidence; the live check fails on marker leakage.
- Input-schema bodies are reduced to SHA-256 drift fingerprints, and tool annotation hints are labeled untrusted.
- Path-policy traversal is bounded to eight relationships and reports the exact matching node/relation path.

### Known gaps

- The official MCP SDK does not emit OTLP automatically; v0.7 validates application instrumentation around real SDK calls.
- Current OTel MCP conventions do not define stable logical server identity and lag the MCP `2026-07-28` lifecycle; `aiebom.mcp.server.*` is a project bridge, not a standard claim.
- Server-side spans, cross-process trace propagation, remote HTTP transports, a second SDK, and production workloads remain unvalidated.

## [0.6.0] - 2026-08-04

### Added

- A complete, deterministic Dify 1.16.1 application-runtime compatibility gate using its official API, plugin daemon, database, Redis, workflow import/publish path, OpenAI plugin, built-in tool, and supported OTLP exporter.
- A checksum-pinned local-package installation path for the official Dify OpenAI plugin, avoiding plugin-container Marketplace DNS as an unrelated source of test nondeterminism.
- Weekly and path-triggered GitHub Actions coverage for the heavier full-runtime check.
- A reproducible evidence record covering both the initial failed setup and the corrected passing run.

### Changed

- Dify compatibility is upgraded from instrumentation execution to live capture for the tested complete workflow path.
- The Phase 1 core exit gate is marked complete: two unrelated framework runtimes now produce the same core agent/model/tool graph semantics over standard OTLP.
- Cleanup of live validation processes and containers is bounded so setup failures return evidence instead of consuming the entire job timeout.

### Security

- The full Dify workflow verifies that system prompt, input, model output, tool argument, and tool output markers do not reach the evidence graph or CycloneDX BOM.
- The pinned Dify plugin package must match its recorded SHA-256 before installation.

### Known gaps

- Dify live capture covers one deterministic LLM-plus-tool workflow, not every node, provider, deployment mode, or production limit.
- Dify telemetry must be explicitly enabled and configured; uninstrumented Agent applications still emit no standard telemetry.
- Stable retrieval data-source identity, MCP server identity, and multi-agent handoff remain unvalidated.

## [0.5.0] - 2026-08-04

### Added

- A deterministic Microsoft Agent Framework live-capture check using released `agent-framework-core==1.13.0`, real Agent/chat/tool telemetry paths, and OTLP/HTTP protobuf export.
- A pinned Dify 1.16.1 instrumentation-execution check covering its real workflow handler, LLM parser, tool parser, and OTLP exporter without claiming a full Dify runtime.
- Bounded cross-request trace-context correlation for child spans that arrive before their parent.
- A `pendingSpans` live-receiver statistic and regression coverage for child-first export ordering.
- A reproducible framework validation record with explicit evidence grades and limitations.

### Changed

- Live receiver observations are reduced to a metadata allowlist before they can enter the pending correlation queue.
- Prompt presence and optional keyed HMAC survive delayed normalization without retaining prompt content.
- Compatibility claims now distinguish source contract, instrumentation execution, live capture, and production validation.

### Security

- Cross-batch correlation removes prompt/input/output content and tool arguments/results before retaining unresolved child metadata.
- Both executable framework checks fail if any sensitive marker reaches the evidence graph or CycloneDX BOM.

### Known gaps

- Complete Dify application-runtime capture is still unverified; the local validation executes its instrumentation modules with deterministic host stubs.
- A child whose parent is missing remains pending until its parent arrives or the bounded queue applies pressure; pending context is not persisted across restarts.
- Microsoft provider integrations, multi-agent handoff, stable retrieval data-source identity, and stable MCP server identity remain unvalidated.

## [0.4.0] - 2026-08-04

### Added

- OTLP `parentSpanId` preservation for JSON, compact observation, and protobuf inputs.
- Trace-context normalization that carries stable agent identity to descendant model and tool spans.
- Source-derived, sanitized OTLP compatibility fixtures for Dify and Microsoft Agent Framework.
- Cross-framework contract tests proving equivalent core agent, model, tool, and relationship semantics.
- An evidence-graded compatibility matrix and a durable release direction-calibration record.

### Changed

- A concrete model child span now supersedes model summary attributes on an ancestor `invoke_agent` span, preventing duplicate model nodes and framework-provider misclassification.
- `dify.app_id` is used as stable agent identity; host `service.version` metadata is no longer misclassified as an explicitly identified agent version.
- Compatibility claims now distinguish source-contract validation from live-capture and production validation.

### Security

- Framework fixtures include sensitive marker values, and tests prove that prompt/input/output bodies and tool arguments/results do not leak into the graph.

### Known gaps

- The framework fixtures are derived from pinned upstream source contracts, not captured from running framework installations.
- Stable Dify retrieval data-source identity, MCP server identity, and multi-agent handoff semantics still require live validation.

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

[Unreleased]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Aaron911/ai-evidence-bom/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Aaron911/ai-evidence-bom/releases/tag/v0.1.0
