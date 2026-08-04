#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
output_dir="$(mktemp -d "${TMPDIR:-/tmp}/aiebom-agent-framework.XXXXXX")"
port="${AIEBOM_LIVE_HTTP_PORT:-14318}"
binary="$output_dir/aiebom"
graph="$output_dir/evidence.json"
bom="$output_dir/bom.cdx.json"
receiver_log="$output_dir/receiver.log"

for required in curl jq; do
  if ! command -v "$required" >/dev/null 2>&1; then
    echo "required command not found: $required" >&2
    exit 1
  fi
done
if [[ -z "${AIEBOM_LIVE_PYTHON:-}" ]] && ! command -v uv >/dev/null 2>&1; then
  echo "required command not found: uv" >&2
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
  --with "agent-framework-core==1.13.0"
  --with "opentelemetry-exporter-otlp-proto-http==1.43.0"
)
if [[ -n "${AIEBOM_LIVE_PYTHON:-}" ]]; then
  python_command=("$AIEBOM_LIVE_PYTHON")
fi

env \
  -u OTEL_EXPORTER_OTLP_ENDPOINT \
  -u OTEL_EXPORTER_OTLP_TRACES_ENDPOINT \
  -u OTEL_EXPORTER_OTLP_METRICS_ENDPOINT \
  -u OTEL_EXPORTER_OTLP_LOGS_ENDPOINT \
  AIEBOM_OTLP_TRACES_ENDPOINT="http://127.0.0.1:$port/v1/traces" \
  OTEL_SERVICE_NAME="agent-framework-live" \
  OTEL_SERVICE_VERSION="1.13.0" \
  "${python_command[@]}" "$repo_root/scripts/live/agent_framework_app.py"

for _ in $(seq 1 50); do
  if curl --fail --silent "http://127.0.0.1:$port/v1/evidence" >"$graph" && \
      jq -e 'any(.nodes[]; .type == "tool" and .name == "weather.lookup")' "$graph" >/dev/null; then
    break
  fi
  sleep 0.1
done

jq -e 'any(.nodes[]; .type == "agent" and .properties["gen_ai.agent.id"] == "travel-assistant-v1")' "$graph" >/dev/null
jq -e 'any(.nodes[]; .type == "model" and .name == "gpt-5" and .provider == "openai")' "$graph" >/dev/null
jq -e 'any(.nodes[]; .type == "tool" and .name == "weather.lookup")' "$graph" >/dev/null
jq -e '[.edges[].relation] | contains(["uses", "invokes"])' "$graph" >/dev/null

if grep -R "MUST_NOT_LEAK" "$graph" "$bom" >/dev/null 2>&1; then
  echo "sensitive marker leaked into generated evidence" >&2
  exit 1
fi

jq '{schemaVersion, nodes: [.nodes[] | {type, name, provider}], relations: [.edges[].relation]}' "$graph"
echo "live evidence: $graph"
echo "live BOM: $bom"
