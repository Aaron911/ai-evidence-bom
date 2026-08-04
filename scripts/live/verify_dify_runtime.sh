#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
output_dir="$(mktemp -d "${TMPDIR:-/tmp}/aiebom-dify-runtime.XXXXXX")"
dify_commit="3ada29bbe06a33b9679b30f37a995562118aa173"
dify_source="${AIEBOM_DIFY_CHECKOUT:-}"
dify_checkout="$output_dir/dify"
http_port="${AIEBOM_LIVE_HTTP_PORT:-14318}"
api_port=5001
mock_port=5004
receiver_token="${AIEBOM_LIVE_TOKEN:-dify-runtime-receiver-token}"
compose_project="aiebom-dify-runtime-$$"
binary="$output_dir/aiebom"
graph="$output_dir/evidence.json"
bom="$output_dir/bom.cdx.json"
receiver_log="$output_dir/receiver.log"
mock_log="$output_dir/mock-openai.log"
app_id_file="$output_dir/app-id"
setup_log="$output_dir/setup-sensitive.log"
compose_started=false
receiver_pid=""
mock_pid=""

for required in curl docker git jq tar uv; do
  if ! command -v "$required" >/dev/null 2>&1; then
    echo "required command not found: $required" >&2
    exit 1
  fi
done
docker compose version >/dev/null

cleanup() {
  status=$?
  if [[ "$status" -ne 0 && "$compose_started" == true ]]; then
    "${compose[@]}" ps >&2 || true
    "${compose[@]}" logs --tail 200 api plugin_daemon >&2 || true
  fi
  if [[ "$compose_started" == true ]]; then
    "${compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
  fi
  for pid in "$mock_pid" "$receiver_pid"; do
    if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
      kill -INT "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    fi
  done
  if [[ "$status" -ne 0 ]]; then
    echo "Dify runtime check artifacts: $output_dir" >&2
    [[ -f "$receiver_log" ]] && tail -n 120 "$receiver_log" >&2 || true
    [[ -f "$mock_log" ]] && tail -n 120 "$mock_log" >&2 || true
  fi
  exit "$status"
}
trap cleanup EXIT

if [[ -n "$dify_source" ]]; then
  if [[ ! -d "$dify_source/.git" ]]; then
    echo "AIEBOM_DIFY_CHECKOUT is not a Git checkout: $dify_source" >&2
    exit 1
  fi
  actual_commit="$(git -C "$dify_source" rev-parse HEAD)"
  if [[ "$actual_commit" != "$dify_commit" ]]; then
    echo "expected Dify commit $dify_commit, found $actual_commit" >&2
    exit 1
  fi
  mkdir -p "$dify_checkout"
  git -C "$dify_source" archive "$dify_commit" docker scripts/stress-test | tar -x -C "$dify_checkout"
else
  git init --quiet "$dify_checkout"
  git -C "$dify_checkout" remote add origin https://github.com/langgenius/dify.git
  git -C "$dify_checkout" config core.sparseCheckout true
  mkdir -p "$dify_checkout/.git/info"
  printf '%s\n' \
    'docker/' \
    'scripts/stress-test/' \
    >"$dify_checkout/.git/info/sparse-checkout"
  git -C "$dify_checkout" fetch --quiet --depth 1 origin "$dify_commit"
  git -C "$dify_checkout" checkout --quiet FETCH_HEAD
  actual_commit="$(git -C "$dify_checkout" rev-parse HEAD)"
fi

if [[ "$actual_commit" != "$dify_commit" ]]; then
  echo "expected Dify commit $dify_commit, found $actual_commit" >&2
  exit 1
fi

cp "$dify_checkout/docker/.env.example" "$dify_checkout/docker/.env"
cp \
  "$repo_root/scripts/live/fixtures/dify-runtime-workflow.yml" \
  "$dify_checkout/scripts/stress-test/setup/dsl/workflow_llm.yml"
mkdir -p "$dify_checkout/scripts/stress-test/setup/config"

cd "$repo_root"
go build -o "$binary" ./cmd/aiebom
umask 077
printf '%s\n' "$receiver_token" >"$output_dir/receiver-token"

"$binary" collect \
  --listen "0.0.0.0:$http_port" \
  --grpc-listen "" \
  --auth-token-file "$output_dir/receiver-token" \
  --graph-out "$graph" \
  --bom-out "$bom" \
  >"$receiver_log" 2>&1 &
receiver_pid=$!

for _ in $(seq 1 100); do
  if curl --fail --silent "http://127.0.0.1:$http_port/healthz" >/dev/null; then
    break
  fi
  sleep 0.1
done
curl --fail --silent "http://127.0.0.1:$http_port/healthz" >/dev/null

uv run --python 3.12 --no-project --with "flask==3.1.3" -- \
  flask --app "$dify_checkout/scripts/stress-test/setup/mock_openai_server.py" \
  run --host 0.0.0.0 --port "$mock_port" \
  >"$mock_log" 2>&1 &
mock_pid=$!

for _ in $(seq 1 200); do
  if curl --fail --silent "http://127.0.0.1:$mock_port/health" >/dev/null; then
    break
  fi
  sleep 0.1
done
curl --fail --silent "http://127.0.0.1:$mock_port/health" >/dev/null

export AIEBOM_LIVE_HTTP_PORT="$http_port"
export AIEBOM_DIFY_API_PORT=5001
export AIEBOM_LIVE_TOKEN="$receiver_token"
export COMPOSE_PROFILES=postgresql
compose=(
  docker compose
  --project-name "$compose_project"
  --project-directory "$dify_checkout/docker"
  --env-file "$dify_checkout/docker/.env"
  -f "$dify_checkout/docker/docker-compose.yaml"
  -f "$repo_root/scripts/live/dify-runtime-compose.override.yaml"
)

compose_started=true
"${compose[@]}" up -d --wait db_postgres redis

if ! "${compose[@]}" exec -T db_postgres \
    psql -U postgres -d dify -tAc "SELECT 1 FROM pg_database WHERE datname='dify_plugin'" | grep -q 1; then
  "${compose[@]}" exec -T db_postgres createdb -U postgres dify_plugin
fi

"${compose[@]}" run --rm init_permissions
"${compose[@]}" up -d --no-deps plugin_daemon api

for _ in $(seq 1 600); do
  if curl --fail --silent "http://127.0.0.1:$api_port/health" >/dev/null; then
    break
  fi
  sleep 0.5
done
curl --fail --silent "http://127.0.0.1:$api_port/health" >/dev/null

python_command=(
  uv run --python 3.12 --no-project
  --with "httpx==0.28.1"
  --
)
setup_dir="$dify_checkout/scripts/stress-test/setup"

cd "$dify_checkout"
"${python_command[@]}" "$setup_dir/setup_admin.py"
"${python_command[@]}" "$setup_dir/login_admin.py"
"${python_command[@]}" "$setup_dir/install_openai_plugin.py"
"${python_command[@]}" "$setup_dir/configure_openai_plugin.py"
"${python_command[@]}" "$setup_dir/import_workflow_app.py"
"${python_command[@]}" "$setup_dir/create_api_key.py" >"$setup_log"
"${python_command[@]}" "$setup_dir/publish_workflow.py"

state_file="$setup_dir/config/stress_test_state.json"
"${python_command[@]}" "$repo_root/scripts/live/dify_runtime_assert.py" \
  --state "$state_file" \
  --base-url "http://127.0.0.1:$api_port" \
  --app-id-out "$app_id_file"
app_id="$(tr -d '\r\n' <"$app_id_file")"

auth_header="Authorization: Bearer $receiver_token"
for _ in $(seq 1 200); do
  curl --fail --silent -H "$auth_header" \
    "http://127.0.0.1:$http_port/v1/evidence" >"$graph"
  if jq -e --arg app_id "$app_id" '
      any(.nodes[]; .type == "agent" and .properties["dify.app_id"] == $app_id) and
      any(.nodes[]; .type == "model" and .name == "gpt-4o") and
      any(.nodes[]; .type == "tool" and .name == "time.current_time")
    ' "$graph" >/dev/null; then
    break
  fi
  sleep 0.1
done

jq -e --arg app_id "$app_id" '
  any(.nodes[]; .type == "agent" and .properties["dify.app_id"] == $app_id) and
  ([.nodes[] | select(.type == "agent")] | length == 1)
' "$graph" >/dev/null
jq -e 'any(.nodes[]; .type == "model" and .name == "gpt-4o" and ((.provider // "") | ascii_downcase | contains("openai")))' "$graph" >/dev/null
jq -e 'any(.nodes[]; .type == "tool" and .name == "time.current_time" and .properties["gen_ai.tool.type"] == "builtin")' "$graph" >/dev/null
jq -e '[.edges[].relation] | contains(["uses", "invokes"])' "$graph" >/dev/null

curl --fail --silent -H "$auth_header" \
  "http://127.0.0.1:$http_port/v1/bom" >"$bom"
if grep -R "MUST_NOT_LEAK" "$graph" "$bom" >/dev/null 2>&1; then
  echo "sensitive marker leaked into generated evidence" >&2
  exit 1
fi

jq '{schemaVersion, nodes: [.nodes[] | {type, name, provider}], relations: [.edges[].relation]}' "$graph"
echo "Dify 1.16.1 full runtime OTLP check passed ($actual_commit)"
echo "runtime evidence: $graph"
echo "runtime BOM: $bom"
