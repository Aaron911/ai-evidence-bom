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

OTLP spans are `observed` by default. The project extension `aiebom.evidence.level` may downgrade an OTLP observation to `declared` or `inferred`; it cannot promote evidence to `verified`. The MCP adapter uses this for content-free capabilities returned by `tools/list`.

Signature-looking OTLP attributes such as `gen_ai.model.signature.verified=true` are untrusted metadata and do not change the evidence level. A compact-input adapter may emit `verified` only after performing verification outside the current normalizer; the built-in OpenSSF Model Signing verifier remains roadmap work.

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

At most 20 trace identifiers are retained per node, edge, or field-value candidate in v0.8. Observation counts continue increasing after that cap.

Continuous collection merges snapshots by stable node and edge identity. It preserves the strongest node evidence level, earliest and latest observation time, all observed versions, and cumulative observation counts.

### Field evidence and conflicts

Node-level evidence describes the node as a whole; it is not automatically proof for each mutable field. v0.8 therefore records candidates for `version`, each digest algorithm, and each retained property:

```json
{
  "version": "weights-v1",
  "observedVersions": ["weights-v1", "weights-v2"],
  "fieldEvidence": [
    {
      "field": "version",
      "selectedValue": "weights-v1",
      "conflict": true,
      "values": [
        {
          "value": "weights-v1",
          "evidence": {
            "level": "verified",
            "sources": ["signature-verifier"],
            "observationCount": 1
          }
        },
        {
          "value": "weights-v2",
          "evidence": {
            "level": "declared",
            "sources": ["deployment-config"],
            "observationCount": 1
          }
        }
      ]
    }
  ],
  "evidence": {
    "level": "verified",
    "observationCount": 2
  }
}
```

`field` is `version`, `digest`, or `property`. Digest and property entries also carry `key`; version does not. `selectedValue` is copied to the existing top-level `version`, `digests[key]`, or `properties[key]` field for compatibility with exporters and consumers.

Selection is deterministic and independent of arrival order:

1. stronger evidence level wins;
2. at the same level, later `lastSeen` wins;
3. an exact tie uses lexically smaller value as the stable tie-breaker.

Distinct candidates remain in `values`, and `conflict` is true whenever more than one value exists. A stronger older value can therefore remain selected while a newer weaker claim is still visible to diff and policy. `forbidFieldConflicts` makes any such conflict a policy violation. CycloneDX export carries the selected and candidate values under `aibom:field-evidence:*` properties and emits `aibom:field-conflict` for each conflict.

When a v0.7 graph without `fieldEvidence` is first merged, its existing mutable values become conservative `inferred` compatibility candidates. The node-level summary is not copied as field-level verification because old graphs cannot establish which observation supplied a field. New v0.8 observations rebuild field-specific strength.

OTLP JSON and protobuf requests are converted to the same internal observation contract. Instrumentation scope name, version, and schema URL are retained on relevant nodes as provenance. Unknown OTLP fields are ignored, and arbitrary attributes are not copied into output properties.

## Directed path policy

`deniedPaths` matches an exact directed relationship sequence. `from`, optional `via`, and `to` selectors support node type, provider, regular-expression name matching, and minimum evidence. Paths are bounded to eight relationships. A violation records the matched node IDs and relations; disconnected nodes do not match. `forbidFieldConflicts` independently rejects nodes with competing version, digest, or property candidates.

See [`examples/policy-mcp-capability.json`](../examples/policy-mcp-capability.json) for an Agent-to-MCP-to-tool capability rule.

## CycloneDX mapping

| Evidence node | CycloneDX component type |
|---|---|
| model | `machine-learning-model` |
| data source | `data` |
| prompt | `data` |
| software | `library` |
| agent, tool, MCP server | `application` |

Evidence-specific fields are exported as namespaced CycloneDX properties beginning with `aibom:`. Field candidate properties contain only values already retained by the evidence graph; they do not add model input, output, prompt, credential, tool argument, or tool result content.
