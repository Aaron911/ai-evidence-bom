# AI Evidence BOM

AI Evidence BOM is an early, vendor-neutral prototype that turns observed GenAI and agent telemetry into a privacy-conscious evidence graph and a CycloneDX AI/ML BOM.

It answers a narrower question than an ordinary scanner:

> What models, agents, tools, MCP servers, prompts, and data sources were actually observed at runtime, and how did that set change?

The project is an experimental v0.4 validation build. It is not a compliance certification, a malware verdict engine, or a complete view of systems that are not instrumented.

## Current capabilities

- Reads OTLP JSON (`resourceSpans`) and a compact observation JSON format.
- Receives OTLP/HTTP JSON or protobuf traces at `/v1/traces` and OTLP/gRPC traces on port 4317.
- Normalizes agents, models, tools, MCP servers, prompts, and data sources into a stable graph.
- Uses OTLP trace parentage to associate model and tool child spans with the correct agent and to avoid duplicate framework-summary model nodes.
- Includes source-derived compatibility contracts for Dify and Microsoft Agent Framework.
- Records evidence as `inferred`, `declared`, `observed`, or `verified`.
- Exports CycloneDX 1.7 JSON with AI/ML component types and relationships.
- Compares two evidence graphs to find new, removed, and changed capabilities.
- Enforces JSON policies suitable for CI gates.
- Signs raw graph or BOM files with Ed25519 and detects tampering.
- Defaults to metadata-only processing. Prompt bodies and tool arguments are never retained.
- Bounds live request sizes, supports gzip, optionally authenticates with a bearer token, and deduplicates recent span retries.

## Quick start

Requirements: Go 1.26.5 or later. Earlier Go 1.26 patch releases contain standard-library vulnerabilities fixed in 1.26.5.

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
  --output work/after.evidence.sig.json

./bin/aiebom verify \
  --input work/after.evidence.json \
  --public-key work/evidence-public.pem \
  --signature work/after.evidence.sig.json
```

Signatures cover the exact file bytes. Reformatting a signed JSON file invalidates the signature.

## Evidence levels

| Level | Meaning |
|---|---|
| `inferred` | Derived by a heuristic and not directly asserted by the source. |
| `declared` | Present in configuration or supplied metadata. |
| `observed` | Seen in a runtime event or trace. |
| `verified` | Backed by an independently verified digest or signature assertion. |

Higher evidence does not mean that a component is safe. It means only that its identity has stronger support.

## Architecture

```text
OTLP/HTTP JSON or protobuf ──┐
OTLP/gRPC protobuf ──────────┼──> live receiver ──┐
                            │                    v
OTLP JSON files / declarations ────────> evidence normalizer
                                                 |
                                                 v
                                      vendor-neutral evidence graph
                                          |          |          |
                                          v          v          v
                                    CycloneDX      diff       policy/sign
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
  "forbidInferred": true
}
```

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
- `mcp.server.*`, `mcp.tool.*`
- selected OpenTelemetry resource attributes such as `service.name` and `service.version`

Standard `invoke_agent {agent.name}` and `execute_tool {tool.name}` span names are used as fallbacks when an attribute is unavailable. Instrumentation scope name, version, and schema URL are retained as provenance metadata. The implementation tracks the developing [OpenTelemetry GenAI semantic conventions](https://github.com/open-telemetry/semantic-conventions-genai) rather than defining a competing telemetry vocabulary.

OTLP `traceId`, `spanId`, and `parentSpanId` are used together during normalization. Explicit `gen_ai.agent.*` identity is inherited by descendant spans in the same trace. For Dify, `dify.app_id` is treated as the stable agent identity and is propagated in the same way.

See [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md) for exact framework coverage and evidence levels, and [docs/SCHEMA.md](docs/SCHEMA.md) for the compact input and graph contracts.

## Project boundaries

This project does not currently:

- capture traffic from closed-source clients without instrumentation;
- ingest OTLP metrics, logs, or profiles; v0.4 deliberately accepts traces only;
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
