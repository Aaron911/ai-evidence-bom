.PHONY: build test vet validate-cyclonedx demo clean

build:
	go build -o ./bin/aiebom ./cmd/aiebom

test:
	go test ./...

vet:
	go vet ./...

validate-cyclonedx:
	scripts/verify_cyclonedx_schema.sh

demo: build
	mkdir -p work
	./bin/aiebom scan --input examples/otlp-before.json --graph-out work/before.evidence.json --bom-out work/before.cdx.json
	./bin/aiebom scan --input examples/otlp-after.json --graph-out work/after.evidence.json --bom-out work/after.cdx.json
	./bin/aiebom diff --before work/before.evidence.json --after work/after.evidence.json --output work/diff.json
	-./bin/aiebom policy --input work/after.evidence.json --policy examples/policy.json --output work/policy-report.json

clean:
	rm -f ./bin/aiebom
	rm -f ./work/*.json
