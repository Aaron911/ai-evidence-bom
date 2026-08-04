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
