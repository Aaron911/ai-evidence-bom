# Contributing

The project is in validation stage. Small, evidence-backed changes are preferred.

## Before opening a pull request

1. Add or update a synthetic fixture for every telemetry format change.
2. Do not include real prompts, credentials, customer hostnames, or trace data.
3. Preserve the metadata-only default.
4. Explain whether a new fact is inferred, declared, observed, or verified.
5. Run:

```bash
gofmt -w cmd internal
go test ./...
go vet ./...
python3 -m pip install 'jsonschema==4.26.0'
make validate-cyclonedx
```

The CycloneDX check downloads the official 1.7 JSON Schema from the tagged
CycloneDX specification repository, verifies its pinned SHA-256, validates a
real `aiebom scan` export, and proves that an intentionally invalid export is
rejected.

New adapters should translate into the internal evidence graph rather than adding vendor-specific concepts to the core model unless a standards mapping justifies them.
