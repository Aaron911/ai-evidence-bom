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

`traceId`, `spanId`, and `parentSpanId` form normalization context rather than graph content. A child model or tool span inherits the nearest explicit agent identity from its ancestors in the same trace. A nested span with its own explicit agent identity starts a new context. Cycles, missing parents, and cross-trace parent IDs are ignored safely.

When an `invoke_agent` span summarizes a requested model and a concrete model operation such as `chat` exists below it, the summary model is suppressed. The concrete model span retains the actual model provider and evidence. This avoids counting one call twice or treating a framework provider as the model provider.

## Evidence graph

The graph contains:

- `schemaVersion`, `generatedAt`, `source`, and privacy metadata;
- stable `nodes` for agents, models, tools, MCP servers, prompts, and data sources;
- stable `edges` describing `uses`, `invokes`, `provided_by`, `connects_to`, `reads_from`, and `uses_prompt` relationships.

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

At most 20 trace identifiers are retained per node or edge in v0.6. Observation counts continue increasing after that cap.

Continuous collection merges snapshots by stable node and edge identity. It preserves the strongest evidence level, earliest and latest observation time, all observed versions, and cumulative observation counts. The latest timestamp wins when version or property values conflict.

OTLP JSON and protobuf requests are converted to the same internal observation contract. Instrumentation scope name, version, and schema URL are retained on relevant nodes as provenance. Unknown OTLP fields are ignored, and arbitrary attributes are not copied into output properties.

## CycloneDX mapping

| Evidence node | CycloneDX component type |
|---|---|
| model | `machine-learning-model` |
| data source | `data` |
| prompt | `data` |
| software | `library` |
| agent, tool, MCP server | `application` |

Evidence-specific fields are exported as namespaced CycloneDX properties beginning with `aibom:`.
