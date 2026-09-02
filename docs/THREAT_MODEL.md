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

## Addressed through v0.11

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
- Every source defaults to a maximum evidence level of observed. Only an exact, case-sensitive operator rule can preserve verified evidence, and source rules can impose stricter caps.
- Source trust policies reject unknown fields, invalid levels, unsupported versions, empty or duplicate source rules, and trailing JSON before ingestion begins.
- A live receiver reapplies current source caps to persisted node, edge, and field-candidate summaries, preventing pre-v0.9 untrusted verification from silently surviving restart.
- Live HTTP and gRPC source credentials are carried outside OTLP and bind directly to exact evidence sources, so `service.name` is no longer sufficient to impersonate a protected source.
- A request using only the global receiver token cannot assert a source protected by a source credential; it is rejected before deduplication or pending correlation, preventing an unbound claim from poisoning retry state.
- Source authentication policies strictly validate versioned, unique SHA-256 credential digests. Presented tokens must contain at least 32 bytes, multiple rotation credentials may retain one stable source identity, and global/read credentials cannot be reused as source credentials.
- Source credentials authorize ingestion only. They cannot read the evidence graph, BOM, or receiver statistics when those endpoints are protected by the global token.
- External SARIF findings attach only to one graph component whose repository-relative artifact URI and selected SHA-256 match without conflict; display-name and URI-only matching are not used.
- SARIF input, run/rule/result/location counts, artifact size, strings, rule/index consistency, invocation success, result kind/level, and relative paths are bounded or validated before findings are merged.
- SARIF content fields are discarded. Only scanner/rule/level and exact target metadata survive, and the assertion is labeled `scanner-reported` rather than verified.
- SARIF `error` is kept as a reporting level and is not converted into an unsupported CVSS or CycloneDX severity claim.

## Known limitations

- An observed trace proves only that an instrumented component reported an event; it does not prove the event is truthful.
- Source authentication proves possession of a static bearer credential, not that telemetry is truthful or that a component is safe. Credentials currently have no intrinsic expiry, audience, or hardware-backed identity; rotate or remove policy bindings after suspected exposure. Independent built-in OpenSSF Model Signing verification is still planned.
- Source authentication applies to the live `collect` transport. Compact `scan` documents remain operator-supplied files whose source labels are not authenticated by this mechanism.
- `scan` input files are loaded into memory and do not yet have a configurable size limit; live collection is bounded.
- The built-in HTTP and gRPC servers do not terminate TLS. Remote use requires a trusted TLS proxy and access controls.
- Retry deduplication is bounded and in-memory, so its history resets on restart and very old duplicates may be counted again.
- A sampled-out or missing parent can leave child metadata pending until queue pressure; monitor `pendingSpans`. Pending context is not persisted across restarts.
- Only OTLP traces are accepted; metrics, logs, and profiles are outside the v0.11 protocol scope.
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
- SARIF is untrusted scanner output. A report can contain false positives, false negatives, misleading rule IDs/levels, or forged scanner identity; import proves only deterministic correlation to the current file digest, not scanner authenticity or exploitability.
- `--sarif-artifact-uri` is an operator assertion needed when the scanner uses another scan root. Exact matching prevents heuristic collisions, but a malicious or mistaken operator can still provide a false mapping.
- v0.11 identifies only one regular source file at a time. It does not bind a built binary, container image, repository tree, transitive dependency set, multi-file MCP server, or Skill package.
- The core validates only the bounded SARIF fields needed for binding. Operators needing complete format validation should use the official OASIS schema; the executable gate does so for the pinned gosec fixture.

Security issues should follow [SECURITY.md](../SECURITY.md), not a public issue containing sensitive reproduction material.
