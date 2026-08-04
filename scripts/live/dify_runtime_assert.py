#!/usr/bin/env python3
"""Run the imported Dify workflow and assert both deterministic nodes executed."""

import argparse
import json
from pathlib import Path

import httpx


INPUT_MARKER = "DIFY_RUNTIME_INPUT_MUST_NOT_LEAK"
TOOL_MARKER = "DIFY_RUNTIME_TOOL_ARGUMENT_MUST_NOT_LEAK"


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--state", type=Path, required=True)
    parser.add_argument("--base-url", default="http://localhost:5001")
    parser.add_argument("--app-id-out", type=Path, required=True)
    args = parser.parse_args()

    state = json.loads(args.state.read_text())
    app_id = state.get("app", {}).get("app_id")
    api_token = state.get("api_key", {}).get("token")
    if not app_id or not api_token:
        raise RuntimeError("Dify setup state is missing app ID or API token")

    response = httpx.post(
        f"{args.base_url}/v1/workflows/run",
        headers={"Authorization": f"Bearer {api_token}"},
        json={
            "inputs": {"question": INPUT_MARKER},
            "user": "aiebom-runtime-check",
            "response_mode": "blocking",
        },
        timeout=120.0,
    )
    response.raise_for_status()
    payload = response.json()
    data = payload.get("data", {})
    if data.get("status") != "succeeded" or data.get("error"):
        raise RuntimeError(f"Dify workflow failed with status {data.get('status')!r}")

    outputs = data.get("outputs", {})
    if INPUT_MARKER not in str(outputs.get("answer", "")):
        raise RuntimeError("mock LLM output did not prove that the LLM node executed")
    if TOOL_MARKER not in str(outputs.get("tool_result", "")):
        raise RuntimeError("tool output did not prove that the tool node executed")

    args.app_id_out.write_text(f"{app_id}\n")
    print("Dify workflow completed through the mock LLM and builtin tool")


if __name__ == "__main__":
    main()
