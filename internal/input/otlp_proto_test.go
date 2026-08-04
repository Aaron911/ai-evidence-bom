package input

import (
	"testing"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
)

func TestParseOTLPProtoPreservesTraceAndScope(t *testing.T) {
	timestamp := uint64(1785768000000000000)
	request := &collectortracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			SchemaUrl: "https://opentelemetry.io/schemas/1.43.0",
			Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
				stringKeyValue("service.name", "review-agent"),
			}},
			ScopeSpans: []*tracepb.ScopeSpans{{
				SchemaUrl: "https://opentelemetry.io/schemas/gen-ai/1.42.0",
				Scope:     &commonpb.InstrumentationScope{Name: "agent.instrumentation", Version: "3.0.0"},
				Spans: []*tracepb.Span{{
					TraceId:           []byte{0x01, 0x02, 0x03},
					SpanId:            []byte{0x04, 0x05},
					ParentSpanId:      []byte{0x06, 0x07},
					Name:              "chat gpt-5",
					StartTimeUnixNano: timestamp,
					Attributes: []*commonpb.KeyValue{
						stringKeyValue("gen_ai.request.model", "gpt-5"),
						boolKeyValue("gen_ai.request.stream", true),
					},
				}},
			}},
		}},
	}

	observations, source, err := ParseOTLPProto(request, "")
	if err != nil {
		t.Fatal(err)
	}
	if source != "review-agent" || len(observations) != 1 {
		t.Fatalf("unexpected parse result: source=%q observations=%d", source, len(observations))
	}
	observation := observations[0]
	if observation.TraceID != "010203" || observation.SpanID != "0405" || observation.ParentSpanID != "0607" {
		t.Fatalf("unexpected identifiers: trace=%q span=%q parent=%q", observation.TraceID, observation.SpanID, observation.ParentSpanID)
	}
	wantTime := time.Unix(0, int64(timestamp)).UTC()
	if !observation.Timestamp.Equal(wantTime) {
		t.Fatalf("timestamp=%s want=%s", observation.Timestamp, wantTime)
	}
	if observation.Attributes["otel.scope.name"] != "agent.instrumentation" || observation.Attributes["otel.scope.version"] != "3.0.0" {
		t.Fatalf("scope provenance missing: %v", observation.Attributes)
	}
	if observation.Attributes["gen_ai.request.stream"] != "true" {
		t.Fatalf("boolean attribute missing: %v", observation.Attributes)
	}
}

func TestParseOTLPProtoAcceptsEmptyRequest(t *testing.T) {
	observations, source, err := ParseOTLPProto(&collectortracepb.ExportTraceServiceRequest{}, "receiver")
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 0 || source != "receiver" {
		t.Fatalf("unexpected empty request result: source=%q observations=%d", source, len(observations))
	}
}

func stringKeyValue(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key: key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{
			StringValue: value,
		}},
	}
}

func boolKeyValue(key string, value bool) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key: key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_BoolValue{
			BoolValue: value,
		}},
	}
}
