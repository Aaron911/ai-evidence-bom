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

## Evidence graph

The graph contains:

- `schemaVersion`, `generatedAt`, `source`, and privacy metadata;
- stable `nodes` for agents, models, tools, MCP servers, prompts, and data sources;
- stable `edges` describing `uses`, `invokes`, `provided_by`, `connects_to`, `reads_from`, and `uses_prompt` relationships.

Node identity is the SHA-256-derived stable key of normalized `type`, `provider`, and `name`. Version is deliberately excluded so an upgrade appears as a changed node rather than a removal and addition.

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

At most 20 trace identifiers are retained per node or edge in v0.1. Observation counts continue increasing after that cap.

## CycloneDX mapping

| Evidence node | CycloneDX component type |
|---|---|
| model | `machine-learning-model` |
| data source | `data` |
| prompt | `data` |
| software | `library` |
| agent, tool, MCP server | `application` |

Evidence-specific fields are exported as namespaced CycloneDX properties beginning with `aibom:`.

