# Validation roadmap

This roadmap is organized around falsifiable adoption gates rather than a promise to build a full platform.

## Phase 0 — executable prototype

- [x] OTLP JSON and compact observation input
- [x] vendor-neutral evidence graph
- [x] CycloneDX 1.7 export
- [x] graph diff and policy gate
- [x] metadata-only privacy default
- [x] Ed25519 signing
- [x] unit tests and reproducible examples

## Phase 1 — real telemetry adapters

- [x] Consume OTLP/HTTP JSON directly with gzip, bounded requests, persistence, and retry deduplication
- [x] Accept OTLP/HTTP binary protobuf and OTLP/gRPC with matching authentication and privacy behavior
- [ ] Validate two independent frameworks, initially Dify and one code-first agent framework
- [x] Normalize the core current OpenTelemetry GenAI model, agent, tool, and retrieval attributes
- [ ] Add fixtures for model calls, MCP tool calls, retrieval, and multi-agent handoff
- [ ] Publish a compatibility matrix

Exit gate: two unrelated frameworks produce the same graph semantics for equivalent behavior.

## Phase 2 — evidence quality

- [ ] Integrate OpenSSF Model Signing verification for local model artifacts
- [ ] Add confidence and source precedence rules
- [ ] Add graph-path policies and capability escalation detection
- [ ] Generate signed, reproducible snapshots with a canonical representation
- [ ] Validate CycloneDX output against the official schema in CI

Exit gate: five real drift scenarios are detected with documented evidence and no prompt content retention.

## Phase 3 — external validation

- [ ] Submit one upstream OpenTelemetry, SPDX, CycloneDX, or framework contribution
- [ ] Obtain feedback from at least three external operators
- [ ] Run one authorized pilot against a non-demo system
- [ ] Measure adapter maintenance effort for eight weeks

Commercial gate: at least one team is willing to pay for a private report, integration, evidence retention, or self-hosted deployment.

## Stop or pivot conditions

- Users want only static MCP scanning already covered by established tools.
- Runtime collection requires a private hook for every client.
- Operators will not enable OpenTelemetry or provide sanitized trace data.
- Inferred and observed facts cannot be clearly distinguished.
- Adapter maintenance consistently exceeds core development time.
