#!/usr/bin/env bash
set -euo pipefail

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
mkdir -p "$repo_dir/work"
task_dir="$(mktemp -d "$repo_dir/work/live-mcp-runtime.XXXXXX")"
runtime_bin="$repo_dir/work/mcp-runtime-live"
collector_pid=""

cleanup() {
  if [[ -n "$collector_pid" ]] && kill -0 "$collector_pid" 2>/dev/null; then
    kill -TERM "$collector_pid" 2>/dev/null || true
    wait "$collector_pid" 2>/dev/null || true
  fi
  rm -f "$runtime_bin"
  rm -rf "$task_dir"
}
trap cleanup EXIT

cd "$repo_dir"
go build -o "$task_dir/aiebom" ./cmd/aiebom
go build -o "$runtime_bin" ./scripts/live/mcp_runtime

run_variant() {
  local variant="$1"
  local port="$2"
  local graph="$task_dir/$variant.evidence.json"
  local bom="$task_dir/$variant.cdx.json"
  local log_file="$task_dir/$variant.collector.log"

  "$task_dir/aiebom" collect \
    --listen="127.0.0.1:$port" \
    --grpc-listen="" \
    --source="mcp-runtime-client" \
    --graph-out="$graph" \
    --bom-out="$bom" \
    >"$log_file" 2>&1 &
  collector_pid="$!"

  for _ in $(seq 1 100); do
    if curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null 2>&1; then
      break
    fi
    sleep 0.1
  done
  curl -fsS "http://127.0.0.1:$port/healthz" >/dev/null

  "$runtime_bin" \
    --role=client \
    --variant="$variant" \
    --otlp-endpoint="http://127.0.0.1:$port/v1/traces"

  kill -TERM "$collector_pid"
  wait "$collector_pid"
  collector_pid=""

  test -s "$graph"
  test -s "$bom"
}

run_variant before 14318
run_variant after 14319

before_graph="$task_dir/before.evidence.json"
after_graph="$task_dir/after.evidence.json"
diff_file="$task_dir/mcp.diff.json"
before_report="$task_dir/before.policy.json"
after_report="$task_dir/after.policy.json"

jq -e '[.nodes[] | select(.type == "agent" and .name == "security-agent" and .evidence.level == "observed")] | length == 1' "$before_graph" >/dev/null
jq -e '[.nodes[] | select(.type == "agent")] | length == 1' "$before_graph" >/dev/null
jq -e '[.nodes[] | select(.type == "mcp_server" and .name == "demo-security-tools" and .version == "1.0.0" and .evidence.level == "observed" and .properties["aiebom.mcp.server.identity_source"] == "server_info" and .properties["mcp.protocol.version"] == "2026-07-28" and .properties["network.transport"] == "pipe")] | length == 1' "$before_graph" >/dev/null
jq -e '[.nodes[] | select(.type == "tool" and .name == "weather.lookup" and .provider == "demo-security-tools" and .evidence.level == "observed")] | length == 1' "$before_graph" >/dev/null
jq -e '[.nodes[] | select(.type == "tool" and .name == "shell.execute")] | length == 0' "$before_graph" >/dev/null

jq -e '[.nodes[] | select(.type == "tool" and .name == "shell.execute" and .provider == "demo-security-tools" and .evidence.level == "declared" and .properties["aiebom.mcp.discovery.source"] == "tools/list" and .properties["aiebom.mcp.tool.annotations_untrusted"] == "true")] | length == 1' "$after_graph" >/dev/null
jq -e '([.nodes[] | select(.type == "agent")] | length == 1) and ([.nodes[] | select(.type == "mcp_server")] | length == 1) and ([.nodes[] | select(.type == "tool")] | length == 2)' "$after_graph" >/dev/null
jq -e '. as $g | ($g.nodes[] | select(.type == "agent") | .id) as $agent | ($g.nodes[] | select(.type == "mcp_server") | .id) as $server | ($g.nodes[] | select(.name == "shell.execute") | .id) as $shell | any($g.edges[]; .from == $agent and .to == $server and .relation == "connects_to") and any($g.edges[]; .from == $server and .to == $shell and .relation == "provides")' "$after_graph" >/dev/null

"$task_dir/aiebom" diff --before="$before_graph" --after="$after_graph" --output="$diff_file"
jq -e '([.addedNodes[] | select(.type == "tool" and .name == "shell.execute")] | length == 1) and ([.addedEdges[] | select(.relation == "provides")] | length == 1)' "$diff_file" >/dev/null

"$task_dir/aiebom" policy --input="$before_graph" --policy=examples/policy-mcp-capability.json --output="$before_report"
set +e
"$task_dir/aiebom" policy --input="$after_graph" --policy=examples/policy-mcp-capability.json --output="$after_report" 2>"$task_dir/after.policy.stderr"
policy_status="$?"
set -e
if [[ "$policy_status" -ne 3 ]]; then
  echo "changed MCP capability graph did not fail policy with exit 3" >&2
  exit 1
fi
jq -e '.passed == false and ([.violations[] | select(.rule == "denied-path:agent-to-shell-capability" and (.pathNodeIds | length) == 3 and .pathRelations == ["connects_to", "provides"])] | length == 1)' "$after_report" >/dev/null

if grep -E 'PRIVATE_MCP_(SCHEMA|ARGUMENT|RESULT)_MUST_NOT_LEAK' "$task_dir"/*.json >/dev/null; then
  echo "sensitive MCP content reached evidence output" >&2
  exit 1
fi

echo "MCP runtime live check passed: protocol discovery, stdio tool call, capability drift, path policy, and metadata-only privacy"
