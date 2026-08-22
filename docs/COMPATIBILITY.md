# Framework compatibility

Compatibility claims are evidence-graded. A green source contract does not imply that a live deployment has been tested.

## Evidence grades

| Grade | Meaning |
|---|---|
| Source contract | A sanitized fixture is derived from pinned upstream instrumentation code and protected by automated normalization tests. |
| Instrumentation execution | Pinned framework instrumentation code is executed against deterministic host objects and exports through a real OTLP exporter, without claiming a complete application runtime. |
| Live capture | A framework runtime has exported a trace through a standard OTLP transport and the resulting graph has been verified. |
| Production validated | An authorized non-demo workload has run long enough to exercise relevant paths and operational limits. |

v0.9 retains the **live capture** grades established for the Microsoft Agent Framework core path, a complete Dify application workflow, and one official MCP Go SDK stdio client/server path. None is production validated. Coverage remains limited to the paths described below.

The v0.8 schema changes merge semantics rather than adding a framework claim: versions, digests, and allowlisted properties retain source-specific candidates, and stronger evidence wins independently of arrival order. v0.9 adds an ingestion trust boundary: sources default to at most observed, and only an exact operator-authorized source can retain verified evidence. Neither change upgrades a framework's evidence grade or makes an untrusted telemetry producer a verifier.

## v0.6 matrix

| Capability | Dify | Microsoft Agent Framework | v0.6 treatment |
|---|---|---|---|
| Standard OTLP transport | HTTP and gRPC exporters selected by configuration | Standard OpenTelemetry exporters, including OTLP configuration | A complete Dify workflow and the Agent Framework core path both reached the receiver over OTLP/HTTP protobuf without a private transport adapter. Framework telemetry still has to be enabled and configured. |
| Stable agent identity | `dify.app_id` on an ancestor workflow span | `gen_ai.agent.id` and `gen_ai.agent.name` on `invoke_agent` | Identity is inherited through `parentSpanId`; nested explicit agent identity takes precedence. |
| Model call | `gen_ai.span.kind=LLM`, model and provider attributes | Concrete `chat` span with model and provider attributes | Both normalize to the same model node and `agent -[uses]-> model` edge. |
| Tool call | Tool span with `gen_ai.tool.name` | `execute_tool` span and tool attributes | Both normalize to the same tool node and `agent -[invokes]-> tool` edge. |
| Prompt metadata | Prompt content can be emitted | Input/output message fields can be emitted | Sensitive bodies are discarded. Dify may yield additional content-free prompt evidence, so compatibility compares the shared core graph rather than forcing identical evidence richness. |
| Retrieval data source | Retrieval query/documents exist, but the reviewed path does not supply a stable data-source ID | Not yet source-validated | Not claimed. Content is never used as identity. |
| MCP server | Tool activity alone does not establish stable MCP server identity | MCP method/network details may exist without a stable server ID, especially for stdio | Partial telemetry is insufficient; no MCP server node is invented. |
| Multi-agent handoff | Not validated | Not validated | Not claimed. |

## v0.7 MCP runtime boundary

| Concern | Upstream behavior | v0.7 treatment |
|---|---|---|
| Protocol runtime | Official Go SDK v1.7.0 client and server over `CommandTransport`/`StdioTransport`; negotiated MCP `2026-07-28` | Real `server/discover`, `tools/list`, and `tools/call` execute in separate processes. |
| Server identity | `server/discover` returns `serverInfo.name` and `serverInfo.version` | Identity is protocol-derived, non-content-based, and attached with the explicit `aiebom.mcp.server.*` extension. |
| Standard telemetry | Current OTel MCP conventions define method, protocol version, tool name, and `network.transport=pipe`, but no logical server-name field | Application middleware emits the standard attributes around actual SDK list/call operations. The SDK is not claimed to auto-emit OTLP. |
| Declared capability | `tools/list` returns tool definitions and schemas | Tool name, selected annotation hints, and input-schema digest are `declared`; descriptions and schema bodies are discarded. |
| Observed invocation | `tools/call` executes `weather.lookup` | The tool, Agent invocation, server connection, and provider relationship become `observed`. |
| Capability drift | Changed server adds `shell.execute` but the Agent never calls it | Diff reports the added declared tool and `provides` edge; it does not add an `invokes` edge. |
| Path policy | Agent connects to a server that provides the added tool | The baseline passes and the changed graph fails `agent -[connects_to]-> mcp_server -[provides]-> shell.execute`. |
| Privacy | MCP descriptions, schema, arguments, and results may contain sensitive data | Marker assertions prove none reach graph or CycloneDX; only the input-schema digest is retained. |

This is client-side application instrumentation and protocol discovery, not server-side OTel instrumentation, cross-process trace-context validation, universal MCP client coverage, or production validation. MCP annotation hints are recorded as untrusted declarations.

Related spans may be split across OTLP export requests. v0.6 delays unresolved children in a bounded metadata-only queue, then releases them when the parent context arrives. This behavior is regression-tested with child-first Dify spans; queued observations exclude prompt/input/output bodies and tool arguments/results.

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
scripts/live/verify_dify_runtime.sh
scripts/live/verify_mcp_runtime.sh
```

The lightweight framework scripts require Go 1.26.6+, Python 3.12+, `uv`, `git`, `curl`, and `jq`. The MCP check requires Go 1.26.6+, `curl`, and `jq`. The full Dify check additionally requires Docker Compose, `tar`, and a SHA-256 utility. Cold runs need network access for pinned source, packages, and the plugin artifact; no check needs model credentials or makes a paid model call.

The Agent Framework and isolated Dify checks exercise equivalent `gpt-5` and `weather.lookup` behavior. The full Dify workflow uses the official OpenAI plugin with deterministic local responses and exercises `gpt-4o` plus the built-in `time.current_time` tool. Framework checks require stable agent/model/tool semantics; the MCP check requires stable agent/server/tool capability semantics. Every check fails if a sensitive marker reaches the graph or CycloneDX output.

## Pinned upstream evidence

The fixtures are manually minimized contracts derived from upstream source, not copied runtime recordings:

- Dify commit [`3ada29b`](https://github.com/langgenius/dify/tree/3ada29bbe06a33b9679b30f37a995562118aa173): [OTLP exporter selection](https://github.com/langgenius/dify/blob/3ada29bbe06a33b9679b30f37a995562118aa173/api/extensions/ext_otel.py) and [GenAI attributes](https://github.com/langgenius/dify/blob/3ada29bbe06a33b9679b30f37a995562118aa173/api/extensions/otel/semconv/gen_ai.py).
- Microsoft Agent Framework commit [`07511b8`](https://github.com/microsoft/agent-framework/tree/07511b80c9bd6369f1dab00981d744354e24d1a9): [Python observability instrumentation](https://github.com/microsoft/agent-framework/blob/07511b80c9bd6369f1dab00981d744354e24d1a9/python/packages/core/agent_framework/observability.py).
- Official MCP Go SDK [`v1.7.0`](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0), commit [`bc72835`](https://github.com/modelcontextprotocol/go-sdk/tree/bc72835f62eb94d0fb484439f886b6885b075f36), with MCP `2026-07-28` support.
- OpenTelemetry GenAI MCP conventions commit [`7e6e188`](https://github.com/open-telemetry/semantic-conventions-genai/tree/7e6e1884b242a277e6e2494e698f69481fe6fea8), status Development.

The live Microsoft check uses released `agent-framework-core==1.13.0`. The lightweight Dify check loads the handler and parsers directly from commit `3ada29b`, with surrounding application/Graphon objects replaced by deterministic stubs.

The full Dify check runs `langgenius/dify-api:1.16.1`, `langgenius/dify-plugin-daemon:0.6.3-local`, PostgreSQL, and Redis from the same pinned Dify commit. It installs official OpenAI plugin identifier `langgenius/openai:0.2.5@373362a028986aae53a7baf73a7f11991ba3c22c69eaf97d6cde048cfd4a9f98`; the downloaded package must match SHA-256 `e055503b333915818e1c26654276cceaa4d0498ced726c3a752087394b0b00b3`. The package is uploaded through Dify's supported console API so plugin-container Marketplace DNS is not part of the telemetry assertion. No Dify source or runtime image is patched. See [the v0.6 evidence record](evidence/v0.6.0.md) for the exact boundary and CI run.

Because upstream protocol and telemetry conventions evolve independently, a source commit, SDK release, or semantic-convention change must trigger fixture and executable-check review. In particular, the current OTel document still centers legacy lifecycle examples while MCP `2026-07-28` uses `server/discover` and per-request metadata; v0.7 records this as a standards gap rather than silently treating project aliases as standard.
