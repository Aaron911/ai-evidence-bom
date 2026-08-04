#!/usr/bin/env python3
"""Upload and install a pinned Dify plugin package without container egress."""

import argparse
import json
import time
from pathlib import Path

import httpx


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--state", type=Path, required=True)
    parser.add_argument("--package", type=Path, required=True)
    parser.add_argument("--expected-identifier", required=True)
    parser.add_argument("--base-url", default="http://localhost:5001")
    args = parser.parse_args()

    state = json.loads(args.state.read_text())
    auth = state.get("auth", {})
    access_token = auth.get("access_token")
    csrf_token = auth.get("csrf_token")
    if not access_token:
        raise RuntimeError("Dify setup state is missing its access token")

    headers = {"Authorization": f"Bearer {access_token}"}
    cookies = {"locale": "en-US", "access_token": access_token}
    if csrf_token:
        headers["X-CSRF-Token"] = csrf_token
        cookies["csrf_token"] = csrf_token

    with httpx.Client(base_url=args.base_url, headers=headers, cookies=cookies, timeout=120.0) as client:
        with args.package.open("rb") as package_stream:
            response = client.post(
                "/console/api/workspaces/current/plugin/upload/pkg",
                files={"pkg": (args.package.name, package_stream, "application/octet-stream")},
            )
        response.raise_for_status()
        identifier = response.json().get("unique_identifier")
        if identifier != args.expected_identifier:
            raise RuntimeError("uploaded Dify plugin identifier does not match the pinned package")

        response = client.post(
            "/console/api/workspaces/current/plugin/install/pkg",
            json={"plugin_unique_identifiers": [identifier]},
        )
        response.raise_for_status()
        install = response.json()
        if install.get("all_installed"):
            print("Pinned Dify plugin was already installed")
            return
        task_id = install.get("task_id")
        if not task_id:
            raise RuntimeError("Dify local plugin install did not return a task ID")

        for _ in range(60):
            time.sleep(1)
            response = client.get(f"/console/api/workspaces/current/plugin/tasks/{task_id}")
            response.raise_for_status()
            task = response.json().get("task", {})
            status = task.get("status")
            if status == "success":
                print("Pinned Dify plugin installed from a local package")
                return
            if status == "failed":
                messages = [str(item.get("message", "unknown error")) for item in task.get("plugins", [])]
                raise RuntimeError(f"Dify local plugin install failed: {'; '.join(messages)}")

    raise RuntimeError("Dify local plugin installation timed out")


if __name__ == "__main__":
    main()
