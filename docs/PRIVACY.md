# Privacy model

AI telemetry may contain source code, credentials, personal data, system prompts, retrieved documents, tool parameters, and model responses. The project therefore uses a metadata-only default.

## Never retained by the v0.10 normalizer

- prompt and completion bodies;
- tool call arguments and results;
- retrieved document content;
- API keys, tokens, cookies, and environment variable values;
- model input and output attachments;
- MCP tool descriptions, raw input/output schemas, arguments, and results.

These attributes may be present in an input OTLP document, but the normalizer does not copy them into the graph.

The Dify, Microsoft Agent Framework, and MCP executable checks intentionally contain marker values in prompt, input, output, tool-description/schema, tool-argument, and tool-result fields. Automated checks fail if any marker reaches the normalized graph or CycloneDX output.

MCP `tools/list` declarations retain only tool names, server identity/version, selected untrusted boolean annotation hints, and a SHA-256 of the adapter's JSON encoding of the input schema. The digest detects schema drift but does not make the schema safe or trustworthy. Tool names and schema digests can still be sensitive metadata.

Field conflict tracking retains distinct historical values for versions, digests, and already allowlisted properties rather than only the selected value. CycloneDX exports these candidates as namespaced properties so evidence strength and conflicts remain auditable. This does not broaden the attribute allowlist or retain content, but it can increase the amount and lifetime of operational metadata; apply the same access control and retention policy to candidate values as to the rest of the graph.

Source trust policies contain only exact source labels and maximum evidence levels. Applying a cap changes the in-memory evidence level before normalization; it does not retain the original higher claim, add input fields to the graph, or inspect prompt/model/tool content. Source labels can still reveal operational names and should be protected as evidence metadata.

Source authentication policies contain exact source labels and SHA-256 digests of randomly generated, high-entropy bearer credentials. Raw source credentials exist only in the producer and receiver request/authentication path: they are not copied into observations, logs, graphs, BOMs, stats, or pending correlation state. A digest is still authentication configuration and must be access-controlled; it must not be used as a substitute for password hashing when credentials are human-chosen or low entropy.

## Prompt change detection

Without an explicit `--sensitive-hmac-key-file`, prompt content is neither stored nor fingerprinted. When a key is supplied, the normalizer writes only an HMAC-SHA-256 fingerprint and discards the content. The same protected key must be reused to compare future scans.

The key should be random, at least 32 bytes, stored separately from evidence files, and rotated according to the operator's key-management policy.

## Live receiver behavior

The receiver authenticates OTLP/HTTP and OTLP/gRPC bearer headers before parsing, never logs the header, parses accepted JSON or protobuf requests in memory, extracts the same allowlisted metadata from every transport, and discards the raw message. It never writes raw telemetry to disk. Evidence graph and CycloneDX outputs are replaced atomically so readers do not observe partially written JSON.

When a child span arrives before its parent in another export batch, only a bounded allowlist of identity and relationship metadata is queued. Prompt presence is represented as a boolean and, only when configured, a keyed HMAC; prompt/input/output bodies and tool arguments/results are removed before queuing. The current pending count is exposed as `pendingSpans` in `/v1/stats`.

The live endpoints expose metadata that may still be operationally sensitive. Both listeners bind to loopback by default. A non-loopback address requires a bearer-token file, and remote deployments should terminate HTTP and gRPC TLS in a trusted proxy because the built-in servers do not provide TLS.

## Residual risks

- Component names, provider names, trace identifiers, host/service names, and network destinations may still be sensitive metadata.
- Input files remain the operator's responsibility. This tool protects its output; it does not erase or secure the original telemetry.
- A trace ID can become identifying when correlated with another telemetry backend.
- MCP server and tool names, capability sets, and schema digests can expose operational or security posture.
- Conflicting historical versions, digests, endpoints, and other allowlisted properties can reveal deployment changes even when they are not selected.
- A source-authentication digest does not reveal a properly random 32-byte token in practical terms, but theft of the live bearer token permits impersonation until the binding is removed or rotated.
- Ed25519 signatures provide integrity and origin authentication, not confidentiality.
- Canonical signing does not redact additional data. It signs every field already retained in the evidence graph, so access controls and retention rules still apply to the graph and its metadata.

For production use, redact at the OpenTelemetry Collector before long-term storage and apply normal access-control and retention policies to both inputs and outputs.
