# OpenTelemetry MCP 2026 lifecycle patch evidence

Status: local upstream-contribution candidate, validated on 2026-08-13. This
record does not claim that an upstream branch, pull request, merge, project
release, or standard acceptance exists.

## Question

Can the MCP `2026-07-28` lifecycle gap be handled as a focused OpenTelemetry
change, independently of peer server identity, and demonstrated with native
telemetry from an official current SDK without retaining tool content?

## Reviewed boundary

- OpenTelemetry GenAI base commit: `8d3e4a0`
- Local patch branch: `mcp-2026-lifecycle`
- Local patch commit: `49fa922ea0f9ac8a73014cd48cd3026d4abf4713`
- MCP specification: `2026-07-28`
- Official MCP Python SDK: `mcp==2.0.0`
- Related upstream discussion:
  [`open-telemetry/semantic-conventions-genai#437`](https://github.com/open-telemetry/semantic-conventions-genai/issues/437)

The patch commit exists only in the ignored local checkout at
`work/research/otel-genai`. It is recorded here for auditability but is not yet
available from a remote repository.

## Deterministic scenario

The reference scenario uses the official SDK's public `Client` and `Server`
APIs with an in-process transport. The client uses automatic protocol handling,
lists one deterministic `get_weather` tool, and calls it once.

The SDK negotiated `2026-07-28` and native server instrumentation produced
exactly these spans:

| Span | Kind | Required lifecycle evidence |
|---|---|---|
| `server/discover` | `SERVER` | `mcp.method.name=server/discover`, `mcp.protocol.version=2026-07-28` |
| `tools/list` | `SERVER` | `mcp.method.name=tools/list`, `mcp.protocol.version=2026-07-28` |
| `tools/call get_weather` | `SERVER` | `mcp.method.name=tools/call`, `mcp.protocol.version=2026-07-28` |

No span contained `mcp.session.id`. The patch therefore:

- adds the current `server/discover` method value;
- documents `initialize`/`notifications/initialized` as legacy lifecycle;
- forbids the session attribute and session-duration metrics when the protocol
  version does not define sessions;
- retains valid legacy session semantics;
- adds the official SDK reference scenario and generated coverage reports.

Peer server identity and `subscriptions/listen` are intentionally excluded so
that every added lifecycle method has direct scenario evidence.

## Privacy and negative evidence

The raw ignored conformance report was checked for the test location, tool
result, tool description, and session identifier. It retained none of them.
Coverage also reported zero occurrences for tool arguments, tool results, tool
definitions, tool descriptions, and `mcp.session.id`.

The conformance runner completed in report-only mode with four warnings for the
`tools/call` span:

- MCP names the span `tools/call get_weather`, while generic GenAI guidance
  expects an `execute_tool` name;
- MCP uses `SERVER` kind, while generic GenAI guidance expects `INTERNAL`;
- `gen_ai.tool.call.id` was absent;
- `gen_ai.tool.type` was absent.

These are recorded as an existing MCP/GenAI composition gap. The scenario and
convention were not weakened and unsupported values were not fabricated to
silence the warnings.

## Validation

The following gates passed in the upstream checkout:

- full `make generate-all` regeneration;
- Weaver registry policy checking with no `after_resolution` violation;
- `uv lock --check` for the new scenario's 41-package lock;
- Python bytecode compilation;
- Ruff lint and formatting checks;
- generated report assertions for the exact three span names, three current
  protocol-version attributes, absence of `mcp.session.id`, and absence of
  content markers;
- `git diff --check` before the local commit.

The generator emitted repository-wide warnings about unstable `definition/2`
files and future requirement-level fields. They were already present outside
this lifecycle change and did not produce policy violations.

## Remaining boundary

This is server-side in-process SDK evidence, not a client-span or stdio/HTTP
transport matrix. Upstream reviewers may require a narrow transport scenario.
The result also does not prove peer server identity semantics or maintainer
acceptance. Publishing the focused pull request requires separate explicit
authorization for the external write.
