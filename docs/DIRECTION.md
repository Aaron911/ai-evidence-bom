# Direction calibration

AI Evidence BOM is trying to establish a vendor-neutral runtime evidence layer for AI agents. The durable product thesis is not “scan another manifest”; it is:

> Use standard telemetry to show which agents, models, tools, MCP servers, prompts, and data sources were actually involved, with explicit evidence strength and without retaining sensitive content.

Every release must answer four questions: what uncertainty did it test, what evidence changed, what would make the project stop or pivot, and what is the smallest next test?

## v0.4 calibration — 2026-08-04

### Uncertainty tested

Can two unrelated AI agent frameworks be represented by one evidence contract through standard OpenTelemetry data, without maintaining a private adapter for each framework?

### Evidence gained

- Dify exposes standard OTLP HTTP/gRPC exporter paths and emits workflow, model, and tool information across related spans.
- Microsoft Agent Framework emits `invoke_agent`, concrete model, and tool spans using OpenTelemetry GenAI attributes.
- The important compatibility problem is trace structure, not another vendor field alias: agent identity is often on a parent span while model/tool facts are on descendants.
- Source-derived fixtures now produce the same stable core agent/model/tool graph. Sensitive values are excluded by test.

### Decision

**Continue, with a narrower claim.** The standards-first direction remains viable. v0.4 establishes pinned source-contract compatibility, not live framework compatibility.

The implementation should continue to invest in trace semantics and evidence quality. It should not branch into dashboards, generic vulnerability scanning, or a large collection of private framework adapters before the live-export gate is passed.

### Risks and pivot signals

- Pivot toward a small set of maintained adapters if real exporters consistently omit stable identity that cannot be restored through OpenTelemetry configuration.
- Reconsider runtime-first positioning if operators will not enable OTLP or share sanitized captures.
- Do not identify a data source, MCP server, or model from prompt/document content; missing stable metadata must remain an explicit gap.
- Do not call source-derived fixtures “live support.” Overclaiming would damage the evidence product's credibility.

### Next smallest falsifiable test

Run minimal Dify and Microsoft Agent Framework applications, export their traces through the unmodified standard OTLP path into `aiebom collect`, and verify:

1. both reach the receiver without a framework-specific transport;
2. equivalent agent/model/tool behavior yields the same core graph semantics;
3. framework-specific extra evidence remains additive;
4. no sensitive content is written to graph or BOM output.

If this test passes, Phase 1 can claim live compatibility for the core path. If it fails, record whether the cause is exporter setup, missing stable identity, semantic drift, or a normalizer defect before adding code.

## v0.5 calibration — 2026-08-04

### Uncertainty tested

Do the two source-derived contracts survive executable framework code and real OTLP export, including the case where related spans are split across export requests?

### Evidence gained

- Released Microsoft Agent Framework 1.13.0 ran its real Agent, chat telemetry, function-invocation, tool-execution, and OTLP exporter paths. The expected agent/model/tool graph arrived without sensitive content.
- Dify 1.16.1's real workflow handler, LLM parser, and tool parser exported the same core semantics through OTLP. This was isolated instrumentation execution with deterministic host stubs, not a complete Dify application runtime.
- Executable validation found a normalizer defect that source fixtures did not expose: child spans exported before their parent were temporarily attributed to `service.name`.
- Bounded, metadata-only cross-batch context now delays unresolved children until parent identity arrives. Regression tests prove that content is removed before queuing and that the false service agent is not created.

### Decision

**Continue, but keep the Phase 1 exit gate open.** The standards-first thesis became stronger: one released framework core runtime and one independent framework's actual instrumentation reached the same graph without a private transport adapter. However, Dify has not yet passed a complete application-runtime capture, so v0.5 must not claim live compatibility for two full frameworks.

The next iteration should finish that missing runtime proof before expanding into generic zero-code auto-instrumentation, dashboards, vulnerability feeds, or additional frameworks. The new cross-batch behavior is core infrastructure, not framework-specific debt.

### Risks and pivot signals

- If a complete Dify run cannot expose stable app identity, model, and tool metadata through its supported OTLP configuration, downgrade the Dify path and consider a small maintained adapter.
- If real deployments routinely exceed the bounded correlation window, add an explicit trace-completion/TTL design rather than silently increasing memory.
- If users cannot enable framework telemetry, test a narrowly scoped zero-code Python launcher; do not promise universal no-code coverage across runtimes.
- Continue to reject content-derived component identity and unverified claims about hosted model weights.

### Next smallest falsifiable test

Start an official minimal Dify application stack in a disposable Docker/CI environment, enable its supported OTLP HTTP exporter, execute one workflow containing an LLM node and a tool node against deterministic local substitutes, and send the unmodified trace to `aiebom collect`.

The test passes only if:

1. no Dify source or runtime patch is required;
2. `dify.app_id`, model provider/name, and tool name form the expected graph;
3. child-first or split-batch export does not create a service fallback agent;
4. graph and BOM contain no prompt, model-response, or tool-argument marker;
5. the exact Dify image/source version and configuration are recorded.

If that cannot be made deterministic at reasonable CI cost, record the blocker and move the check to a documented periodic manual environment rather than weakening the evidence grade.

## v0.6 calibration — 2026-08-04

### Uncertainty tested

Can a complete official Dify application runtime emit stable agent, model, and tool evidence through its supported OTLP configuration, without patching Dify or introducing a private telemetry adapter?

### Evidence gained

- A pinned Dify 1.16.1 API, plugin daemon, PostgreSQL, and Redis stack imported, published, and executed a workflow containing an LLM node and a built-in tool node.
- With Dify's supported OTel settings enabled, unmodified OTLP/HTTP protobuf reached `aiebom collect`. The graph contained exactly one agent identified by `dify.app_id`, OpenAI `gpt-4o`, `time.current_time`, a content-free prompt node, and `uses`, `invokes`, and `uses_prompt` relationships.
- Prompt, input, model output, tool argument, and tool output markers did not reach the graph or CycloneDX BOM.
- The first CI attempt failed usefully: the Dify container could not resolve the Marketplace host while installing the plugin, and failure cleanup did not terminate promptly. The final check downloads a fixed official plugin package on the host, verifies its SHA-256, uploads it through Dify's supported API, and bounds cleanup time. This makes setup deterministic without changing Dify telemetry behavior.
- Together with the released Microsoft Agent Framework core-runtime check, two unrelated runtimes now reach the same vendor-neutral core graph semantics through standard OTLP. The Phase 1 core exit gate is therefore passed.

### Decision

**Continue and move the primary focus to evidence quality.** The standards-first thesis passed its core adoption test. The project should not respond by collecting framework logos, building a dashboard, or adding generic vulnerability feeds. Optional retrieval, MCP, and multi-agent coverage remains open, but new work should be selected for security value and its ability to falsify graph or policy claims.

### Risks and pivot signals

- Full Dify validation is heavier than the isolated check and depends on Docker plus availability of pinned upstream source and plugin artifacts. Keep the fast check in ordinary CI and the full stack in a separate scheduled and path-triggered workflow.
- Dify can emit content-bearing telemetry when configured to do so. Metadata-only filtering at the receiver remains a security boundary; this result does not make upstream traces safe to store elsewhere.
- One deterministic workflow is live capture, not production validation. Do not claim coverage of every Dify node, provider, deployment mode, or operational limit.
- If upstream semantic conventions repeatedly break stable identity or require framework-specific transport code, measure that maintenance cost and reconsider the adapter boundary.
- If real operators will not enable OTel or cannot route OTLP safely, validate a narrow auto-instrumentation launcher before making any zero-code claim.

### Next smallest falsifiable test

Execute one real MCP client/server tool call with standard telemetry and a stable, non-content-derived server identity. Capture a baseline and a changed server capability set, then verify that graph diff and a graph-path policy detect the new capability without retaining arguments or results.

The test fails if server identity can only be guessed from tool text or network content, if the capability change cannot be tied to the invoking agent, or if policy evaluation requires a framework-specific private transport. A failure should be recorded as an MCP telemetry-standard gap before adding an adapter.

## v0.7 calibration — 2026-08-05

### Uncertainty tested

Can one real MCP stdio client/server interaction establish stable, non-content-derived `Agent → MCP Server → Tool` evidence and detect an added server capability without falsely claiming that the Agent invoked it?

### Evidence gained

- Official MCP Go SDK v1.7.0 completed `server/discover`, `tools/list`, and `tools/call` in separate client/server processes and negotiated protocol `2026-07-28`.
- `serverInfo.name` and version provide a stable logical identity independent of tool descriptions, arguments, results, process path, and network content.
- A baseline with `weather.lookup` and a changed server that also declares `shell.execute` normalize to the same Agent and server identities. The called weather tool becomes observed; the uncalled shell tool remains declared.
- Generic graph diff detects the added capability node and `provides` edge. A bounded directed path policy passes the baseline and rejects `Agent -[connects_to]-> Server -[provides]-> shell.execute` in the changed graph.
- Description, schema, argument, and result markers do not reach graph, diff, policy, or CycloneDX output.
- Negative evidence matters: the official SDK does not auto-emit OTLP, and the current Development-status OTel MCP convention has no logical server-name field. Its documented lifecycle still centers legacy methods while MCP `2026-07-28` moved to `server/discover` and per-request metadata.

### Decision

**Continue, while moving the next effort upstream rather than multiplying private adapters.** MCP runtime evidence is feasible when protocol discovery supplies identity and OTel supplies observed operations. The explicit `aiebom.mcp.*` bridge is acceptable as a tested compatibility boundary, but it should not quietly become a permanent competing convention.

v0.7 also completes the first Phase 2 drift scenario and the minimum graph-path policy primitive. The project should not expand this into active tool execution, trust tool descriptions/annotations, add a dashboard, or claim zero-code MCP coverage.

### Risks and pivot signals

- If OTel maintainers choose a different stable server-identity model, migrate the bridge and preserve legacy input compatibility rather than defending the current attribute name.
- If a second official SDK cannot expose `serverInfo` and `tools/list` without a private transport hook, narrow support to SDKs with protocol-level discovery access.
- If operators consider tool names and schema digests too sensitive, add configurable capability redaction before collecting broader real-world samples.
- If protocol and telemetry lifecycle drift repeatedly requires substantial compatibility code, move that mapping into a small versioned adapter package and measure maintenance cost.
- Declared capability reachability is not actual execution. Any policy or report that collapses `declared` and `observed` would invalidate the evidence thesis.

### Next smallest falsifiable test

Prepare one evidence-backed OpenTelemetry GenAI proposal covering MCP `2026-07-28` lifecycle names and stable logical server identity. The test passes if maintainers confirm the gap and accept a concrete direction (even with different attribute names); it fails if the use case is rejected or already represented by a standard field we missed.

Until that feedback exists, do not add another project-specific server-identity alias. If the proposal is rejected, retain the explicit extension and validate the same identity/capability semantics with one second official MCP SDK before expanding product scope.

## v0.8 proposal-preparation calibration — 2026-08-05

This is an unreleased documentation and external-validation iteration, not a v0.8 product release.

### Uncertainty tested

Does the current OpenTelemetry GenAI MCP convention still have a concrete, reproducible gap for MCP `2026-07-28` lifecycle and logical peer-server metadata, or is AI Evidence BOM's project bridge duplicating a standard field that was added after v0.7?

### Evidence gained

- OpenTelemetry GenAI `main` still resolves to reviewed commit `7e6e188`. Its MCP convention remains Development, contains no `server/discover` method or logical MCP server name/version attribute, recommends `mcp.session.id`, defines session-duration metrics, and demonstrates the `2025-06-18` `initialize` lifecycle.
- The current MCP `2026-07-28` specification removes the protocol-level handshake and session, adds `server/discover` and `subscriptions/listen`, and carries self-reported server information in response metadata.
- Existing `server.address` is an endpoint rather than logical implementation metadata; server-side Resource `service.name` is not generally visible to a client; `gen_ai.tool.name` is only unique within a server. The v0.7 stdio reproduction therefore exercises a real interoperability gap.
- Searches of the upstream issue and pull-request lists found related MCP work but no issue specifically covering `2026-07-28`, `server/discover`, session removal, and protocol-reported peer server identity together.
- A focused English proposal records the pinned boundary, live reproduction, privacy constraints, candidate semantics, reference scenario, and open maintainer questions. It was submitted with explicit authorization as [`open-telemetry/semantic-conventions-genai#437`](https://github.com/open-telemetry/semantic-conventions-genai/issues/437). No maintainer agreement is claimed yet.

### Decision

**Continue upstream-first, without a product version bump.** The standards gap remains real and evidence-backed, and the proposal is now awaiting external review. The correct next step is to evaluate maintainer feedback, not add another private alias, framework adapter, dashboard, or vulnerability feed.

The draft deliberately treats MCP server information as untrusted and non-unique. It proposes lifecycle alignment and semantic requirements while leaving final attribute naming to the GenAI SIG.

### Risks and pivot signals

- Maintainers may prefer an existing peer-service/entity model instead of `mcp.server.*`; the project should migrate the bridge once an accepted direction exists.
- Maintainers may split lifecycle modernization and server metadata into separate changes. That is compatible with the goal and should not be treated as rejection.
- If maintainers determine that an existing standard field already carries the same client-visible protocol semantics, stop proposing a new attribute and add compatibility for that field.
- If protocol-reported name/version is considered too weak or ambiguous for a semantic convention, retain it only as declared project provenance and validate a second official SDK before broadening support.
- Issue submission is an external contribution attempt, not proof of acceptance. Do not mark the standards direction validated until maintainers respond.

### Next smallest falsifiable test

Monitor issue [`#437`](https://github.com/open-telemetry/semantic-conventions-genai/issues/437) and obtain maintainer feedback on three decisions: the peer server metadata model, version-aware treatment of `mcp.session.id` and session metrics, and a `2026-07-28` reference scenario.

The test passes when maintainers confirm the gap and choose a concrete direction, even if the final names differ. It fails if the use case is rejected or is already represented by an existing convention; in that case, record the decision, update the compatibility bridge, and test the accepted model rather than defending the draft.

## v0.8 CycloneDX schema-conformance calibration — 2026-08-06

This is an unreleased evidence-quality iteration, not a product or evidence-graph schema version bump.

### Uncertainty tested

Does the current CycloneDX 1.7 exporter actually conform to the official JSON Schema, and can a deterministic CI gate reject a structurally invalid BOM rather than merely checking project-owned Go structs?

### Evidence gained

- CycloneDX documents its JSON Schema as the reference implementation of the standard. The test pins the official 1.7 schema at Git blob `08d6a3c884614630075dbb841c74397fbd5fc5d2` and SHA-256 `df472ef4aaf593904c479293723a1a5c191d6672715c93b3c0b5c318f3914221`.
- A real five-component, five-dependency `aiebom scan` export validated successfully with JSON Schema Draft 7 format checks enabled.
- The official schema exposed that the exporter still used the valid but deprecated legacy `metadata.tools` array. The exporter now emits the current `metadata.tools.components` form and retains no empty `bom-ref` for the generator component.
- The reusable gate validates a real current-source export and then changes `bomFormat` to `NotCycloneDX`. It passes only when the real export is accepted and the negative control is rejected specifically at `/bomFormat`.
- Race-enabled unit tests and `go vet` passed in a disposable copy using the locally available Go 1.26.2 toolchain. The repository requirement remains Go 1.26.5; downloading that toolchain locally failed with a TLS handshake timeout, so the pushed GitHub gate remains the authoritative required-version run.
- GitHub Actions [`ci #12`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/31079178515) passed on the required toolchain, including the test job, all three framework-compatibility jobs, and the new positive/negative CycloneDX gate. The path-triggered full Dify workflow [`#5`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/31079178480) also passed.
- OpenTelemetry issue [`#437`](https://github.com/open-telemetry/semantic-conventions-genai/issues/437) remains open with no maintainer comments as of this calibration. No standards acceptance is inferred from the absence of feedback.

### Decision

**Continue the standards-first evidence-quality direction.** The CycloneDX claim is now backed by an independently maintained schema and a negative control, not only project tests. This strengthens output interoperability without expanding the collected evidence, adding content retention, or claiming CycloneDX certification.

The deprecated tools shape was small, directly evidenced compatibility debt, so correcting it was justified in the same iteration. No unrelated framework, MCP identity alias, dashboard, vulnerability feed, or broad instrumentation work is introduced.

### Risks and pivot signals

- Schema validity proves structural conformance, not that every evidence mapping is semantically complete, trustworthy, or certified by CycloneDX.
- The CI check downloads an external official artifact. A pinned SHA-256 prevents silent substitution, but upstream or network unavailability can still fail the gate; a local checksum-verified file override exists for controlled/offline reproduction.
- The validation helper pins Python `jsonschema==4.26.0` in CI. If this test-only dependency becomes unstable or materially increases maintenance, replace it with an equivalently strict pinned validator rather than weakening the negative control.
- `metadata.tools.components` identifies the generator, not an observed runtime component. It must not be merged into the evidence graph or interpreted as runtime evidence.

### Next smallest falsifiable test

Continue monitoring OpenTelemetry issue `#437`; actionable maintainer feedback preempts speculative MCP implementation. If no feedback exists at the start of the next iteration, test whether one fixed evidence graph can produce byte-identical canonical snapshot bytes and a stable signature across repeated exports, while a one-field evidence change necessarily changes the canonical digest.

The test fails if reproducibility requires removing meaningful evidence timestamps, if map ordering or JSON formatting changes the signed identity, or if verification cannot clearly distinguish canonical content from transport serialization. A failure should narrow the signing claim before adding retention or attestation features.

## v0.8 canonical evidence-signing calibration — 2026-08-07

This is an unreleased evidence-quality iteration, not a product or evidence-graph schema version bump.

### Uncertainty tested

Can one fixed evidence graph produce byte-identical canonical identity bytes and a stable Ed25519 signature across harmless JSON and graph-order changes, while any retained evidence-field change necessarily changes the canonical digest?

### Evidence gained

- The new canonical profile strictly decodes the project evidence graph, rejects duplicate or unknown JSON members, applies the graph's set/order semantics, normalizes timestamps to UTC, and then uses RFC 8785 JCS for the final cryptographic representation.
- Two equivalent fixtures with different whitespace, object-member order, node order, repeated/set-like values, and UTC-offset spellings produce byte-identical canonical data, the same SHA-256 digest, and the same Ed25519 signature under one key.
- Verification succeeds against an equivalently reformatted transport document but fails with `payload digest mismatch` after a one-field `observationCount` change.
- Signature envelope v0.2 declares both the evidence payload type and canonicalization profile. Its signature input is domain-separated from legacy v0.1 raw-byte signing; legacy envelopes remain valid and formatting-sensitive.
- Meaningful graph timestamps are retained and signed. No prompt, response, tool argument, result, or other new content is collected by this feature.
- The RFC 8785 reference Go implementation is pinned to upstream commit `19d51d7fe467d4706a3ff08adf8a748f29fc21e0` and attributed under Apache-2.0.
- Race-enabled tests and `go vet` passed in a disposable copy on the locally available Go 1.26.2 toolchain. A real CLI scan/keygen/sign/reformat/verify run produced the same canonical digest and signature and rejected a changed `observationCount`; the CycloneDX positive/negative schema gate also remained green.
- Local `govulncheck` correctly rejected Go 1.26.2 for seven reachable standard-library findings fixed by Go 1.26.5. The repository minimum was not lowered; the required-version GitHub run remains the authoritative vulnerability gate.
- GitHub Actions [`ci #14`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/31145359216) passed on Go 1.26.5: race tests, `go vet`, `govulncheck`, build, the CycloneDX positive/negative gate, and all three lightweight framework/MCP compatibility jobs succeeded. The path-triggered full Dify workflow [`#6`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/31145359217) also passed against the complete application stack.

### Decision

**Continue the evidence-quality direction.** The Phase 2 reproducible-snapshot item is complete for AI Evidence BOM graphs. The result is deliberately narrower than arbitrary JSON or CycloneDX signing and does not claim a trusted signing time, transparency log, confidentiality, or independent model-artifact verification.

The canonical profile is opt-in so existing exact-byte signatures and BOM workflows do not change semantics. Verification follows explicit envelope metadata rather than attempting to infer whether a file should be canonicalized.

### Risks and pivot signals

- Canonical identity is versioned against the current evidence-graph contract. A future graph field or changed set/order semantic requires a reviewed profile version rather than silently changing `aiebom-evidence-v1+jcs-rfc8785`.
- The stable signature claim applies to one fixed graph and key. A new collection timestamp or any retained evidence change is intentionally a new identity.
- Envelope `createdAt` remains informational and unsigned. If operators need proof of signing time, validate a trusted timestamp or transparency-log integration instead of overstating this envelope.
- CycloneDX provides its own JSF/JWS signature mechanisms. If BOM-native interoperability becomes a validated user need, implement and test that standard separately rather than relabeling the graph profile.
- The canonicalizer is a pinned pseudoversion of the RFC-listed Go implementation. Dependency staleness, an upstream security issue, or cross-language fixture disagreement should fail the gate and trigger replacement or vendored review, not a fallback to ad hoc serialization.

### Next smallest falsifiable test

Continue monitoring OpenTelemetry issue `#437`; actionable maintainer feedback still preempts speculative MCP aliases. If no feedback exists, construct a conflicting-evidence fixture for one stable model identity and test explicit source precedence: a weaker or merely newer declaration must not overwrite independently verified identity data, and the conflict must remain visible rather than being silently resolved.

The test fails if precedence is determined only by arrival time, if telemetry can self-promote to verified, or if exposing the conflict requires retaining model inputs, outputs, or credentials.

## v0.8 field-evidence precedence calibration — 2026-08-07

This iteration changes the experimental evidence-graph schema to `0.8.0`; it does not create a tag or release.

### Uncertainty tested

Can conflicting claims about one stable model identity be merged without arrival order deciding the result, without a weaker or newer declaration overwriting independently verified field evidence, and without hiding the disagreement?

### Evidence gained

- Versions, digest algorithms, and retained properties now carry value-specific evidence summaries instead of borrowing only the node's strongest aggregate level.
- Selection is deterministic: evidence strength wins first, then latest observation time, then lexical order. Forward and reverse permutations of the same verified and declared model claims produce identical nodes.
- A newer declared version, digest, and endpoint cannot replace older verified candidates. Both values remain in `fieldEvidence` with `conflict=true` and their original source, strength, count, and time window.
- Graph diff reports changed field evidence, `forbidFieldConflicts` can reject the ambiguity, and CycloneDX preserves selected values, candidates, strengths, and conflict markers under the `aibom:` namespace.
- Canonical signing moves new graphs to envelope v0.3/profile v2 so field candidate ordering and timestamps are part of an explicit domain-separated contract; existing v0.2/profile-v1 signatures remain verifiable and reject v0.8-only field evidence.
- OTLP's downgrade-only evidence boundary now also ignores `gen_ai.model.signature.verified=true`; an untrusted span remains observed. No prompt, response, credential, tool argument, or tool result is needed to expose a conflict.
- Legacy v0.7 graphs cannot prove which observation supplied a mutable field. Their existing values therefore migrate as inferred candidates instead of inheriting a potentially misleading verified node summary.
- Race-enabled Go tests, `go vet`, build, and the CycloneDX 1.7 positive/negative schema gate passed in a disposable copy on the locally available Go 1.26.2 toolchain. A real CLI run selected the verified fixture values, produced three policy conflicts, and generated and verified a v0.3/profile-v2 canonical signature.
- Local `govulncheck` found only the same seven reachable Go 1.26.2 standard-library issues, all fixed by the repository-required Go 1.26.5. The minimum remains unchanged, and the required-version GitHub run remains the authoritative vulnerability gate.
- GitHub Actions [`ci #16`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/31160371946) completed successfully on commit `36d7371`, covering the required Go 1.26.5 race, vet, vulnerability, build, CycloneDX, Agent Framework, Dify instrumentation, and MCP jobs. The path-triggered full Dify workflow [`#7`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/31160371962) also completed successfully.
- OpenTelemetry issue [`#437`](https://github.com/open-telemetry/semantic-conventions-genai/issues/437) remained open with no maintainer comments at the start of this iteration, so no standard acceptance or new MCP alias is inferred.

### Decision

**Continue the evidence-quality direction.** The previous arrival-time merge could combine a weak field value with a strong node-level label; v0.8 removes that misleading composition and turns ambiguity into auditable, enforceable evidence. This remains a BOM evidence problem, not a dashboard, generic vulnerability feed, or active model-safety verdict.

The schema bump is justified because `fieldEvidence` is a new public graph contract. Existing top-level values remain for consumers and CycloneDX export, but their meaning is now explicitly “strongest-supported selected value,” not “last value received.”

### Risks and pivot signals

- A compact adapter can still label an observation verified. Until a source trust cap or built-in verifier exists, operators must restrict which adapter can supply that input.
- The strongest-supported candidate is not necessarily the currently deployed candidate. Security gates that require certainty should enable `forbidFieldConflicts` rather than looking only at the top-level value.
- Legacy migration deliberately loses unsupported field-level confidence. If operators require historical confidence, they must retain original signed evidence or re-run a trusted verifier; the project must not reconstruct proof that was never recorded.
- Candidate values increase retained metadata and are not yet cardinality-bounded. If a high-cardinality producer can grow a long-lived graph materially, bound the representation without making merge results depend on arrival order.
- If real operators prefer explicit unresolved fields over a selected strongest value, add a separate resolution status rather than silently changing precedence.

### Next smallest falsifiable test

Continue monitoring OpenTelemetry issue `#437`; actionable maintainer feedback still preempts speculative MCP work. If no feedback exists, add operator-defined source trust caps and test a malicious compact adapter that labels deployment metadata `verified`: the claim must be downgraded or rejected unless that exact source is authorized for verified evidence, while a trusted verifier source remains accepted.

The test fails if trust depends only on a self-reported evidence level, if source rules change results by arrival order, or if configuration requires framework-specific code. A failure should narrow the meaning of `verified` before integrating more attestation formats.

## OpenTelemetry MCP 2026 lifecycle patch calibration — 2026-08-13

This is an unreleased upstream-contribution iteration. It does not change the
AI Evidence BOM product, evidence graph, or schema.

### Uncertainty tested

Can the MCP `2026-07-28` lifecycle gap be separated from the disputed peer
server-identity proposal and supported by native telemetry from a current
official SDK, without inventing fields or retaining tool content?

### Evidence gained

- The OpenTelemetry GenAI repository at reviewed commit `8d3e4a0` still
  documents the session-oriented lifecycle and does not include
  `server/discover` in the well-known MCP methods.
- Official MCP Python SDK `2.0.0` negotiated protocol `2026-07-28` through its
  public client/server APIs. Its native server telemetry emitted
  `server/discover`, `tools/list`, and `tools/call get_weather` spans, with
  `mcp.protocol.version=2026-07-28` on all three and no `mcp.session.id`.
- The captured telemetry did not retain the test location, tool result, tool
  description, tool arguments, tool result attribute, or schema content.
- A lifecycle-only OpenTelemetry patch now adds `server/discover`, makes the
  session attribute and session-duration metrics explicitly version-aware,
  documents current and legacy lifecycles, and adds a deterministic official
  Python SDK reference scenario. It is committed locally as `49fa922` on
  branch `mcp-2026-lifecycle`; it has not been pushed or submitted upstream.
  Exact boundaries and negative evidence are recorded in
  [`docs/evidence/otel-mcp-2026-lifecycle.md`](evidence/otel-mcp-2026-lifecycle.md).
- The upstream repository's full generation target and registry policy check
  passed. The scenario lock resolved, Python syntax and Ruff checks passed,
  generated output was reproducible, and explicit privacy assertions passed.
- Project documentation commit `a24fc0c` was pushed to `main`, and GitHub
  Actions [`ci #18`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/31675945542)
  completed successfully. This validates the project record, not the unpushed
  upstream patch.
- The conformance runner completed in report-only mode but exposed four
  pre-existing overlaps between MCP server spans and generic GenAI
  `execute_tool` rules: span name, `SERVER` versus `INTERNAL` kind, and absent
  `gen_ai.tool.call.id` and `gen_ai.tool.type`. These warnings were retained as
  negative evidence rather than suppressed.

### Decision

**Continue upstream-first with a deliberately split contribution.** The
stateless lifecycle is independently demonstrable and should be reviewed
without peer identity or subscription semantics in the same patch. The local
patch therefore does not add `mcp.server.*`, does not add
`subscriptions/listen`, and does not modify the AI Evidence BOM schema.

Issue discussion and a review-ready local commit are not standards acceptance.
No upstream branch, pull request, tag, or release has been created in this
iteration.

### Risks and pivot signals

- Issue `#437` still has no confirmed maintainer decision. External reviewer
  feedback to split lifecycle and peer metadata is useful scope guidance, not
  maintainer approval.
- The native reference currently demonstrates server-side in-process spans. If
  reviewers require client spans or a concrete stdio/HTTP transport, extend the
  scenario narrowly rather than synthesizing those values.
- The `execute_tool` warnings indicate an MCP/GenAI semantic-composition gap.
  If maintainers consider those warnings blocking, address them as a separate
  convention question instead of weakening the lifecycle assertions.
- Legacy MCP versions still define initialization and can define sessions.
  Compatibility must remain version-aware; current behavior must not erase
  valid legacy telemetry.
- If upstream has already started overlapping lifecycle work before submission,
  rebase and reduce this patch to the missing pieces rather than competing with
  the accepted direction.

### Next smallest falsifiable test

After explicit authorization for the external write, publish the lifecycle-only
commit as an upstream pull request linked to issue `#437`. The test passes when
maintainers confirm the version-aware lifecycle direction or request bounded
changes that preserve the same evidence boundary. It fails if the lifecycle
gap is rejected, already solved by another accepted change, or the reference
cannot credibly demonstrate the convention; record that result before resuming
peer metadata work.

## v0.9 source trust-cap calibration — 2026-08-22

This is an unreleased product/schema candidate. It does not publish the local
OpenTelemetry lifecycle patch or claim any new framework compatibility grade.

### Uncertainty tested

Can a malicious compact adapter obtain `verified` authority solely by
self-reporting its evidence level, or can a vendor-neutral operator policy cap
that claim while preserving a separately authorized verifier and deterministic
field selection?

### Evidence gained

- Every unmatched source now has a fixed maximum of `observed`. Only an exact,
  case-sensitive source rule can preserve `verified`; the same rule model can
  impose stricter `observed`, `declared`, or `inferred` caps.
- A deterministic fixture combines an authorized verified `weights-v1` claim
  with a newer malicious self-reported verified `weights-v2` claim. The latter
  becomes observed, the trusted value remains selected, and both values remain
  visible as a conflict.
- Trusted-first and malicious-first permutations produce deeply equal nodes,
  so source policy does not reintroduce arrival-order precedence.
- `scan` and the live receiver share the same cap. The receiver applies it
  before cross-batch pending correlation and reports unique reductions through
  `evidenceDowngrades`.
- Persisted graphs are recapped on receiver startup across node, edge, and
  field-candidate summaries, then field selections are recomputed. A pre-v0.9
  untrusted verified claim therefore does not silently survive a restart.
- Strict policy parsing rejects unsupported versions, unknown fields, invalid
  levels, empty or duplicate exact-source rules, and trailing JSON.
- Synthetic input-message and tool-argument markers do not reach the graph.
  The policy introduces no new content retention or identity inference.
- The public OpenTelemetry issue `#437` remained open without a visible
  actionable maintainer conclusion at the start of the iteration, so no MCP
  alias or lifecycle product change was mixed into this work.
- The security gate found six reachable standard-library vulnerabilities in
  Go 1.26.5, all fixed in 1.26.6. The minimum toolchain was raised to 1.26.6;
  `govulncheck@v1.6.0` then reported no vulnerabilities.
- Go 1.26.6 race tests, vet, build, CycloneDX schema positive/negative checks,
  Agent Framework live capture, Dify instrumentation execution, MCP runtime,
  and a real v0.9 canonical sign/verify passed locally. Full Dify could not run
  locally because Docker is unavailable.
- Implementation commit `699ec14` passed GitHub Actions
  [`ci #20`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/32557634364)
  on Go 1.26.6 and the complete path-triggered Dify workflow
  [`#10`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/32557634376).
  This validates the candidate; it does not create a tag or release.

### Decision

**Continue with the narrower verified-evidence claim.** This closes the known
path where compact input could label itself verified and silently win field
precedence. It directly strengthens the project's evidence credibility without
adding a framework adapter, dashboard, vulnerability feed, or sensitive
content collection.

`verified` now means both that a verifier adapter claims independent support
and that the operator has granted that exact source the authority to make the
claim. It still does not mean the source identity is cryptographically proven,
the component is safe, or hosted model weights were independently verified.

### Risks and pivot signals

- Source names in compact documents and OTLP resources remain self-reported
  labels. A malicious producer that can use an authorized name can still spoof
  authority unless the surrounding process or transport binds that name to an
  authenticated identity.
- The default observed cap prevents self-promotion to verified but does not
  prove that arbitrary compact metadata came from a runtime event. Operators
  should use stricter source rules for configuration-only adapters.
- Policy files are trusted local configuration. Their access control, review,
  and distribution are outside the evidence graph.
- Candidate values remain cardinality-unbounded. If real long-lived collection
  shows material growth, design an order-independent bound before adding more
  retained field types.
- If operators cannot identify and control verifier adapters, narrow or remove
  externally supplied verified evidence rather than adding more attestation
  formats on an unauthenticated boundary.

### Next smallest falsifiable test

Bind one live-receiver source to a source-specific authenticated credential
outside the OTLP payload. A producer using the trusted source label with the
wrong credential must be rejected or capped, while the correctly authenticated
verifier retains its configured authority; no credential may enter the graph,
logs, pending queue, or BOM.

The test fails if authority still depends only on `service.name` or another
self-reported attribute, if credential rotation changes graph identity, or if
the design requires framework-specific code. Until this boundary is proven,
do not broaden verified attestation formats.

## v0.10 live source-authentication calibration — 2026-08-27

This is an unreleased product/schema candidate. It does not add a vulnerability scanner, publish the local OpenTelemetry lifecycle patch, or claim a new framework compatibility grade.

### Uncertainty tested

Can one protected live-receiver source be bound to a source-specific credential outside OTLP so a forged `service.name` cannot obtain its authority, while correct and rotation credentials preserve a stable source and no credential reaches evidence state?

### Evidence gained

- OpenTelemetry Specification v1.60.0 defines configurable headers for both OTLP HTTP and gRPC exporters. A standard `Authorization` header can therefore carry the credential without changing OTLP or adding framework-specific transport code.
- A strict versioned policy maps SHA-256 digests of random credentials to exact sources. Multiple credentials can map to the same source for rotation, but one digest cannot map to multiple sources.
- A correct credential replaces payload-derived evidence authority with the bound source. A wrong credential is unauthenticated; a global credential attempting to self-report a protected source is rejected before deduplication and pending correlation.
- Live trust rules above `observed` cannot start without an exact source-authentication binding. Global/read and source/ingest credential roles cannot reuse the same token.
- Source credentials cannot read graph, BOM, or stats endpoints protected by the global credential. Unit and live CLI controls found no raw credential in HTTP rejection responses, graphs, BOMs, pending observations, or receiver logs.
- Authentication deliberately does not promote OTLP above `observed`: producer identity is not evidence that an assertion is true or a component is safe.
- Go 1.26.6 race tests, vet, build, pinned vulnerability scanning, CycloneDX schema positive/negative checks, a real source-authenticated CLI receiver, Microsoft Agent Framework live capture, and the official MCP Go SDK runtime check passed locally.
- The isolated Dify check produced no local product failure but could not complete because its pinned GitHub sparse fetch stalled, and Docker is unavailable locally. Implementation commit `305dd7b` subsequently passed GitHub Actions [`ci #22`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/33038176470), including isolated Dify, and the complete Dify workflow [`#12`](https://github.com/Aaron911/ai-evidence-bom/actions/runs/33038176467).

### Decision

**Continue, with the evidence-layer boundary intact.** v0.10 closes the precise spoofing gap left by v0.9: an exact source name can now be protected by a credential outside the telemetry payload. It does not turn static bearer possession into attestation, introduce a private Agent protocol, or broaden content retention.

This strengthens the foundation needed before accepting external verification or vulnerability findings. Building a generic MCP/Skill scanner now would duplicate specialist tools and move away from the project's differentiator: correlating runtime inventory, declarations, verification, drift, and policy with explicit provenance.

### Risks and pivot signals

- Static bearer tokens have no intrinsic expiry, audience, revocation service, or hardware identity. Suspected exposure requires removing or rotating the digest; remote transport still requires external TLS.
- A correctly authenticated producer may still lie. If operators interpret authentication as safety or verification despite documentation and evidence levels, rename or further constrain the feature before adding attestation formats.
- Offline compact input remains an operator-controlled file boundary. Do not describe `scan` source labels as authenticated.
- Source bindings are exact configuration. If deployments need dynamic workload identity at scale, test mTLS or a trusted proxy assertion as a separate bounded mechanism rather than adding framework branches.
- If external scanner findings cannot be joined to graph components using stable identity and artifact digests, do not fall back to display-name matching or build a second scanner inside the core.

### Next smallest falsifiable test

Ingest one external SARIF 2.1.0 result for a deliberately vulnerable MCP server or Skill fixture and attach it to an already discovered component only when stable artifact identity and digest agree. Preserve tool, rule, severity, and evidence provenance; make one policy fail; retain no source code or finding payload beyond an explicit metadata allowlist.

The test fails if mapping requires a framework-specific name heuristic, if a mismatched digest still attaches, if scanner output is relabeled as built-in verification, or if the implementation starts duplicating vulnerability detection. First verify the chosen external tool's actual export contract; keep the core scanner-agnostic.

## v0.11 external SARIF correlation calibration — 2026-09-02

This is an unreleased product/schema candidate. It does not add an internal vulnerability scanner, claim scanner authenticity or exploitability, cover a package/Skill, or create a tag or release.

### Uncertainty tested

Can a real external SARIF 2.1.0 finding be joined to an already observed MCP server only through stable artifact URI plus current SHA-256, fail policy, and produce standards-valid CycloneDX VDR output without retaining scanner messages or source content?

### Evidence gained

- The final OASIS SARIF 2.1.0 contract supplies portable scanner, rule, result level, invocation, suppression, and physical artifact-location fields. The official schema is checksum-pinned and validates the actual scanner output in CI.
- gosec 2.28.0 reports two real `G204` occurrences for the vulnerable MCP Go fixture. The importer groups them into one finding with occurrence evidence instead of multiplying graph identities.
- Runtime instrumentation supplies repository-relative artifact URI and SHA-256 to the already protocol-discovered MCP server. Import rehashes the current regular file and requires one exact, conflict-free graph target.
- gosec reports `main.go` relative to its scan root. An explicit second URI maps that scanner location to the digested repository artifact; no basename, suffix, display-name, framework, or source-content heuristic is used.
- Wrong digest and missing graph URI fail before output. A result for another exact URI is skipped. Invalid binding-critical SARIF, unsafe paths, failed scanner invocation, and conflicting target evidence are rejected.
- Policy can reject standard SARIF `error`, and CycloneDX maps the finding to `vulnerabilities[].affects`. The level remains SARIF metadata because `error` does not mean CycloneDX `high` or `critical`.
- Scanner messages, source snippets, regions, and fixes do not enter graph, policy, or BOM. Evidence is explicitly `scanner-reported` at `observed`, not independently verified.
- The first push exposed a newly published high-severity gRPC-Go DATA-frame fragmentation denial of service through GitHub Dependabot even though the pinned local `govulncheck` database reported no reachable issue. The directly exposed OTLP/gRPC dependency was raised from vulnerable 1.83.0 to patched 1.83.1 before accepting the candidate.
- Local focused tests and the real gosec → MCP/OTLP → graph → policy/CycloneDX bridge passed. Full regression and remote gate results are recorded in [`docs/evidence/v0.11.0.md`](evidence/v0.11.0.md).

### Decision

**Continue, but keep the scope narrow.** The single-file test confirms the project's evidence-layer advantage: it can correlate specialist scanner output with a component known from runtime evidence without duplicating scanning logic. That supports vulnerability context and deployment gates while preserving standards-first inputs and metadata-only output.

This does not justify a generic MCP/Skill scanner, a scanner marketplace, automatic remediation, or severity synthesis. External scanners remain responsible for detection; AI Evidence BOM owns identity correlation, provenance, evidence strength, drift, export, and policy.

### Risks and pivot signals

- A SARIF report may be forged, stale, incomplete, or wrong. Digest binding identifies the scanned file but does not authenticate the scanner or prove exploitability.
- Explicit scan-root URI mapping is operator-controlled. If real integrations cannot produce a deterministic mapping without per-scanner heuristics, stop broadening the importer rather than weakening identity matching.
- One file is not a server package. If a deterministic manifest cannot cover multi-file source and build inputs independent of path/order, retain single-file experimental status and do not market Skill/MCP package coverage.
- SARIF levels are not vulnerability severities. If users require CVSS/OSV enrichment, accept separately sourced and evidenced standard fields rather than infer them from `error`/`warning`.
- Runtime artifact attributes are current application instrumentation, not automatic MCP or OTel fields. If operators cannot emit trustworthy artifact identity, the finding bridge cannot attach safely.

### Next smallest falsifiable test

Define a deterministic multi-file artifact manifest/root digest and bind one observed MCP server to SARIF results in two files. Reordering the manifest, moving the checkout, or changing the scanner's scan root must not change identity; changing either file must break the binding.

The test fails if it needs absolute paths, scanner-specific result parsing, source-content retention, or a display-name fallback. Until it passes, keep `aiebom sarif` explicitly single-file and do not claim complete MCP/Skill vulnerability coverage.

## Release calibration template

For each later release, append a dated section containing:

- **Uncertainty tested:** one falsifiable question;
- **Evidence gained:** results, including negative results;
- **Decision:** continue, narrow, pause, or pivot;
- **Risks and pivot signals:** measurable conditions rather than optimism;
- **Next smallest falsifiable test:** one bounded step toward the current phase exit gate.
