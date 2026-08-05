package input

import (
	"testing"
	"time"
)

func TestParseOTLPWithQuotedNanoseconds(t *testing.T) {
	data := []byte(`{
      "resourceSpans":[{
        "resource":{"attributes":[{"key":"service.name","value":{"stringValue":"demo"}}]},
        "scopeSpans":[{"spans":[{
          "traceId":"abc","spanId":"def","name":"chat",
          "startTimeUnixNano":"1785768000000000000",
          "attributes":[{"key":"gen_ai.request.model","value":{"stringValue":"model-a"}}]
        }]}]
      }]
    }`)
	observations, source, err := Parse(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if source != "demo" || len(observations) != 1 {
		t.Fatalf("unexpected parse result: source=%q observations=%d", source, len(observations))
	}
	want := time.Unix(0, 1785768000000000000).UTC()
	if !observations[0].Timestamp.Equal(want) {
		t.Fatalf("timestamp=%s want=%s", observations[0].Timestamp, want)
	}
}

func TestParseOTLPAcceptsEmptyExportRequest(t *testing.T) {
	observations, source, err := ParseOTLP([]byte(`{}`), "receiver")
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 0 || source != "receiver" {
		t.Fatalf("unexpected empty request result: source=%q observations=%d", source, len(observations))
	}
}

func TestParseCompactObservationPreservesParentSpan(t *testing.T) {
	data := []byte(`{
	  "source":"demo",
	  "observations":[{
	    "traceId":"trace-1",
	    "spanId":"child",
	    "parentSpanId":"parent",
	    "attributes":{"gen_ai.tool.name":"search"}
	  }]
	}`)
	observations, _, err := Parse(data, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || observations[0].ParentSpanID != "parent" {
		t.Fatalf("unexpected observations: %+v", observations)
	}
}

func TestParseOTLPPreservesScopeProvenance(t *testing.T) {
	data := []byte(`{
	  "resourceSpans":[{
	    "schemaUrl":"https://opentelemetry.io/schemas/1.43.0",
	    "resource":{"attributes":[{"key":"service.name","value":{"stringValue":"demo"}}]},
	    "scopeSpans":[{
	      "schemaUrl":"https://opentelemetry.io/schemas/gen-ai/1.42.0",
	      "scope":{"name":"demo.instrumentation","version":"2.0.0"},
	      "spans":[{"traceId":"abc","spanId":"def","parentSpanId":"123","name":"invoke_agent reviewer"}]
	    }]
	  }]
	}`)
	observations, _, err := ParseOTLP(data, "")
	if err != nil {
		t.Fatal(err)
	}
	attributes := observations[0].Attributes
	if attributes["otel.scope.name"] != "demo.instrumentation" || attributes["otel.scope.version"] != "2.0.0" {
		t.Fatalf("scope provenance missing: %v", attributes)
	}
	if attributes["otel.scope.schema_url"] != "https://opentelemetry.io/schemas/gen-ai/1.42.0" {
		t.Fatalf("scope schema URL missing: %v", attributes)
	}
	if observations[0].ParentSpanID != "123" {
		t.Fatalf("parent span ID=%q want=123", observations[0].ParentSpanID)
	}
}

func TestParseOTLPEvidenceExtensionCanOnlyDowngrade(t *testing.T) {
	data := []byte(`{
	  "resourceSpans":[{"scopeSpans":[{"spans":[
	    {"name":"declared","attributes":[{"key":"aiebom.evidence.level","value":{"stringValue":"declared"}}]},
	    {"name":"verified","attributes":[{"key":"aiebom.evidence.level","value":{"stringValue":"verified"}}]}
	  ]}]}]
	}`)
	observations, _, err := ParseOTLP(data, "receiver")
	if err != nil {
		t.Fatal(err)
	}
	if observations[0].Level != "declared" {
		t.Fatalf("declared level=%q", observations[0].Level)
	}
	if observations[1].Level != "observed" {
		t.Fatalf("verified promotion was accepted: %q", observations[1].Level)
	}
}
