# Privacy model

AI telemetry may contain source code, credentials, personal data, system prompts, retrieved documents, tool parameters, and model responses. The project therefore uses a metadata-only default.

## Never retained by the v0.7 normalizer

- prompt and completion bodies;
- tool call arguments and results;
- retrieved document content;
- API keys, tokens, cookies, and environment variable values;
- model input and output attachments.
- MCP tool descriptions, raw input/output schemas, arguments, and results.

These attributes may be present in an input OTLP document, but the normalizer does not copy them into the graph.

The Dify, Microsoft Agent Framework, and MCP executable checks intentionally contain marker values in prompt, input, output, tool-description/schema, tool-argument, and tool-result fields. Automated checks fail if any marker reaches the normalized graph or CycloneDX output.

MCP `tools/list` declarations retain only tool names, server identity/version, selected untrusted boolean annotation hints, and a SHA-256 of the adapter's JSON encoding of the input schema. The digest detects schema drift but does not make the schema safe or trustworthy. Tool names and schema digests can still be sensitive metadata.

## Prompt change detection

Without an explicit `--sensitive-hmac-key-file`, prompt content is neither stored nor fingerprinted. When a key is supplied, the normalizer writes only an HMAC-SHA-256 fingerprint and discards the content. The same protected key must be reused to compare future scans.

The key should be random, at least 32 bytes, stored separately from evidence files, and rotated according to the operator's key-management policy.

## Live receiver behavior

The receiver parses OTLP/HTTP JSON, OTLP/HTTP protobuf, and OTLP/gRPC requests in memory, extracts the same allowlisted metadata from every transport, and discards the raw message. It never writes raw telemetry to disk. Evidence graph and CycloneDX outputs are replaced atomically so readers do not observe partially written JSON.

When a child span arrives before its parent in another export batch, only a bounded allowlist of identity and relationship metadata is queued. Prompt presence is represented as a boolean and, only when configured, a keyed HMAC; prompt/input/output bodies and tool arguments/results are removed before queuing. The current pending count is exposed as `pendingSpans` in `/v1/stats`.

The live endpoints expose metadata that may still be operationally sensitive. Both listeners bind to loopback by default. A non-loopback address requires a bearer-token file, and remote deployments should terminate HTTP and gRPC TLS in a trusted proxy because the built-in servers do not provide TLS.

## Residual risks

- Component names, provider names, trace identifiers, host/service names, and network destinations may still be sensitive metadata.
- Input files remain the operator's responsibility. This tool protects its output; it does not erase or secure the original telemetry.
- A trace ID can become identifying when correlated with another telemetry backend.
- MCP server and tool names, capability sets, and schema digests can expose operational or security posture.
- Ed25519 signatures provide integrity and origin authentication, not confidentiality.

For production use, redact at the OpenTelemetry Collector before long-term storage and apply normal access-control and retention policies to both inputs and outputs.
