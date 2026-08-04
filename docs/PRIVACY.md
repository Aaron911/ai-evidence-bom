# Privacy model

AI telemetry may contain source code, credentials, personal data, system prompts, retrieved documents, tool parameters, and model responses. The project therefore uses a metadata-only default.

## Never retained by the v0.1 normalizer

- prompt and completion bodies;
- tool call arguments and results;
- retrieved document content;
- API keys, tokens, cookies, and environment variable values;
- model input and output attachments.

These attributes may be present in an input OTLP document, but the normalizer does not copy them into the graph.

## Prompt change detection

Without an explicit `--sensitive-hmac-key-file`, prompt content is neither stored nor fingerprinted. When a key is supplied, the normalizer writes only an HMAC-SHA-256 fingerprint and discards the content. The same protected key must be reused to compare future scans.

The key should be random, at least 32 bytes, stored separately from evidence files, and rotated according to the operator's key-management policy.

## Residual risks

- Component names, provider names, trace identifiers, host/service names, and network destinations may still be sensitive metadata.
- Input files remain the operator's responsibility. This tool protects its output; it does not erase or secure the original telemetry.
- A trace ID can become identifying when correlated with another telemetry backend.
- Ed25519 signatures provide integrity and origin authentication, not confidentiality.

For production use, redact at the OpenTelemetry Collector before long-term storage and apply normal access-control and retention policies to both inputs and outputs.

