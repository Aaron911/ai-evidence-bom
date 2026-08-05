# Proposal: align OpenTelemetry MCP conventions with MCP 2026-07-28

Status: **submitted for upstream discussion** as [open-telemetry/semantic-conventions-genai#437](https://github.com/open-telemetry/semantic-conventions-genai/issues/437)

Reviewed: 2026-08-05

Suggested issue title:

> MCP: align semantic conventions with protocol 2026-07-28 and expose peer server implementation metadata

## Summary

The current OpenTelemetry GenAI MCP conventions describe the stateful MCP lifecycle through `2025-06-18`: the examples use `initialize`, spans recommend `mcp.session.id`, and the metric set includes client and server session duration. MCP `2026-07-28` removes the protocol-level handshake and session, adds `server/discover`, carries client metadata on each request and server metadata on each response, and introduces `subscriptions/listen` for change notifications.

This proposal asks the GenAI SIG to:

1. make the existing lifecycle and session conventions explicitly version-aware;
2. add the current well-known MCP methods and a stateless reference scenario; and
3. define how client-side instrumentation records the server's self-reported implementation name and version without confusing that declaration with a verified or globally unique identity.

The proposal is based on a reproducible official Go SDK v1.7.0 stdio client/server run. It does not require recording prompts, tool arguments, tool results, descriptions, or schemas.

## Reviewed upstream boundary

### OpenTelemetry

- Repository: [`open-telemetry/semantic-conventions-genai`](https://github.com/open-telemetry/semantic-conventions-genai)
- Reviewed commit: [`7e6e1884b242a277e6e2494e698f69481fe6fea8`](https://github.com/open-telemetry/semantic-conventions-genai/tree/7e6e1884b242a277e6e2494e698f69481fe6fea8)
- MCP document: [`docs/gen-ai/mcp.md`](https://github.com/open-telemetry/semantic-conventions-genai/blob/7e6e1884b242a277e6e2494e698f69481fe6fea8/docs/gen-ai/mcp.md)
- Status: Development

At that commit, the convention:

- has no `server/discover` method value or example;
- has no logical or peer MCP server implementation name/version attribute;
- recommends `mcp.session.id` on client and server spans;
- defines `mcp.client.session.duration` and `mcp.server.session.duration`;
- demonstrates `initialize` and `notifications/initialized` with protocol `2025-06-18`;
- already specifies W3C Trace Context propagation through `params._meta`.

### MCP

- Protocol: [`2026-07-28`](https://modelcontextprotocol.io/specification/2026-07-28)
- Discovery: [`server/discover`](https://modelcontextprotocol.io/specification/draft/server/discover)
- Lifecycle change: [`SEP-2575`](https://modelcontextprotocol.io/seps/2575-stateless-mcp)
- Release explanation: [`2026-07-28` release candidate](https://blog.modelcontextprotocol.io/posts/2026-07-28-release-candidate/)
- Official implementation: [Go SDK v1.7.0](https://github.com/modelcontextprotocol/go-sdk/releases/tag/v1.7.0), commit [`bc72835f62eb94d0fb484439f886b6885b075f36`](https://github.com/modelcontextprotocol/go-sdk/tree/bc72835f62eb94d0fb484439f886b6885b075f36)

For `2026-07-28`:

- `initialize` and `notifications/initialized` are removed from the current lifecycle;
- the protocol-level session and `Mcp-Session-Id` are removed;
- `server/discover` exposes supported versions, capabilities, and self-reported server metadata;
- requests carry protocol version, client capabilities, and optional client information in `_meta`;
- responses may carry `io.modelcontextprotocol/serverInfo` in `_meta`;
- `subscriptions/listen` replaces the legacy session-oriented notification channel;
- `traceparent`, `tracestate`, and `baggage` remain the standard propagation keys.

The protocol explicitly treats server information as self-reported and not suitable by itself for security decisions. Tool names are scoped to a server, while a server name is not guaranteed to be globally unique.

## Why existing attributes are insufficient

`server.address` identifies a network endpoint. It is absent for common stdio deployments and does not identify the logical server implementation behind a gateway, proxy, or changing endpoint.

Server-side Resource `service.name` identifies the process or deployed service that emitted telemetry. A client usually cannot see that Resource, and it is not necessarily equal to the protocol implementation metadata returned to the client.

`gen_ai.tool.name` identifies a tool only within its server scope. Two MCP servers may both provide a tool named `search`, so a client trace containing only the tool name cannot support stable server-scoped analysis.

The missing concept is therefore not another transport address. It is low-cardinality peer implementation metadata learned from the MCP protocol and recorded as an untrusted declaration.

## Reproducible evidence

AI Evidence BOM runs the official Go SDK v1.7.0 client and server in separate processes over stdio. The deterministic test performs:

1. `server/discover` and protocol negotiation for `2026-07-28`;
2. `tools/list` for declared capabilities;
3. `tools/call weather.lookup` with standard MCP and GenAI span attributes;
4. a second run in which the same server additionally declares but does not call `shell.execute`.

The server name and version returned by protocol discovery remain stable across the two runs. `weather.lookup` is observed, while `shell.execute` remains declared. Without the protocol server metadata, the stdio client spans contain no standard field that distinguishes the logical server.

Reproduction:

```bash
git clone https://github.com/Aaron911/ai-evidence-bom.git
cd ai-evidence-bom
git checkout 932b4e4c61d99157ae0ffd6a0e490577f0011a35
scripts/live/verify_mcp_runtime.sh
```

The check requires Go 1.26.5+, `curl`, and `jq`; it uses no model credential or paid API. Its exact assertions and negative evidence are recorded in [`docs/evidence/v0.7.0.md`](https://github.com/Aaron911/ai-evidence-bom/blob/932b4e4c61d99157ae0ffd6a0e490577f0011a35/docs/evidence/v0.7.0.md).

## Proposed semantic direction

This section proposes semantics for discussion, not a demand for the final attribute names.

### 1. Make lifecycle semantics version-aware

- Add `server/discover` and `subscriptions/listen` to the well-known `mcp.method.name` values.
- Add a `2026-07-28` stateless client/server example.
- Keep `initialize` and `notifications/initialized` as well-known legacy values, labeled as applicable to older protocol versions.
- Make `mcp.session.id` conditional on a protocol/transport session actually existing. It should not be emitted merely because an SDK exposes an in-process object named “session.”
- Scope session-duration metrics to protocol versions that define a session, or deprecate them in favor of operation/subscription duration for stateless versions.

### 2. Record self-reported peer server implementation metadata

One possible naming direction is:

| Candidate attribute | Semantics |
|---|---|
| `mcp.server.name` | Server implementation name reported by MCP response metadata or legacy initialization. |
| `mcp.server.version` | Server implementation version reported with that name. |

Suggested requirements:

- conditionally required when the instrumentation has received the protocol metadata;
- available on client spans after discovery or after response metadata becomes available;
- available on server spans from the implementation metadata the server is configured to report;
- explicitly documented as self-reported, unverified, and not globally unique;
- never derived from tool names, descriptions, schemas, arguments, results, executable paths, or prompts;
- not a replacement for `server.address`, `service.name`, or an independently verified deployment identity;
- not sufficient alone for authorization, trust, or security decisions.

If the SIG prefers an existing peer-service/entity convention or a different namespace, the important interoperability requirement is to preserve the protocol source and the declaration boundary.

### 3. Keep sensitive content out of the identity proposal

The proposal does not need MCP instructions, tool descriptions, schemas, arguments, or results. Server implementation name/version and method/protocol/transport metadata are enough for the identity and lifecycle use case. Existing opt-in content fields should remain opt-in.

## Suggested reference scenario

A minimal reference scenario can use a deterministic stdio server:

```text
invoke_agent travel-agent
  ├─ server/discover                 CLIENT
  │    mcp.protocol.version = 2026-07-28
  │    mcp.server.name = demo-security-tools
  │    mcp.server.version = 1.0.0
  ├─ tools/list                      CLIENT
  │    mcp.server.name = demo-security-tools
  └─ tools/call weather.lookup       CLIENT
       gen_ai.operation.name = execute_tool
       gen_ai.tool.name = weather.lookup
       mcp.server.name = demo-security-tools
       network.transport = pipe
```

The scenario should assert that:

- no `mcp.session.id` is present for protocol `2026-07-28`;
- server metadata is a declaration, not a verified identity;
- trace context reaches the server span through `_meta`;
- tool arguments and results are absent unless content capture is explicitly enabled;
- duplicate outer `execute_tool` instrumentation does not produce duplicate spans.

## Questions for maintainers

1. Should protocol-reported server implementation metadata use `mcp.server.*`, an existing peer entity, or another naming model?
2. Should the metadata be recorded on every operation after it is known, or only on discovery and linked through another identity mechanism?
3. Should the existing session-duration metrics be retained only for legacy versions, or deprecated entirely as MCP moves to the stateless lifecycle?
4. Would the SIG prefer lifecycle alignment and server metadata as two separate focused pull requests?

## Acceptance signal for this project

The proposal was submitted on 2026-08-05 as [issue #437](https://github.com/open-telemetry/semantic-conventions-genai/issues/437). Submission alone is not acceptance.

For AI Evidence BOM, this upstream experiment succeeds when maintainers confirm that the interoperability gap is real and select a concrete semantic direction, even if the final names differ from this draft. Until then, the project will keep its `aiebom.mcp.server.*` fields explicitly labeled as a compatibility bridge and will not add another standard-looking alias.
