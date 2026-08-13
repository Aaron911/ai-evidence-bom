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
- [x] Add source-derived contract fixtures for Dify and Microsoft Agent Framework
- [x] Preserve OTLP parent context across export batches so child model and tool spans retain the correct agent identity
- [x] Validate live exports from two independent framework runtimes
  - [x] Microsoft Agent Framework released core runtime
  - [x] Dify isolated instrumentation execution
  - [x] Complete Dify application runtime
- [x] Normalize the core current OpenTelemetry GenAI model, agent, tool, and retrieval attributes
- [x] Add a real MCP stdio tool-call fixture with protocol-derived server identity and declared capability drift
- [ ] Add fixtures for retrieval and multi-agent handoff
- [x] Publish an evidence-graded compatibility matrix

Exit gate: live exports from two unrelated frameworks produce the same core graph semantics for equivalent behavior. **The core exit gate passed in v0.6.** Retrieval and multi-agent fixtures remain useful coverage work, but they no longer block Phase 2 evidence-quality validation. MCP gained a real stdio runtime path in v0.7.

## Phase 2 — evidence quality

- [ ] Integrate OpenSSF Model Signing verification for local model artifacts
- [x] Add field-level evidence candidates, deterministic evidence-strength precedence, and explicit conflict policy
- [ ] Add operator-defined trust rules that cap the evidence level allowed from each source
- [x] Add bounded graph-path policies and MCP capability escalation detection
- [x] Generate signed, reproducible evidence snapshots with an explicit RFC 8785-based canonical representation
- [x] Validate CycloneDX output against the checksum-pinned official schema in CI, including a negative control

Exit gate: five real drift scenarios are detected with documented evidence and no prompt content retention.

## Phase 3 — external validation

- [ ] Complete one upstream OpenTelemetry, SPDX, CycloneDX, or framework contribution
  - [x] Prepare an evidence-backed OpenTelemetry MCP `2026-07-28` lifecycle and server-metadata proposal
  - [x] Submit it after explicit authorization as [`open-telemetry/semantic-conventions-genai#437`](https://github.com/open-telemetry/semantic-conventions-genai/issues/437)
  - [x] Prepare and validate a lifecycle-only upstream patch locally at `49fa922`
  - [ ] Submit the lifecycle-only patch after explicit authorization
  - [ ] Obtain maintainer feedback and a concrete semantic direction
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

The release-by-release decision log and the next smallest falsifiable test are maintained in [DIRECTION.md](DIRECTION.md).
