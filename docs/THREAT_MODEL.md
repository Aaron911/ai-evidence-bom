# Threat model

## Protected assets

- integrity of the evidence graph and exported BOM;
- confidentiality of prompt, response, tool, and credential content;
- correctness of component identities and relationships;
- auditability of changes between collection windows.

## Trust boundaries

1. Telemetry producers may be buggy, compromised, or dishonest.
2. The normalizer processes untrusted JSON and protobuf telemetry.
3. Evidence files may be modified after generation.
4. Policy authors are trusted to define organizational intent.
5. Hosted model providers remain outside the verifier's control.

## Addressed through v0.8

- Evidence provenance is explicit rather than collapsing declarations and observations.
- Prompt and tool content is not retained.
- Optional prompt fingerprints use a secret HMAC key rather than an unsalted digest.
- Stable identities make unexpected capability and version drift visible.
- Exact evidence files can be signed with Ed25519 and verified later.
- Evidence graphs can instead be signed through an explicit RFC 8785-based canonical profile, so harmless transport reformatting and graph collection order do not change their signed identity.
- Policy failures use a distinct exit code for CI.
- Key generation refuses to overwrite existing key files.
- The live receiver defaults to loopback, requires a bearer token for non-loopback binds, and compares tokens in constant time.
- OTLP/HTTP request bodies are limited before and after gzip decompression; OTLP/gRPC receive messages use the same configured limit. The default is 64 MiB.
- HTTP and gRPC bearer tokens use constant-time comparison and share the same authorization rule.
- Recent trace/span pairs are deduplicated to prevent common OTLP retries from inflating evidence counts.
- Cross-batch parent correlation retains only bounded, allowlisted metadata; content fields are removed before an unresolved child is queued.
- Evidence snapshots are written through atomic replacement rather than in-place truncation.
- MCP `serverInfo` identity and `tools/list` capabilities are retained as declarations; only runtime calls upgrade tool evidence to observed.
- Directed path policies detect reachability such as an observed Agent connection to a server that declares a denied tool, without falsely claiming the tool was invoked.
- MCP descriptions, schema bodies, arguments, and results are discarded; only bounded identity metadata, annotation hints, and an input-schema digest survive.
- The OTLP evidence-level extension can only downgrade evidence and cannot self-promote a span to verified.
- Signature-looking OTLP attributes cannot promote model evidence to verified; verification must occur in a separately trusted adapter.
- Versions, digests, and retained properties keep field-specific candidate evidence. Stronger evidence wins before recency, competing values remain visible, and policy can reject conflicts.
- Merge selection is independent of arrival order, including a deterministic lexical tie-breaker when strength and time are equal.
- Legacy graphs without field provenance are migrated as inferred field candidates instead of inheriting a potentially misleading strong node summary.

## Known limitations

- An observed trace proves only that an instrumented component reported an event; it does not prove the event is truthful.
- `verified` compact observations still depend on the caller selecting and trusting an external verifier adapter. Independent built-in OpenSSF Model Signing verification is planned.
- `scan` input files are loaded into memory and do not yet have a configurable size limit; live collection is bounded.
- The built-in HTTP and gRPC servers do not terminate TLS. Remote use requires a trusted TLS proxy and access controls.
- Retry deduplication is bounded and in-memory, so its history resets on restart and very old duplicates may be counted again.
- A sampled-out or missing parent can leave child metadata pending until queue pressure; monitor `pendingSpans`. Pending context is not persisted across restarts.
- Only OTLP traces are accepted; metrics, logs, and profiles are outside the v0.8 protocol scope.
- There is no sandbox around input parsing.
- Hosted model aliases and weights cannot be independently verified.
- Exact-byte signing remains the default and is formatting-sensitive. Canonical signing is opt-in and currently accepts AI Evidence BOM graph JSON only; it does not implement CycloneDX JSF/JWS or canonicalize arbitrary files.
- Canonical signatures cover every retained graph field, including meaningful evidence timestamps. Re-running collection at a new time is not the same fixed snapshot and is expected to change the digest.
- Signature-envelope `createdAt` is informational and is not authenticated by the payload signature. Use a separate trusted timestamp or transparency service when signing time itself must be proven.
- MCP `serverInfo`, `tools/list`, and annotations are statements from an untrusted server. They do not prove the implementation is benign or that its runtime behavior matches its schema.
- Current OpenTelemetry MCP conventions do not define stable logical server identity. The `aiebom.mcp.server.*` bridge is an explicit project extension derived from protocol discovery, not a standard attribute.
- The official Go MCP SDK does not emit OTel automatically. v0.7 validates application instrumentation around real SDK calls, not universal zero-code capture or server-side trace-context propagation.
- Path policy matches exact directed relation sequences and does not yet support counts, time windows, negative reachability, or aggregate behavior.
- A selected field value is the strongest-supported candidate, not necessarily the most recently deployed value. Consumers must inspect `conflict` or enable `forbidFieldConflicts` when ambiguity is unacceptable.
- Distinct field candidates and `observedVersions` are not yet cardinality-bounded. A malicious high-cardinality producer can grow a long-lived graph even though request sizes and trace-ID retention are bounded.

Security issues should follow [SECURITY.md](../SECURITY.md), not a public issue containing sensitive reproduction material.
