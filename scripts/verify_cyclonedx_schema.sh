#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
schema_url="https://raw.githubusercontent.com/CycloneDX/specification/1.7/schema/bom-1.7.schema.json"
schema_sha256="df472ef4aaf593904c479293723a1a5c191d6672715c93b3c0b5c318f3914221"

if [[ -z "${AIEBOM_CYCLONEDX_SCHEMA_FILE:-}" ]] && ! command -v curl >/dev/null 2>&1; then
  echo "curl is required to retrieve the pinned official CycloneDX schema" >&2
  exit 2
fi
if ! python3 -c 'import jsonschema' >/dev/null 2>&1; then
  echo "Python package jsonschema==4.26.0 is required" >&2
  exit 2
fi

mkdir -p "$repo_root/work"
task_tmp="$(mktemp -d "$repo_root/work/cyclonedx-schema.XXXXXX")"
cleanup() {
  case "${task_tmp:-}" in
    "$repo_root"/work/cyclonedx-schema.*)
      rm -rf -- "$task_tmp"
      ;;
  esac
}
trap cleanup EXIT

schema_path="$task_tmp/bom-1.7.schema.json"
graph_path="$task_tmp/evidence.json"
bom_path="$task_tmp/aiebom.cdx.json"
invalid_bom_path="$task_tmp/invalid.cdx.json"
negative_log="$task_tmp/negative-validation.log"

if [[ -n "${AIEBOM_CYCLONEDX_SCHEMA_FILE:-}" ]]; then
  cp -- "$AIEBOM_CYCLONEDX_SCHEMA_FILE" "$schema_path"
else
  curl \
    --fail \
    --silent \
    --show-error \
    --location \
    --connect-timeout 20 \
    --max-time 120 \
    --retry 3 \
    --retry-all-errors \
    --output "$schema_path" \
    "$schema_url"
fi

if command -v sha256sum >/dev/null 2>&1; then
  actual_sha256="$(sha256sum "$schema_path" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual_sha256="$(shasum -a 256 "$schema_path" | awk '{print $1}')"
else
  echo "sha256sum or shasum is required to verify the official schema" >&2
  exit 2
fi
if [[ "$actual_sha256" != "$schema_sha256" ]]; then
  echo "CycloneDX schema checksum mismatch: got $actual_sha256" >&2
  exit 1
fi

cd "$repo_root"
if [[ -n "${AIEBOM_BIN:-}" ]]; then
  "$AIEBOM_BIN" scan \
    --input examples/otlp-before.json \
    --graph-out "$graph_path" \
    --bom-out "$bom_path"
else
  go run ./cmd/aiebom scan \
    --input examples/otlp-before.json \
    --graph-out "$graph_path" \
    --bom-out "$bom_path"
fi

python3 scripts/validate_cyclonedx.py "$schema_path" "$bom_path"

python3 - "$bom_path" "$invalid_bom_path" <<'PY'
import json
import pathlib
import sys

source = pathlib.Path(sys.argv[1])
target = pathlib.Path(sys.argv[2])
bom = json.loads(source.read_text(encoding="utf-8"))
bom["bomFormat"] = "NotCycloneDX"
target.write_text(json.dumps(bom), encoding="utf-8")
PY

if python3 scripts/validate_cyclonedx.py "$schema_path" "$invalid_bom_path" >"$negative_log" 2>&1; then
  echo "negative CycloneDX schema check unexpectedly passed" >&2
  exit 1
fi
if ! grep -Fq "/bomFormat" "$negative_log"; then
  echo "negative CycloneDX schema check failed for an unexpected reason" >&2
  sed -n '1,40p' "$negative_log" >&2
  exit 1
fi

echo "CycloneDX 1.7 positive and negative schema checks passed"
