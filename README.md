# AI Evidence BOM

AI Evidence BOM is an early, vendor-neutral prototype that turns observed GenAI and agent telemetry into a privacy-conscious evidence graph and a CycloneDX AI/ML BOM.

It answers a narrower question than an ordinary scanner:

> What models, agents, tools, MCP servers, prompts, and data sources were actually observed at runtime, and how did that set change?

The project is an experimental v0.11 validation build. It is not a compliance certification, a malware verdict engine, or a complete view of systems that are not instrumented.

## Current capabilities

- Reads OTLP JSON (`resourceSpans`) and a compact observation JSON format.
- Receives OTLP/HTTP JSON or protobuf traces at `/v1/traces` and OTLP/gRPC traces on port 4317.
- Normalizes agents, models, tools, MCP servers, prompts, and data sources into a stable graph.
- Uses OTLP trace parentage across export batches to associate model and tool child spans with the correct agent and to avoid duplicate framework-summary model nodes.
- Includes source-derived contracts plus executable compatibility checks for Dify and Microsoft Agent Framework.
- Uses MCP protocol discovery and runtime telemetry together to distinguish declared server capabilities from observed tool calls.
- Records evidence as `inferred`, `declared`, `observed`, or `verified`.
- Caps every source at `observed` by default, preserves `verified` only for exact operator-authorized sources, and can bind protected live sources to ingest-only credentials outside OTLP.
- Retains field-level evidence for versions, digests, and properties, resolves them independently of arrival order, and exposes competing values as conflicts.
- Imports a bounded, metadata-only SARIF 2.1.0 subset from external scanners and attaches findings only when a runtime component's artifact URI and SHA-256 agree.
- Exports CycloneDX 1.7 JSON with AI/ML component types and relationships, validated in CI against the checksum-pinned official schema.
- Compares two evidence graphs to find new, removed, and changed capabilities.
- Enforces node and directed graph-path JSON policies suitable for CI gates.
- Can fail policy when conflicting field evidence exists, even when the strongest selected value did not change.
- Signs exact graph/BOM bytes or RFC 8785-canonical evidence-graph identities with Ed25519 and detects tampering.
- Defaults to metadata-only processing. Prompt bodies and tool arguments are never retained.
- Bounds live request sizes, supports gzip, separates global/read access from source-specific ingest credentials, and deduplicates recent span retries.

## Quick start

Requirements: Go 1.26.6 or later. Earlier Go 1.26 patch releases contain reachable standard-library vulnerabilities fixed in 1.26.6.

```bash
go install github.com/Aaron911/ai-evidence-bom/cmd/aiebom@latest
```

Or build from a checkout:

```bash
go build -o ./bin/aiebom ./cmd/aiebom

./bin/aiebom scan \
  --input examples/otlp-before.json \
  --graph-out work/before.evidence.json \
  --bom-out work/before.cdx.json

./bin/aiebom scan \
  --input examples/otlp-after.json \
  --graph-out work/after.evidence.json \
  --bom-out work/after.cdx.json

./bin/aiebom diff \
  --before work/before.evidence.json \
  --after work/after.evidence.json \
  --output work/diff.json

./bin/aiebom policy \
  --input work/after.evidence.json \
  --policy examples/policy.json \
  --output work/policy-report.json
```

The sample policy intentionally rejects the new `shell.execute` capability and exits with status 3.

### Framework compatibility checks

The repository includes deterministic checks that require no model API key or paid call:

```bash
scripts/live/verify_agent_framework.sh
scripts/live/verify_dify_instrumentation.sh
scripts/live/verify_dify_runtime.sh
scripts/live/verify_mcp_runtime.sh
scripts/live/verify_sarif_bridge.sh
```

The Microsoft check runs the released Agent Framework core, including its real Agent, chat telemetry, function invocation, tool execution, and OTLP exporter paths. The lightweight Dify check executes the pinned 1.16.1 OTel workflow handler and node parsers in isolation. The full Dify check starts an official minimal application stack and verifies its unmodified OTLP export. The MCP check runs an official Go SDK v1.7.0 client and server over stdio, obtains stable identity from `server/discover`, executes `tools/list` and `tools/call`, and proves that an added declared capability is caught by graph diff and a directed path policy. The SARIF check runs gosec 2.28.0 against the real MCP fixture and validates both SARIF and the resulting CycloneDX VDR document against checksum-pinned official schemas. No check needs a model API key or paid call. See [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md), [the v0.6 Dify record](docs/evidence/v0.6.0.md), [the v0.7 MCP record](docs/evidence/v0.7.0.md), and [the v0.11 SARIF record](docs/evidence/v0.11.0.md) for exact evidence grades and prerequisites.

### Attach external SARIF findings

AI Evidence BOM does not replace a source scanner. An operator first runs a scanner that emits [SARIF 2.1.0](https://docs.oasis-open.org/sarif/sarif/v2.1.0/sarif-v2.1.0.html), then joins its results to an already observed component:

```bash
./bin/aiebom sarif \
  --input work/runtime.evidence.json \
  --sarif work/gosec.sarif \
  --artifact scripts/live/mcp_runtime/main.go \
  --sarif-artifact-uri main.go \
  --output work/enriched.evidence.json \
  --bom-out work/enriched.cdx.json

./bin/aiebom policy \
  --input work/enriched.evidence.json \
  --policy examples/policy-sarif-error.json \
  --output work/sarif-policy.json
```

`--artifact` is a repository-relative regular file used to derive the stable graph URI and SHA-256. `--sarif-artifact-uri` is needed only when the scanner reports a different scan-root-relative URI; it is an explicit exact mapping, not a name heuristic. The target graph must already contain the same artifact URI and selected digest, with no conflict. A digest mismatch, URI-only/name-only match, ambiguous target, unsafe path, failed scanner invocation, or malformed binding-critical SARIF is rejected.

Only scanner name/version, rule ID, standard SARIF level (`note`, `warning`, or `error`), target URI/digest, occurrence count, and the `affected_by` relationship survive. Messages, snippets, regions, call flows, fixes, and source content are discarded. A finding means “this external scanner reported this rule,” not that AI Evidence BOM verified exploitability or safety. Finding nodes become CycloneDX `vulnerabilities`; they are not mislabeled as software components, and SARIF `error` is not translated into a CycloneDX `high` or `critical` rating.

### Live OTLP collection

Start a local receiver:

```bash
./bin/aiebom collect \
  --listen 127.0.0.1:4318 \
  --grpc-listen 127.0.0.1:4317 \
  --graph-out work/live.evidence.json \
  --bom-out work/live.cdx.json
```

Send an OTLP JSON export request:

```bash
curl --fail-with-body \
  -H 'Content-Type: application/json' \
  --data-binary @examples/otlp-before.json \
  http://127.0.0.1:4318/v1/traces
```

The HTTP receiver also accepts `application/x-protobuf`; the gRPC listener implements the standard OTLP `TraceService/Export` method. It exposes `GET /healthz`, `/v1/evidence`, `/v1/bom`, and `/v1/stats` over HTTP. Only health is intentionally unauthenticated when a token is configured.

To place an OpenTelemetry Collector in front of the receiver, set `AIEBOM_TOKEN` and use [the OTLP/HTTP protobuf example](examples/otel-collector-http.yaml) or [the OTLP/gRPC example](examples/otel-collector-grpc.yaml). See [docs/RUNTIME_RECEIVER.md](docs/RUNTIME_RECEIVER.md) for authentication, limits, persistence, and deployment boundaries.

### Optional private prompt fingerprints

Prompt content is ignored by default. To detect prompt changes without storing the prompt, supply a secret HMAC key containing at least 32 bytes:

```bash
./bin/aiebom scan \
  --input examples/otlp-before.json \
  --graph-out work/before.evidence.json \
  --sensitive-hmac-key-file /secure/path/prompt-hmac.key
```

Use the same protected key for later scans. The key is never written to the graph. Ordinary, unkeyed hashes are deliberately not used because short prompts can be susceptible to dictionary guessing.

### Sign and verify evidence

```bash
./bin/aiebom keygen \
  --private-key work/evidence-private.pem \
  --public-key work/evidence-public.pem

./bin/aiebom sign \
  --input work/after.evidence.json \
  --private-key work/evidence-private.pem \
  --canonical-evidence \
  --output work/after.evidence.sig.json

./bin/aiebom verify \
  --input work/after.evidence.json \
  --public-key work/evidence-public.pem \
  --signature work/after.evidence.sig.json
```

`--canonical-evidence` is the recommended mode for evidence graphs. It strictly decodes the graph, rejects duplicate or unknown JSON members and inconsistent field-evidence selections, normalizes the graph's set-like collections and timestamps, and then applies [RFC 8785 JSON Canonicalization Scheme](https://www.rfc-editor.org/rfc/rfc8785.html). Equivalent whitespace, JSON object-member order, node/edge order, source order, and UTC-offset spelling therefore produce the same payload digest and deterministic Ed25519 signature for the same key. Any retained evidence-field change changes the digest and fails verification.

New signatures use envelope v0.3 with `payloadType=aiebom-evidence-graph` and `canonicalization=aiebom-evidence-v2+jcs-rfc8785`, which includes v0.8 field-evidence ordering and timestamp semantics. `verify` reads these fields and never guesses the mode; existing v0.2/v1 canonical envelopes and v0.1 exact-byte envelopes remain verifiable. The default without `--canonical-evidence` remains the exact-byte mode, which can also sign CycloneDX or another file and is invalidated by reformatting. Canonical mode currently supports AI Evidence BOM graphs only; it is not a CycloneDX JSF/JWS implementation. Envelope `createdAt` is informational and is not part of the signed canonical identity.

See the [canonical-signing evidence record](docs/evidence/canonical-signing.md) for the pinned dependency, positive and negative controls, and validation boundary.

### Authorize verifier sources

Compact input cannot grant itself `verified` authority. Without a source trust policy, any `verified` claim is reduced to `observed`. To authorize a separately controlled verifier adapter, use an exact-source policy:

```json
{
  "version": "0.1.0",
  "sources": [
    {"source": "fixture-signature-verifier", "maxEvidence": "verified"},
    {"source": "fixture-deployment-config", "maxEvidence": "declared"}
  ]
}
```

```bash
./bin/aiebom scan \
  --input examples/conflicting-model-evidence.json \
  --source-trust-policy examples/source-trust-policy.json \
  --graph-out work/conflict.evidence.json
```

Rules match the complete source name exactly and case-sensitively; there are no wildcard or prefix grants. A rule can also reduce a source to `observed`, `declared`, or `inferred`. The same flag is available on `collect`, and `/v1/stats` reports `evidenceDowngrades`.

For live collection, any trust grant above `observed` additionally requires `--source-auth-policy`. Its versioned bindings map SHA-256 digests of random bearer tokens to exact source names:

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

The producer sends the raw token through the standard OTLP `Authorization` header. A valid source token overrides the authority label derived from `service.name`; an unbound producer cannot claim that protected source. Multiple token digests may map to the same source during rotation, and source credentials cannot read graph/BOM/stat endpoints. Authentication identifies the configured producer but does not make OTLP evidence `verified` or prove safety. See [runtime source authentication](docs/RUNTIME_RECEIVER.md#bind-a-live-producer-to-an-evidence-source), the [v0.9 source-trust record](docs/evidence/v0.9.0.md), and the [v0.10 source-authentication record](docs/evidence/v0.10.0.md).

## Evidence levels

| Level | Meaning |
|---|---|
| `inferred` | Derived by a heuristic and not directly asserted by the source. |
| `declared` | Present in configuration or supplied metadata. |
| `observed` | Seen in a runtime event or trace. |
| `verified` | Supplied by a separately controlled non-OTLP verifier adapter whose exact source is authorized by operator policy and, for live transport, authenticated outside the payload; built-in model-artifact verification is not implemented yet. |

Higher evidence does not mean that a component is safe. It means only that its identity has stronger support.

For mutable fields, the selected value is the candidate with the strongest evidence, then the latest observation time, then lexical order as a deterministic tie-breaker. All competing candidates remain in `fieldEvidence`, and `conflict=true` prevents a weaker but newer declaration from silently replacing stronger identity evidence. Ordinary OTLP attributes, including signature-looking assertions, cannot promote evidence to `verified`; compact input also remains capped unless an exact source rule authorizes it.

The reproducible conflict fixture and its positive, negative, and migration controls are documented in the [v0.8 field-evidence record](docs/evidence/v0.8.0.md).

## Architecture

```text
OTLP/HTTP JSON or protobuf ──┐
OTLP/gRPC protobuf ──────────┼──> live receiver ──> source authentication ──┐
                                                                           │
OTLP JSON files / declarations ──> scan ───────────────────────────────────┤
                                                                           v
                                                                source trust caps
                                                                           |
                                                                           v
                                                                evidence normalizer
                                                                           |
                                                                           v
                                                              runtime evidence graph ─┐
                                                                                      │
external SARIF + local artifact ──> exact URI/SHA-256 binding ────────────────────────┤
                                                                                      v
                                                                        enriched graph
                                                                         |     |     |
                                                                         v     v     v
                                                                  CycloneDX  diff  policy/sign
```

The internal graph is the source of truth. Export formats are adapters so the core is not coupled to one BOM standard.

## Policy example

Policies are JSON:

```json
{
  "version": "0.1.0",
  "minimumEvidence": {"model": "observed", "tool": "observed"},
  "allowedProviders": ["openai", "local-mcp"],
  "requireProvidersFor": ["model", "tool"],
  "deniedNamePatterns": ["(?i)shell|execute"],
  "requireVersionsFor": ["model", "tool"],
  "deniedFindingLevels": ["error"],
  "forbidInferred": true,
  "forbidFieldConflicts": true
}
```

Directed path policies can reject reachability without claiming that every declared tool was invoked. For example, [examples/policy-mcp-capability.json](examples/policy-mcp-capability.json) rejects `agent -[connects_to]-> mcp_server -[provides]-> shell.execute` when the agent/server relationship is observed and the capability is at least declared.

Exit codes:

- `0`: success or policy passed;
- `1`: invalid input or operational error;
- `2`: graph changed when `diff --fail-on-change` is used;
- `3`: policy violation.

## Supported attributes

The normalizer accepts current and common legacy forms including:

- `gen_ai.agent.*`
- `gen_ai.request.model`, `gen_ai.response.model`, `gen_ai.provider.name`
- `gen_ai.tool.*`, `gen_ai.tool.call.name`
- `gen_ai.data_source.id`, `gen_ai.retrieval.data_source.id`
- `gen_ai.prompt.template.*`, `gen_ai.system_instructions`
- standard MCP telemetry including `mcp.method.name`, `mcp.protocol.version`, and `network.transport`
- `aiebom.mcp.server.*` and `aiebom.mcp.tool.*` project extensions for protocol-derived identity and content-free capability metadata
- legacy `mcp.server.*` and `mcp.tool.*` compatibility aliases; these are not presented as OpenTelemetry standard attributes
- selected OpenTelemetry resource attributes such as `service.name` and `service.version`

Standard `invoke_agent {agent.name}` and `execute_tool {tool.name}` span names are used as fallbacks when an attribute is unavailable. Instrumentation scope name, version, and schema URL are retained as provenance metadata. The implementation tracks the developing [OpenTelemetry GenAI semantic conventions](https://github.com/open-telemetry/semantic-conventions-genai). Those conventions currently define MCP method, protocol, transport, and tool-call attributes but no stable logical server-name attribute, so v0.7 uses an explicitly documented `aiebom.*` bridge populated from MCP `serverInfo` rather than mislabeling a project field as a standard.

OTLP `traceId`, `spanId`, and `parentSpanId` are used together during normalization, including when related spans arrive in separate export requests. Explicit `gen_ai.agent.*` identity is inherited by descendant spans in the same trace. For Dify, `dify.app_id` is treated as the stable agent identity and is propagated in the same way. Unresolved children wait in a bounded metadata-only queue; content-bearing attributes are discarded before queuing.

See [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) for exact framework coverage and evidence levels, and [docs/SCHEMA.md](docs/SCHEMA.md) for the compact input and graph contracts.

## Project boundaries

This project does not currently:

- capture traffic from closed-source clients without instrumentation;
- ingest OTLP metrics, logs, or profiles; v0.11 deliberately accepts traces only;
- make the official MCP SDK emit OTLP automatically; the v0.7 live check installs application instrumentation around its real SDK calls;
- trust MCP tool descriptions, schemas, or annotations as proof of safe behavior;
- scan source code itself, infer SARIF paths, or treat scanner findings as verified vulnerabilities;
- prove which weights a hosted model provider actually served;
- retain prompt, completion, tool argument, or tool result content;
- declare a prompt, model, or tool safe;
- provide automatic remediation or a web dashboard;
- claim AI Act, NIST, SPDX, or CycloneDX certification.

The roadmap and validation gates are in [docs/ROADMAP.md](docs/ROADMAP.md). Each release's direction check is recorded in [docs/DIRECTION.md](docs/DIRECTION.md). Security and privacy decisions are documented in [docs/THREAT_MODEL.md](docs/THREAT_MODEL.md) and [docs/PRIVACY.md](docs/PRIVACY.md).

Version history is recorded in [CHANGELOG.md](CHANGELOG.md).

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
go build ./cmd/aiebom
```

Contributions should include fixtures or tests for every new telemetry adapter. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0. See [LICENSE](LICENSE).
