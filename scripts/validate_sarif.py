#!/usr/bin/env python3
"""Validate a SARIF document against an explicit official JSON Schema."""

from __future__ import annotations

import json
import sys
from pathlib import Path

import jsonschema


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: validate_sarif.py SCHEMA SARIF", file=sys.stderr)
        return 2
    try:
        schema = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
        document = json.loads(Path(sys.argv[2]).read_text(encoding="utf-8"))
        validator_class = jsonschema.validators.validator_for(schema)
        validator_class.check_schema(schema)
        validator = validator_class(
            schema,
            format_checker=jsonschema.FormatChecker(),
        )
    except (OSError, json.JSONDecodeError, jsonschema.SchemaError) as exc:
        print(f"SARIF validation setup failed: {exc}", file=sys.stderr)
        return 2

    errors = sorted(
        validator.iter_errors(document),
        key=lambda error: (
            tuple(str(part) for part in error.absolute_path),
            error.message,
        ),
    )
    if not errors:
        print(f"{sys.argv[2]}: valid SARIF")
        return 0
    for error in errors[:20]:
        pointer = "/" + "/".join(str(part) for part in error.absolute_path)
        print(f"{sys.argv[2]}{pointer}: {error.message}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
