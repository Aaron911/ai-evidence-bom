# Privacy model

AI telemetry may contain source code, credentials, personal data, system prompts, retrieved documents, tool parameters, and model responses. The project therefore uses a metadata-only default.

## Never retained by the v0.4 normalizer

- prompt and completion bodies;
- tool call arguments and results;
- retrieved document content;
- API keys, tokens, cookies, and environment variable values;
- model input and output attachments.

These attributes may be present in an input OTLP document, but the normalizer does not copy them into the graph.

The Dify and Microsoft Agent Framework contract fixtures intentionally contain marker values in prompt, input, output, tool-argument, and tool-result fields. Automated compatibility tests fail if any marker reaches the normalized graph.

## Prompt change detection

Without an explicit `--sensitive-hmac-key-file`, prompt content is neither stored nor fingerprinted. When a key is supplied, the normalizer writes only an HMAC-SHA-256 fingerprint and discards the content. The same protected key must be reused to compare future scans.

The key should be random, at least 32 bytes, stored separately from evidence files, and rotated according to the operator's key-management policy.

## Live receiver behavior

The receiver parses OTLP/HTTP JSON, OTLP/HTTP protobuf, and OTLP/gRPC requests in memory, extracts the same allowlisted metadata from every transport, and discards the raw message. It never writes raw telemetry to disk. Evidence graph and CycloneDX outputs are replaced atomically so readers do not observe partially written JSON.

The live endpoints expose metadata that may still be operationally sensitive. Both listeners bind to loopback by default. A non-loopback address requires a bearer-token file, and remote deployments should terminate HTTP and gRPC TLS in a trusted proxy because the built-in servers do not provide TLS.

## Residual risks

- Component names, provider names, trace identifiers, host/service names, and network destinations may still be sensitive metadata.
- Input files remain the operator's responsibility. This tool protects its output; it does not erase or secure the original telemetry.
- A trace ID can become identifying when correlated with another telemetry backend.
- Ed25519 signatures provide integrity and origin authentication, not confidentiality.

For production use, redact at the OpenTelemetry Collector before long-term storage and apply normal access-control and retention policies to both inputs and outputs.
