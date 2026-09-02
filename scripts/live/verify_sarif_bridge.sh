#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
mkdir -p "$repo_dir/work"
task_dir="$(mktemp -d "$repo_dir/work/live-sarif-bridge.XXXXXX")"
collector_pid=""

cleanup() {
  if [[ -n "$collector_pid" ]] && kill -0 "$collector_pid" 2>/dev/null; then
    kill -TERM "$collector_pid" 2>/dev/null || true
    wait "$collector_pid" 2>/dev/null || true
  fi
  if [[ "${AIEBOM_KEEP_WORK:-}" == "1" ]]; then
    echo "preserving SARIF bridge work directory: $task_dir" >&2
    return
  fi
  case "${task_dir:-}" in
    "$repo_dir"/work/live-sarif-bridge.*)
      rm -rf -- "$task_dir"
      ;;
  esac
}
trap cleanup EXIT

hash_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "sha256sum or shasum is required" >&2
    return 2
  fi
}

for command_name in go curl jq python3; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "$command_name is required for the SARIF bridge check" >&2
    exit 2
  fi
done
if ! python3 -c 'import jsonschema' >/dev/null 2>&1; then
  echo "Python package jsonschema==4.26.0 is required" >&2
  exit 2
fi

gosec_bin="${GOSEC_BIN:-$(command -v gosec || true)}"
if [[ -z "$gosec_bin" ]] || [[ ! -x "$gosec_bin" ]]; then
  echo "gosec 2.28.0 is required; set GOSEC_BIN or add it to PATH" >&2
  exit 2
fi
gosec_version="$("$gosec_bin" -version 2>&1 | awk '/^Version:/ {print $2}')"
if [[ "$gosec_version" != "2.28.0" ]]; then
  echo "gosec 2.28.0 is required; found ${gosec_version:-unknown}" >&2
  exit 2
fi

sarif_schema_url="https://docs.oasis-open.org/sarif/sarif/v2.1.0/errata01/os/schemas/sarif-schema-2.1.0.json"
sarif_schema_sha256="c3b4bb2d6093897483348925aaa73af03b3e3f4bd4ca38cef26dcb4212a2682e"
cyclonedx_schema_url="https://raw.githubusercontent.com/CycloneDX/specification/1.7/schema/bom-1.7.schema.json"
cyclonedx_schema_sha256="df472ef4aaf593904c479293723a1a5c191d6672715c93b3c0b5c318f3914221"

curl --fail --silent --show-error --location --connect-timeout 20 --max-time 120 --retry 3 --retry-all-errors \
  --output "$task_dir/sarif-schema.json" "$sarif_schema_url"
curl --fail --silent --show-error --location --connect-timeout 20 --max-time 120 --retry 3 --retry-all-errors \
  --output "$task_dir/cyclonedx-schema.json" "$cyclonedx_schema_url"
if [[ "$(hash_file "$task_dir/sarif-schema.json")" != "$sarif_schema_sha256" ]]; then
  echo "official SARIF schema checksum mismatch" >&2
  exit 1
fi
if [[ "$(hash_file "$task_dir/cyclonedx-schema.json")" != "$cyclonedx_schema_sha256" ]]; then
  echo "official CycloneDX schema checksum mismatch" >&2
  exit 1
fi

cd "$repo_dir"
go build -o "$task_dir/aiebom" ./cmd/aiebom
go build -o "$task_dir/mcp-runtime" ./scripts/live/mcp_runtime

artifact_uri="scripts/live/mcp_runtime/main.go"
artifact_sha256="$(hash_file "$artifact_uri")"
sarif_artifact_uri="main.go"
"$gosec_bin" \
  -no-fail \
  -include=G204 \
  -fmt=sarif \
  -out="$task_dir/findings.sarif" \
  ./scripts/live/mcp_runtime \
  >"$task_dir/gosec.log" 2>&1
python3 scripts/validate_sarif.py "$task_dir/sarif-schema.json" "$task_dir/findings.sarif"

"$task_dir/aiebom" collect \
  --listen=127.0.0.1:14320 \
  --grpc-listen="" \
  --source=mcp-runtime-client \
  --graph-out="$task_dir/base.evidence.json" \
  >"$task_dir/collector.log" 2>&1 &
collector_pid="$!"
for _ in $(seq 1 100); do
  if curl -fsS http://127.0.0.1:14320/healthz >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done
curl -fsS http://127.0.0.1:14320/healthz >/dev/null

runtime_ok="false"
for attempt in 1 2 3; do
  if "$task_dir/mcp-runtime" \
    --role=client \
    --variant=vulnerable \
    --otlp-endpoint=http://127.0.0.1:14320/v1/traces \
    --artifact-uri="$artifact_uri" \
    --artifact-sha256="$artifact_sha256" \
    >"$task_dir/runtime.log" 2>&1; then
    runtime_ok="true"
    break
  fi
  if ! grep -Eq 'EOF|connection closed' "$task_dir/runtime.log"; then
    cat "$task_dir/runtime.log" >&2
    exit 1
  fi
  echo "transient MCP stdio startup failure on attempt $attempt; retrying" >&2
done
cat "$task_dir/runtime.log"
if [[ "$runtime_ok" != "true" ]]; then
  echo "MCP runtime did not start after 3 attempts" >&2
  exit 1
fi
kill -TERM "$collector_pid"
wait "$collector_pid"
collector_pid=""

jq -e --arg uri "$artifact_uri" --arg digest "$artifact_sha256" \
  '[.nodes[] | select(.type == "mcp_server" and .properties["aiebom.artifact.uri"] == $uri and .digests.sha256 == $digest)] | length == 1' \
  "$task_dir/base.evidence.json" >/dev/null

"$task_dir/aiebom" sarif \
  --input="$task_dir/base.evidence.json" \
  --sarif="$task_dir/findings.sarif" \
  --artifact="$artifact_uri" \
  --sarif-artifact-uri="$sarif_artifact_uri" \
  --output="$task_dir/enriched.evidence.json" \
  --bom-out="$task_dir/enriched.cdx.json"

jq -e --arg uri "$artifact_uri" --arg digest "$artifact_sha256" '
  ([.nodes[] | select(
    .type == "finding" and
    .provider == "gosec" and
    .name == "G204" and
    .properties["aiebom.finding.sarif.level"] == "error" and
    .properties["aiebom.finding.assertion"] == "scanner-reported" and
    .properties["aiebom.finding.artifact.uri"] == $uri and
    .properties["aiebom.finding.artifact.sha256"] == $digest and
    .evidence.observationCount == 2 and
    .evidence.sources == ["sarif:gosec@2.28.0"]
  )] | length == 1) and
  (. as $g | ($g.nodes[] | select(.type == "mcp_server") | .id) as $server |
    ($g.nodes[] | select(.type == "finding") | .id) as $finding |
    any($g.edges[]; .from == $server and .to == $finding and .relation == "affected_by"))
' "$task_dir/enriched.evidence.json" >/dev/null

jq -e '
  ([.components[] | select(.name == "G204")] | length == 0) and
  ([.vulnerabilities[] | select(
    .id == "G204" and
    .source.name == "gosec" and
    .properties[]? == {"name":"aibom:sarif:level","value":"error"}
  )] | length == 1)
' "$task_dir/enriched.cdx.json" >/dev/null
python3 scripts/validate_cyclonedx.py "$task_dir/cyclonedx-schema.json" "$task_dir/enriched.cdx.json"

set +e
"$task_dir/aiebom" policy \
  --input="$task_dir/enriched.evidence.json" \
  --policy=examples/policy-sarif-error.json \
  --output="$task_dir/policy.json" \
  2>"$task_dir/policy.stderr"
policy_status="$?"
set -e
if [[ "$policy_status" -ne 3 ]]; then
  echo "SARIF error finding did not fail policy with exit 3" >&2
  exit 1
fi
jq -e '.passed == false and ([.violations[] | select(.rule == "denied-finding-level")] | length == 1)' "$task_dir/policy.json" >/dev/null

jq '(.nodes[] | select(.type == "mcp_server") | .digests.sha256) = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"' \
  "$task_dir/base.evidence.json" >"$task_dir/wrong-digest.evidence.json"
set +e
"$task_dir/aiebom" sarif \
  --input="$task_dir/wrong-digest.evidence.json" \
  --sarif="$task_dir/findings.sarif" \
  --artifact="$artifact_uri" \
  --sarif-artifact-uri="$sarif_artifact_uri" \
  --output="$task_dir/wrong-digest-output.json" \
  2>"$task_dir/wrong-digest.stderr"
digest_status="$?"
set -e
if [[ "$digest_status" -eq 0 ]] || [[ -e "$task_dir/wrong-digest-output.json" ]]; then
  echo "digest mismatch attached a SARIF finding" >&2
  exit 1
fi
grep -Fq "SHA-256 did not" "$task_dir/wrong-digest.stderr"

jq '(.nodes[] | select(.type == "mcp_server") | .properties) |= del(."aiebom.artifact.uri")' \
  "$task_dir/base.evidence.json" >"$task_dir/name-only.evidence.json"
set +e
"$task_dir/aiebom" sarif \
  --input="$task_dir/name-only.evidence.json" \
  --sarif="$task_dir/findings.sarif" \
  --artifact="$artifact_uri" \
  --sarif-artifact-uri="$sarif_artifact_uri" \
  --output="$task_dir/name-only-output.json" \
  2>"$task_dir/name-only.stderr"
name_status="$?"
set -e
if [[ "$name_status" -eq 0 ]] || [[ -e "$task_dir/name-only-output.json" ]]; then
  echo "display-name-only match attached a SARIF finding" >&2
  exit 1
fi
grep -Fq "no component matches" "$task_dir/name-only.stderr"

if grep -F 'Subprocess launched with a potential tainted input or cmd arguments' \
  "$task_dir/enriched.evidence.json" "$task_dir/enriched.cdx.json" "$task_dir/policy.json" >/dev/null; then
  echo "SARIF message or source snippet reached evidence output" >&2
  exit 1
fi

echo "SARIF bridge check passed: real gosec output, exact artifact binding, policy failure, CycloneDX mapping, and metadata-only privacy"
