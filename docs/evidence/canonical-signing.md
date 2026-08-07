# Canonical evidence-signing validation

Date: 2026-08-07

This record tests a narrow Phase 2 claim: one fixed AI Evidence BOM graph can retain a stable cryptographic identity across harmless transport formatting and graph collection order, while a retained evidence change cannot verify under the old identity. It does not claim canonical signing for arbitrary JSON or CycloneDX, trusted signing time, confidentiality, or independent verification of a model artifact.

## Pinned boundary

- Evidence graph schema and CLI version: `0.7.0`
- Signature envelope: legacy raw-byte `0.1.0`; canonical evidence `0.2.0`
- Signature algorithm: Ed25519
- Payload digest: SHA-256 over canonical evidence bytes
- Canonical representation: project graph semantics followed by [RFC 8785 JSON Canonicalization Scheme](https://www.rfc-editor.org/rfc/rfc8785.html)
- Go JCS implementation: [`cyberphone/json-canonicalization`](https://github.com/cyberphone/json-canonicalization) commit [`19d51d7fe467d4706a3ff08adf8a748f29fc21e0`](https://github.com/cyberphone/json-canonicalization/tree/19d51d7fe467d4706a3ff08adf8a748f29fc21e0), Apache-2.0

## Canonical profile

The signature envelope records:

```json
{
  "version": "0.2.0",
  "payloadType": "aiebom-evidence-graph",
  "canonicalization": "aiebom-evidence-v1+jcs-rfc8785"
}
```

Before signing or verifying, the implementation:

1. requires valid UTF-8 and JCS-compatible JSON;
2. rejects duplicate object members, duplicate node/edge IDs, unknown evidence-graph fields, and trailing JSON values;
3. decodes the typed evidence graph;
4. sorts nodes and edges by stable ID and reduces set-like versions, sources, and trace IDs to unique sorted values;
5. writes graph and evidence timestamps in UTC without removing them;
6. applies RFC 8785 JCS and hashes the resulting bytes;
7. signs a profile-specific domain prefix plus those canonical bytes.

The explicit domain prefix prevents a canonical signature from being downgraded to a legacy exact-byte interpretation. The envelope's `createdAt` is deliberately not part of the canonical payload identity and is informational rather than proof of signing time.

## Positive and negative controls

Automated tests construct two representations of the same fixed graph that differ in:

- whitespace and object-member order;
- node and edge collection order;
- ordering and duplication of set-like evidence values;
- JSON map order;
- equivalent `+08:00` and `Z` timestamp spellings.

The test passes only when both representations produce identical canonical bytes, SHA-256 digest, and Ed25519 signature under the same key, and when either representation verifies against the same envelope.

The negative controls require:

- changing one `observationCount` value causes `payload digest mismatch`;
- duplicate JSON members are rejected;
- duplicate node or edge identities are rejected;
- unknown evidence fields are rejected;
- multiple top-level JSON values are rejected;
- a reformatted legacy raw-byte payload still fails, preserving v0.1 behavior.

## Local result

In a disposable repository copy using the locally installed Go 1.26.2 toolchain:

- `go test -race ./...` passed;
- `go vet ./...` passed;
- `go build ./cmd/aiebom` passed;
- the checksum-pinned CycloneDX positive and negative schema checks passed;
- a real `scan → keygen → sign --canonical-evidence → reformat → verify` command sequence passed;
- signing both transport representations with the same key produced identical `payloadDigest` and `signature` values;
- incrementing one real graph node's `observationCount` was rejected.

`govulncheck` did not pass on Go 1.26.2: it reported seven reachable standard-library vulnerabilities fixed by Go 1.26.5. No reachable vulnerability was reported in the newly pinned JCS dependency. The repository continues to require Go 1.26.5, so the pushed GitHub Actions run on the required toolchain is the authoritative vulnerability result; the minimum version was not weakened to make a local check green.

## Required-version and regression result

GitHub Actions [`ci #14`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/31145359216) passed on the repository-required Go 1.26.5 toolchain. Its successful steps include:

- `go test -race ./...`;
- `go vet ./...`;
- `govulncheck@v1.6.0 ./...`;
- `go build ./cmd/aiebom`;
- the checksum-pinned CycloneDX positive and negative schema controls;
- released Microsoft Agent Framework, pinned Dify instrumentation, and real MCP Go SDK compatibility jobs.

Because the implementation changed the CLI, internal signing package, and module graph, the path-triggered complete Dify application workflow also ran. [`Dify full runtime compatibility #6`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/31145359217) passed, preserving the unmodified OTLP application-runtime evidence boundary.

## Privacy and security boundary

Canonicalization is not redaction and does not read raw telemetry. It operates only on an already normalized evidence graph and signs every retained field. It introduces no prompt, model response, retrieved document, tool argument, tool result, credential, or schema-body retention.

Ed25519 proves integrity and possession of the corresponding private key; it does not establish that telemetry was truthful, that the signer was authorized, that the model artifact matched a hosted response, or when the signature was created. Key protection, access control, retention, trusted timestamps, and transparency remain operator responsibilities.

## Reproduce

From the repository root with Go 1.26.5+:

```bash
go test -race ./internal/signing ./cmd/aiebom
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
go build ./cmd/aiebom
```

The two linked GitHub Actions runs are the authoritative remote evidence for commit [`011daee`](https://github.com/Aaron911/ai-evidence-bom/commit/011daee1d04c494736e254f4d3686521e94f3afc).
