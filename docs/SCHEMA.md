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

Signature-looking OTLP attributes such as `gen_ai.model.signature.verified=true` are untrusted metadata and do not change the evidence level. Compact input is also capped at `observed` by default. A compact-input adapter may retain `verified` only after performing verification outside the current normalizer and only when the operator authorizes its exact source name; the built-in OpenSSF Model Signing verifier remains roadmap work.

## Source trust policy

`scan` and `collect` accept `--source-trust-policy`. The policy is vendor-neutral and contains exact, case-sensitive source names with their maximum evidence authority:

```json
{
  "version": "0.1.0",
  "sources": [
    {"source": "model-signing-verifier", "maxEvidence": "verified"},
    {"source": "deployment-config", "maxEvidence": "declared"}
  ]
}
```

Sources without a matching rule have a fixed maximum of `observed`. Rules do not use wildcards, prefixes, framework names, or transport-specific code. A rule can grant `verified` or impose the stricter `observed`, `declared`, or `inferred` maximum. Invalid levels, unknown fields, unsupported policy versions, empty sources, duplicate exact-source rules, and trailing JSON values are rejected before collection starts.

The cap runs before normalization and before unresolved spans can enter the cross-batch correlation queue. When continuous collection loads an existing graph, it also reapplies the current policy to node, edge, and field-candidate summaries and recomputes selected fields, so pre-v0.9 untrusted verification does not survive a receiver restart. The cap changes only evidence levels; it does not copy raw input or add attributes.

## Live source authentication policy

`collect` additionally accepts `--source-auth-policy`. This versioned policy binds SHA-256 digests of high-entropy bearer credentials to exact source names outside the OTLP payload:

```json
{
  "version": "0.1.0",
  "bindings": [
    {
      "source": "model-signing-verifier",
      "tokenSha256": "0000000000000000000000000000000000000000000000000000000000000000"
    }
  ]
}
```

Each token digest must be unique. Multiple digests may map to the same source so credentials can overlap during rotation without changing graph identity. Unknown fields, unsupported versions, empty bindings or sources, non-canonical digests, duplicate digests, and trailing JSON are rejected. Presented source credentials must contain at least 32 bytes.

For a correctly authenticated HTTP or gRPC export, the bound source replaces the observation authority label that would otherwise be derived from `service.name`. If a request authenticated only with the global receiver token self-reports a protected source, the whole request is rejected before deduplication, normalization, pending correlation, or persistence. Trust rules above `observed` are rejected at receiver startup unless their exact source is protected by an authentication binding. Source credentials grant ingestion only and do not authorize the evidence, BOM, or stats read endpoints.

This binding authenticates the configured producer, not the truth or safety of its claims. OTLP evidence remains capped at `observed`; source authentication does not turn telemetry into verification. Offline `scan` input remains an operator-controlled file boundary and does not use the live transport policy.

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
| `aiebom.artifact.uri` | Operator-supplied repository-relative source artifact URI for an MCP server. |
| `aiebom.artifact.sha256` | Operator-computed SHA-256 of that exact artifact; normalized into `digests.sha256`. |

These are AI Evidence BOM extensions, not OpenTelemetry standard attributes. Legacy `mcp.server.*` and `mcp.tool.*` aliases remain readable for compatibility but have the same non-standard status. Descriptions, schema bodies, arguments, and results are never graph properties.

`traceId`, `spanId`, and `parentSpanId` form normalization context rather than graph content. A child model or tool span inherits the nearest explicit agent identity from its ancestors in the same trace. A nested span with its own explicit agent identity starts a new context. Cycles, missing parents, and cross-trace parent IDs are ignored safely.

When an `invoke_agent` span summarizes a requested model and a concrete model operation such as `chat` exists below it, the summary model is suppressed. The concrete model span retains the actual model provider and evidence. This avoids counting one call twice or treating a framework provider as the model provider.

## External SARIF finding bridge

`aiebom sarif` accepts one existing graph, one external SARIF 2.1.0 document, and one repository-relative regular file. The command hashes the file at import time and finds exactly one non-finding graph node whose selected `aiebom.artifact.uri` and `digests.sha256` match without field conflict. URI-only, digest-only, display-name, ambiguous, conflicted, absolute, parent-traversing, symlink, and digest-mismatch bindings are rejected.

The SARIF result location must equal the derived artifact URI. When a scanner reports a path relative to a different scan root, `--sarif-artifact-uri` provides that second exact URI. This is an explicit operator mapping; the graph identity and digest still come from `--artifact`, and the importer does not strip prefixes, search suffixes, compare basenames, or inspect source content to guess a target. Absolute/file URIs and unresolved `uriBaseId` values are rejected; callers must provide one resolved, canonically escaped relative URI.

The core validates a bounded binding-critical subset of SARIF rather than duplicating its complete extensible JSON Schema: version `2.1.0`, 1–32 runs, scanner identity/version, up to 20,000 rules and 10,000 results per run, rule ID/index consistency, successful invocations, valid result kind/level, suppression state, and up to 32 locations per result. SARIF and the artifact are each limited to 16 MiB. The live gate separately validates real gosec output against the checksum-pinned official OASIS schema.

Only unsuppressed `fail` results for the exact SARIF artifact URI become graph nodes. A suppression with absent or `accepted` status is treated as active; `underReview` and `rejected` findings remain visible to the security gate. Duplicate occurrences of one scanner/rule/artifact identity merge into one `finding` node whose evidence `observationCount` records result occurrences. Its selected metadata is restricted to:

- scanner name and optional version;
- rule ID;
- SARIF level `note`, `warning`, or `error`;
- `sarif-2.1.0` format and `scanner-reported` assertion;
- target repository URI and SHA-256.

The target component points to the finding with `affected_by`. SARIF messages, markdown, snippets, regions, stacks, call flows, fixes, taxonomies, and source content are neither copied nor fingerprinted. `observed` describes the scanner execution/report observation; `scanner-reported` explicitly prevents it from being interpreted as independently verified exploitability.

`deniedFindingLevels` can reject exact standard SARIF levels. It deliberately accepts only `none`, `note`, `warning`, and `error`; these are result-reporting levels, not CVSS/CycloneDX severity ratings.

## Evidence graph

The v0.11 graph keeps the existing top-level structure and adds a `finding` node contract plus `affected_by` edges for externally reported SARIF results. The graph contains:

- `schemaVersion`, `generatedAt`, `source`, and privacy metadata;
- stable `nodes` for agents, models, tools, MCP servers, prompts, data sources, and scanner findings;
- stable `edges` describing `uses`, `invokes`, `provides`, `connects_to`, `reads_from`, `uses_prompt`, and `affected_by` relationships.

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

`deniedFindingLevels` is independent of graph-path matching. See [`examples/policy-sarif-error.json`](../examples/policy-sarif-error.json) for a gate that rejects any selected or conflicting candidate SARIF `error` level.

## CycloneDX mapping

| Evidence node | CycloneDX component type |
|---|---|
| model | `machine-learning-model` |
| data source | `data` |
| prompt | `data` |
| software | `library` |
| agent, tool, MCP server | `application` |

Finding nodes are not exported as components. Each becomes a CycloneDX `vulnerabilities` entry whose `id` is the SARIF rule ID, `source.name` is the scanner, and `affects[].ref` points to the affected component BOM reference. Standard SARIF levels are retained as `aibom:sarif:level` properties rather than incorrectly translated into CycloneDX ratings.

Evidence-specific fields are exported as namespaced CycloneDX properties beginning with `aibom:`. Field candidate properties contain only values already retained by the evidence graph; they do not add model input, output, prompt, credential, tool argument, tool result, SARIF message, or source content.
