#!/usr/bin/env python3
"""Validate one or more JSON BOMs against a JSON Schema."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, TextIO

import jsonschema


def load_json(path: str, stdin: TextIO) -> Any:
    if path == "-":
        return json.load(stdin)
    with Path(path).open(encoding="utf-8") as handle:
        return json.load(handle)


def json_pointer(parts: list[Any]) -> str:
    if not parts:
        return "/"
    escaped = [str(part).replace("~", "~0").replace("/", "~1") for part in parts]
    return "/" + "/".join(escaped)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate CycloneDX JSON output against an explicit schema file."
    )
    parser.add_argument(
        "schema",
        help="JSON Schema path, or '-' to read the schema from standard input",
    )
    parser.add_argument("bom", nargs="+", help="CycloneDX JSON file(s) to validate")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        schema = load_json(args.schema, sys.stdin)
        validator_class = jsonschema.validators.validator_for(schema)
        validator_class.check_schema(schema)
        validator = validator_class(
            schema,
            format_checker=jsonschema.FormatChecker(),
        )
    except (OSError, json.JSONDecodeError, jsonschema.SchemaError) as exc:
        print(f"schema error: {exc}", file=sys.stderr)
        return 2

    failed = False
    schema_id = schema.get("$id", args.schema) if isinstance(schema, dict) else args.schema
    for bom_path in args.bom:
        try:
            bom = load_json(bom_path, sys.stdin)
        except (OSError, json.JSONDecodeError) as exc:
            print(f"{bom_path}: unable to read JSON: {exc}", file=sys.stderr)
            failed = True
            continue

        errors = sorted(
            validator.iter_errors(bom),
            key=lambda error: (
                tuple(str(part) for part in error.absolute_path),
                error.message,
            ),
        )
        if not errors:
            print(f"{bom_path}: valid against {schema_id}")
            continue

        failed = True
        print(f"{bom_path}: {len(errors)} schema validation error(s)", file=sys.stderr)
        for error in errors[:20]:
            print(
                f"{bom_path}{json_pointer(list(error.absolute_path))}: {error.message}",
                file=sys.stderr,
            )
        if len(errors) > 20:
            print(f"{bom_path}: {len(errors) - 20} additional error(s)", file=sys.stderr)

    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
