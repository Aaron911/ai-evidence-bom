#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
output_dir="$(mktemp -d "${TMPDIR:-/tmp}/aiebom-dify.XXXXXX")"
port="${AIEBOM_LIVE_HTTP_PORT:-14319}"
binary="$output_dir/aiebom"
graph="$output_dir/evidence.json"
bom="$output_dir/bom.cdx.json"
receiver_log="$output_dir/receiver.log"
dify_commit="3ada29bbe06a33b9679b30f37a995562118aa173"
dify_checkout="${AIEBOM_DIFY_CHECKOUT:-$output_dir/dify}"

for required in curl git jq; do
  if ! command -v "$required" >/dev/null 2>&1; then
    echo "required command not found: $required" >&2
    exit 1
  fi
done
if [[ -z "${AIEBOM_LIVE_PYTHON:-}" ]] && ! command -v uv >/dev/null 2>&1; then
  echo "required command not found: uv" >&2
  exit 1
fi

if [[ ! -d "$dify_checkout/.git" ]]; then
  git init --quiet "$dify_checkout"
  git -C "$dify_checkout" remote add origin https://github.com/langgenius/dify.git
  git -C "$dify_checkout" config core.sparseCheckout true
  mkdir -p "$dify_checkout/.git/info"
  printf 'api/extensions/otel/\napi/pyproject.toml\n' >"$dify_checkout/.git/info/sparse-checkout"
  git -C "$dify_checkout" fetch --quiet --depth 1 origin "$dify_commit"
  git -C "$dify_checkout" checkout --quiet FETCH_HEAD
fi

actual_commit="$(git -C "$dify_checkout" rev-parse HEAD)"
if [[ "$actual_commit" != "$dify_commit" ]]; then
  echo "expected Dify commit $dify_commit, found $actual_commit" >&2
  exit 1
fi

cd "$repo_root"
go build -o "$binary" ./cmd/aiebom

"$binary" collect \
  --listen "127.0.0.1:$port" \
  --grpc-listen "" \
  --graph-out "$graph" \
  --bom-out "$bom" \
  >"$receiver_log" 2>&1 &
receiver_pid=$!

cleanup() {
  if kill -0 "$receiver_pid" 2>/dev/null; then
    kill -INT "$receiver_pid" 2>/dev/null || true
    wait "$receiver_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT

for _ in $(seq 1 50); do
  if curl --fail --silent "http://127.0.0.1:$port/healthz" >/dev/null; then
    break
  fi
  sleep 0.1
done
curl --fail --silent "http://127.0.0.1:$port/healthz" >/dev/null

python_command=(
  uv run --python 3.12 --no-project
  --with "pydantic==2.13.4"
  --with "opentelemetry-sdk==1.44.0"
  --with "opentelemetry-exporter-otlp-proto-http==1.44.0"
)
if [[ -n "${AIEBOM_LIVE_PYTHON:-}" ]]; then
  python_command=("$AIEBOM_LIVE_PYTHON")
fi

env \
  -u OTEL_EXPORTER_OTLP_ENDPOINT \
  -u OTEL_EXPORTER_OTLP_TRACES_ENDPOINT \
  -u OTEL_EXPORTER_OTLP_METRICS_ENDPOINT \
  -u OTEL_EXPORTER_OTLP_LOGS_ENDPOINT \
  AIEBOM_DIFY_API_ROOT="$dify_checkout/api" \
  AIEBOM_OTLP_TRACES_ENDPOINT="http://127.0.0.1:$port/v1/traces" \
  "${python_command[@]}" "$repo_root/scripts/live/dify_instrumentation_app.py"

for _ in $(seq 1 50); do
  if curl --fail --silent "http://127.0.0.1:$port/v1/evidence" >"$graph" && \
      jq -e 'any(.nodes[]; .type == "tool" and .name == "weather.lookup")' "$graph" >/dev/null; then
    break
  fi
  sleep 0.1
done

jq -e 'any(.nodes[]; .type == "agent" and .name == "travel-assistant-v1" and .properties["dify.app_id"] == "travel-assistant-v1")' "$graph" >/dev/null
jq -e 'any(.nodes[]; .type == "model" and .name == "gpt-5" and .provider == "openai")' "$graph" >/dev/null
jq -e 'any(.nodes[]; .type == "tool" and .name == "weather.lookup" and .properties["gen_ai.tool.type"] == "builtin")' "$graph" >/dev/null
jq -e '[.edges[].relation] | contains(["uses", "invokes"])' "$graph" >/dev/null

if grep -R "MUST_NOT_LEAK" "$graph" "$bom" >/dev/null 2>&1; then
  echo "sensitive marker leaked into generated evidence" >&2
  exit 1
fi

jq '{schemaVersion, nodes: [.nodes[] | {type, name, provider}], relations: [.edges[].relation]}' "$graph"
echo "instrumentation evidence: $graph"
echo "instrumentation BOM: $bom"
