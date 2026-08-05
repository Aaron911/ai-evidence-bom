# Data contracts

## Compact observation input

The compact format is useful for adapters that do not emit OTLP directly:

```json
{
  "source": "agent-platform",
  "observations": [
    {
      "timestamp": "2026-08-04T12:00:00Z",
      "level": "declared",
      "traceId": "optional-trace-id",
      "spanId": "model-call-span",
      "parentSpanId": "agent-invocation-span",
      "attributes": {
        "service.name": "release-assistant",
        "gen_ai.agent.name": "release-assistant",
        "gen_ai.provider.name": "provider",
        "gen_ai.request.model": "model-alias"
      }
    }
  ]
}
```

Unknown attributes remain available to future adapters, but only an allowlisted subset is copied into graph properties.

OTLP spans are `observed` by default. The project extension `aiebom.evidence.level` may downgrade an OTLP observation to `declared` or `inferred`; it cannot promote evidence to `verified`. v0.7 uses this only for content-free MCP capabilities returned by `tools/list`.

## MCP identity and capability bridge

Current OpenTelemetry MCP conventions provide `mcp.method.name`, `mcp.protocol.version`, `gen_ai.tool.name`, and `network.transport`, but do not provide a stable logical MCP server identity. v0.7 therefore keeps two evidence sources explicit:

- MCP `serverInfo` supplies the declared server name and version;
- standard client spans show that an Agent actually listed or called tools over the negotiated protocol and transport.

The live adapter attaches protocol-derived identity using project-owned attributes:

| Attribute | Meaning |
|---|---|
| `aiebom.mcp.server.name` | Logical `serverInfo.name` used for stable identity. |
| `aiebom.mcp.server.version` | `serverInfo.version`. |
| `aiebom.mcp.server.identity_source` | Protocol source, currently `server_info`. |
| `aiebom.mcp.discovery.source` | Capability source, currently `tools/list`. |
| `aiebom.mcp.tool.input_schema_sha256` | SHA-256 of the adapter's JSON encoding; raw schema is discarded. |
| `aiebom.mcp.tool.read_only`, `aiebom.mcp.tool.destructive` | Untrusted MCP annotation hints. |
| `aiebom.mcp.tool.annotations_untrusted` | Explicit warning that annotation hints are not verified behavior. |

These are AI Evidence BOM extensions, not OpenTelemetry standard attributes. Legacy `mcp.server.*` and `mcp.tool.*` aliases remain readable for compatibility but have the same non-standard status. Descriptions, schema bodies, arguments, and results are never graph properties.

`traceId`, `spanId`, and `parentSpanId` form normalization context rather than graph content. A child model or tool span inherits the nearest explicit agent identity from its ancestors in the same trace. A nested span with its own explicit agent identity starts a new context. Cycles, missing parents, and cross-trace parent IDs are ignored safely.

When an `invoke_agent` span summarizes a requested model and a concrete model operation such as `chat` exists below it, the summary model is suppressed. The concrete model span retains the actual model provider and evidence. This avoids counting one call twice or treating a framework provider as the model provider.

## Evidence graph

The graph contains:

- `schemaVersion`, `generatedAt`, `source`, and privacy metadata;
- stable `nodes` for agents, models, tools, MCP servers, prompts, and data sources;
- stable `edges` describing `uses`, `invokes`, `provides`, `connects_to`, `reads_from`, and `uses_prompt` relationships.

MCP capability paths are directed: `agent -[connects_to]-> mcp_server -[provides]-> tool`. A tool returned by `tools/list` is `declared`; an actual `tools/call` observation upgrades that same tool and `provides` edge to `observed` and adds `agent -[invokes]-> tool`.

Node identity is a SHA-256-derived stable key of normalized `type`, `provider`, and the strongest available identity. A standards-defined ID such as `gen_ai.agent.id` is preferred; display name is the fallback. Version is deliberately excluded so an upgrade appears as a changed node rather than a removal and addition.

Each node and edge has an evidence summary:

```json
{
  "level": "observed",
  "sources": ["release-assistant"],
  "firstSeen": "2026-08-04T12:00:00Z",
  "lastSeen": "2026-08-04T12:05:00Z",
  "observationCount": 2,
  "traceIds": ["trace-id"]
}
```

At most 20 trace identifiers are retained per node or edge in v0.7. Observation counts continue increasing after that cap.

Continuous collection merges snapshots by stable node and edge identity. It preserves the strongest evidence level, earliest and latest observation time, all observed versions, and cumulative observation counts. The latest timestamp wins when version or property values conflict.

OTLP JSON and protobuf requests are converted to the same internal observation contract. Instrumentation scope name, version, and schema URL are retained on relevant nodes as provenance. Unknown OTLP fields are ignored, and arbitrary attributes are not copied into output properties.

## Directed path policy

`deniedPaths` matches an exact directed relationship sequence. `from`, optional `via`, and `to` selectors support node type, provider, regular-expression name matching, and minimum evidence. Paths are bounded to eight relationships. A violation records the matched node IDs and relations; disconnected nodes do not match.

See [`examples/policy-mcp-capability.json`](../examples/policy-mcp-capability.json) for an Agent-to-MCP-to-tool capability rule.

## CycloneDX mapping

| Evidence node | CycloneDX component type |
|---|---|
| model | `machine-learning-model` |
| data source | `data` |
| prompt | `data` |
| software | `library` |
| agent, tool, MCP server | `application` |

Evidence-specific fields are exported as namespaced CycloneDX properties beginning with `aibom:`.
