# Threat model

## Protected assets

- integrity of the evidence graph and exported BOM;
- confidentiality of prompt, response, tool, and credential content;
- correctness of component identities and relationships;
- auditability of changes between collection windows.

## Trust boundaries

1. Telemetry producers may be buggy, compromised, or dishonest.
2. The normalizer processes untrusted JSON.
3. Evidence files may be modified after generation.
4. Policy authors are trusted to define organizational intent.
5. Hosted model providers remain outside the verifier's control.

## Addressed in v0.1

- Evidence provenance is explicit rather than collapsing declarations and observations.
- Prompt and tool content is not retained.
- Optional prompt fingerprints use a secret HMAC key rather than an unsalted digest.
- Stable identities make unexpected capability and version drift visible.
- Exact evidence files can be signed with Ed25519 and verified later.
- Policy failures use a distinct exit code for CI.
- Key generation refuses to overwrite existing key files.

## Known limitations

- An observed trace proves only that an instrumented component reported an event; it does not prove the event is truthful.
- `verified` currently trusts an upstream `*.signature.verified=true` assertion. Independent OpenSSF Model Signing verification is planned.
- Inputs are loaded into memory and do not yet have a configurable size limit.
- There is no sandbox around input parsing.
- Hosted model aliases and weights cannot be independently verified.
- Signatures cover raw bytes, not canonical JSON.
- The policy language is intentionally small and does not yet reason over paths or aggregate behavior.

Security issues should follow [SECURITY.md](../SECURITY.md), not a public issue containing sensitive reproduction material.

