# Direction calibration

AI Evidence BOM is trying to establish a vendor-neutral runtime evidence layer for AI agents. The durable product thesis is not “scan another manifest”; it is:

> Use standard telemetry to show which agents, models, tools, MCP servers, prompts, and data sources were actually involved, with explicit evidence strength and without retaining sensitive content.

Every release must answer four questions: what uncertainty did it test, what evidence changed, what would make the project stop or pivot, and what is the smallest next test?

## v0.4 calibration — 2026-08-04

### Uncertainty tested

Can two unrelated AI agent frameworks be represented by one evidence contract through standard OpenTelemetry data, without maintaining a private adapter for each framework?

### Evidence gained

- Dify exposes standard OTLP HTTP/gRPC exporter paths and emits workflow, model, and tool information across related spans.
- Microsoft Agent Framework emits `invoke_agent`, concrete model, and tool spans using OpenTelemetry GenAI attributes.
- The important compatibility problem is trace structure, not another vendor field alias: agent identity is often on a parent span while model/tool facts are on descendants.
- Source-derived fixtures now produce the same stable core agent/model/tool graph. Sensitive values are excluded by test.

### Decision

**Continue, with a narrower claim.** The standards-first direction remains viable. v0.4 establishes pinned source-contract compatibility, not live framework compatibility.

The implementation should continue to invest in trace semantics and evidence quality. It should not branch into dashboards, generic vulnerability scanning, or a large collection of private framework adapters before the live-export gate is passed.

### Risks and pivot signals

- Pivot toward a small set of maintained adapters if real exporters consistently omit stable identity that cannot be restored through OpenTelemetry configuration.
- Reconsider runtime-first positioning if operators will not enable OTLP or share sanitized captures.
- Do not identify a data source, MCP server, or model from prompt/document content; missing stable metadata must remain an explicit gap.
- Do not call source-derived fixtures “live support.” Overclaiming would damage the evidence product's credibility.

### Next smallest falsifiable test

Run minimal Dify and Microsoft Agent Framework applications, export their traces through the unmodified standard OTLP path into `aiebom collect`, and verify:

1. both reach the receiver without a framework-specific transport;
2. equivalent agent/model/tool behavior yields the same core graph semantics;
3. framework-specific extra evidence remains additive;
4. no sensitive content is written to graph or BOM output.

If this test passes, Phase 1 can claim live compatibility for the core path. If it fails, record whether the cause is exporter setup, missing stable identity, semantic drift, or a normalizer defect before adding code.

## Release calibration template

For each later release, append a dated section containing:

- **Uncertainty tested:** one falsifiable question;
- **Evidence gained:** results, including negative results;
- **Decision:** continue, narrow, pause, or pivot;
- **Risks and pivot signals:** measurable conditions rather than optimism;
- **Next smallest falsifiable test:** one bounded step toward the current phase exit gate.
