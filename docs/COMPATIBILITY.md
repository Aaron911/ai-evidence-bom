# Framework compatibility

Compatibility claims are evidence-graded. A green source contract does not imply that a live deployment has been tested.

## Evidence grades

| Grade | Meaning |
|---|---|
| Source contract | A sanitized fixture is derived from pinned upstream instrumentation code and protected by automated normalization tests. |
| Instrumentation execution | Pinned framework instrumentation code is executed against deterministic host objects and exports through a real OTLP exporter, without claiming a complete application runtime. |
| Live capture | A framework runtime has exported a trace through a standard OTLP transport and the resulting graph has been verified. |
| Production validated | An authorized non-demo workload has run long enough to exercise relevant paths and operational limits. |

v0.5 reaches **live capture** for the Microsoft Agent Framework core path and **instrumentation execution** for Dify. Neither framework is production validated, and Dify full-runtime live capture remains open.

## v0.5 matrix

| Capability | Dify | Microsoft Agent Framework | v0.5 treatment |
|---|---|---|---|
| Standard OTLP transport | HTTP and gRPC exporters selected by configuration | Standard OpenTelemetry exporters, including OTLP configuration | Dify's real parser output and the full Agent Framework core path both reached the receiver over OTLP/HTTP protobuf. |
| Stable agent identity | `dify.app_id` on an ancestor workflow span | `gen_ai.agent.id` and `gen_ai.agent.name` on `invoke_agent` | Identity is inherited through `parentSpanId`; nested explicit agent identity takes precedence. |
| Model call | `gen_ai.span.kind=LLM`, model and provider attributes | Concrete `chat` span with model and provider attributes | Both normalize to the same model node and `agent -[uses]-> model` edge. |
| Tool call | Tool span with `gen_ai.tool.name` | `execute_tool` span and tool attributes | Both normalize to the same tool node and `agent -[invokes]-> tool` edge. |
| Prompt metadata | Prompt content can be emitted | Input/output message fields can be emitted | Sensitive bodies are discarded. Dify may yield additional content-free prompt evidence, so compatibility compares the shared core graph rather than forcing identical evidence richness. |
| Retrieval data source | Retrieval query/documents exist, but the reviewed path does not supply a stable data-source ID | Not yet source-validated | Not claimed. Content is never used as identity. |
| MCP server | Tool activity alone does not establish stable MCP server identity | MCP method/network details may exist without a stable server ID, especially for stdio | Partial telemetry is insufficient; no MCP server node is invented. |
| Multi-agent handoff | Not validated | Not validated | Not claimed. |

Related spans may be split across OTLP export requests. v0.5 delays unresolved children in a bounded metadata-only queue, then releases them when the parent context arrives. This behavior is regression-tested with child-first Dify spans; queued observations exclude prompt/input/output bodies and tool arguments/results.

## Reproducible contract

The sanitized fixtures are:

- [`examples/frameworks/dify-otlp.json`](../examples/frameworks/dify-otlp.json)
- [`examples/frameworks/microsoft-agent-framework-otlp.json`](../examples/frameworks/microsoft-agent-framework-otlp.json)

Run the source-contract test with:

```bash
go test ./internal/normalize -run SourceDerivedFrameworkFixtures
```

Run the executable checks with:

```bash
scripts/live/verify_agent_framework.sh
scripts/live/verify_dify_instrumentation.sh
```

The scripts require Go 1.26.5+, Python 3.12+, `uv`, `git`, `curl`, and `jq`. They require network access to obtain pinned Python packages and source on a cold run, but no model credentials. For equivalent behavior, every check requires the same stable agent identity, OpenAI `gpt-5` model identity, `weather.lookup` tool identity, and core `uses`/`invokes` edges. It also fails if any sensitive marker reaches the graph or CycloneDX output.

## Pinned upstream evidence

The fixtures are manually minimized contracts derived from upstream source, not copied runtime recordings:

- Dify commit [`3ada29b`](https://github.com/langgenius/dify/tree/3ada29bbe06a33b9679b30f37a995562118aa173): [OTLP exporter selection](https://github.com/langgenius/dify/blob/3ada29bbe06a33b9679b30f37a995562118aa173/api/extensions/ext_otel.py) and [GenAI attributes](https://github.com/langgenius/dify/blob/3ada29bbe06a33b9679b30f37a995562118aa173/api/extensions/otel/semconv/gen_ai.py).
- Microsoft Agent Framework commit [`07511b8`](https://github.com/microsoft/agent-framework/tree/07511b80c9bd6369f1dab00981d744354e24d1a9): [Python observability instrumentation](https://github.com/microsoft/agent-framework/blob/07511b80c9bd6369f1dab00981d744354e24d1a9/python/packages/core/agent_framework/observability.py).

The live Microsoft check uses released `agent-framework-core==1.13.0`. The Dify execution check loads the handler and parsers directly from commit `3ada29b`, with surrounding application/Graphon objects replaced by deterministic stubs. See [the v0.5 evidence record](evidence/v0.5.0.md) for the exact boundary.

Because upstream telemetry conventions evolve, a source commit or released package change must trigger fixture and executable-check review.
