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

## v0.5 calibration — 2026-08-04

### Uncertainty tested

Do the two source-derived contracts survive executable framework code and real OTLP export, including the case where related spans are split across export requests?

### Evidence gained

- Released Microsoft Agent Framework 1.13.0 ran its real Agent, chat telemetry, function-invocation, tool-execution, and OTLP exporter paths. The expected agent/model/tool graph arrived without sensitive content.
- Dify 1.16.1's real workflow handler, LLM parser, and tool parser exported the same core semantics through OTLP. This was isolated instrumentation execution with deterministic host stubs, not a complete Dify application runtime.
- Executable validation found a normalizer defect that source fixtures did not expose: child spans exported before their parent were temporarily attributed to `service.name`.
- Bounded, metadata-only cross-batch context now delays unresolved children until parent identity arrives. Regression tests prove that content is removed before queuing and that the false service agent is not created.

### Decision

**Continue, but keep the Phase 1 exit gate open.** The standards-first thesis became stronger: one released framework core runtime and one independent framework's actual instrumentation reached the same graph without a private transport adapter. However, Dify has not yet passed a complete application-runtime capture, so v0.5 must not claim live compatibility for two full frameworks.

The next iteration should finish that missing runtime proof before expanding into generic zero-code auto-instrumentation, dashboards, vulnerability feeds, or additional frameworks. The new cross-batch behavior is core infrastructure, not framework-specific debt.

### Risks and pivot signals

- If a complete Dify run cannot expose stable app identity, model, and tool metadata through its supported OTLP configuration, downgrade the Dify path and consider a small maintained adapter.
- If real deployments routinely exceed the bounded correlation window, add an explicit trace-completion/TTL design rather than silently increasing memory.
- If users cannot enable framework telemetry, test a narrowly scoped zero-code Python launcher; do not promise universal no-code coverage across runtimes.
- Continue to reject content-derived component identity and unverified claims about hosted model weights.

### Next smallest falsifiable test

Start an official minimal Dify application stack in a disposable Docker/CI environment, enable its supported OTLP HTTP exporter, execute one workflow containing an LLM node and a tool node against deterministic local substitutes, and send the unmodified trace to `aiebom collect`.

The test passes only if:

1. no Dify source or runtime patch is required;
2. `dify.app_id`, model provider/name, and tool name form the expected graph;
3. child-first or split-batch export does not create a service fallback agent;
4. graph and BOM contain no prompt, model-response, or tool-argument marker;
5. the exact Dify image/source version and configuration are recorded.

If that cannot be made deterministic at reasonable CI cost, record the blocker and move the check to a documented periodic manual environment rather than weakening the evidence grade.

## v0.6 calibration — 2026-08-04

### Uncertainty tested

Can a complete official Dify application runtime emit stable agent, model, and tool evidence through its supported OTLP configuration, without patching Dify or introducing a private telemetry adapter?

### Evidence gained

- A pinned Dify 1.16.1 API, plugin daemon, PostgreSQL, and Redis stack imported, published, and executed a workflow containing an LLM node and a built-in tool node.
- With Dify's supported OTel settings enabled, unmodified OTLP/HTTP protobuf reached `aiebom collect`. The graph contained exactly one agent identified by `dify.app_id`, OpenAI `gpt-4o`, `time.current_time`, a content-free prompt node, and `uses`, `invokes`, and `uses_prompt` relationships.
- Prompt, input, model output, tool argument, and tool output markers did not reach the graph or CycloneDX BOM.
- The first CI attempt failed usefully: the Dify container could not resolve the Marketplace host while installing the plugin, and failure cleanup did not terminate promptly. The final check downloads a fixed official plugin package on the host, verifies its SHA-256, uploads it through Dify's supported API, and bounds cleanup time. This makes setup deterministic without changing Dify telemetry behavior.
- Together with the released Microsoft Agent Framework core-runtime check, two unrelated runtimes now reach the same vendor-neutral core graph semantics through standard OTLP. The Phase 1 core exit gate is therefore passed.

### Decision

**Continue and move the primary focus to evidence quality.** The standards-first thesis passed its core adoption test. The project should not respond by collecting framework logos, building a dashboard, or adding generic vulnerability feeds. Optional retrieval, MCP, and multi-agent coverage remains open, but new work should be selected for security value and its ability to falsify graph or policy claims.

### Risks and pivot signals

- Full Dify validation is heavier than the isolated check and depends on Docker plus availability of pinned upstream source and plugin artifacts. Keep the fast check in ordinary CI and the full stack in a separate scheduled and path-triggered workflow.
- Dify can emit content-bearing telemetry when configured to do so. Metadata-only filtering at the receiver remains a security boundary; this result does not make upstream traces safe to store elsewhere.
- One deterministic workflow is live capture, not production validation. Do not claim coverage of every Dify node, provider, deployment mode, or operational limit.
- If upstream semantic conventions repeatedly break stable identity or require framework-specific transport code, measure that maintenance cost and reconsider the adapter boundary.
- If real operators will not enable OTel or cannot route OTLP safely, validate a narrow auto-instrumentation launcher before making any zero-code claim.

### Next smallest falsifiable test

Execute one real MCP client/server tool call with standard telemetry and a stable, non-content-derived server identity. Capture a baseline and a changed server capability set, then verify that graph diff and a graph-path policy detect the new capability without retaining arguments or results.

The test fails if server identity can only be guessed from tool text or network content, if the capability change cannot be tied to the invoking agent, or if policy evaluation requires a framework-specific private transport. A failure should be recorded as an MCP telemetry-standard gap before adding an adapter.

## Release calibration template

For each later release, append a dated section containing:

- **Uncertainty tested:** one falsifiable question;
- **Evidence gained:** results, including negative results;
- **Decision:** continue, narrow, pause, or pivot;
- **Risks and pivot signals:** measurable conditions rather than optimism;
- **Next smallest falsifiable test:** one bounded step toward the current phase exit gate.
