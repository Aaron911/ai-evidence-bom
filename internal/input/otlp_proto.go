package input

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

// ParseOTLPProto converts an OTLP protobuf ExportTraceServiceRequest into the
// same observations used by JSON and compact-input adapters.
func ParseOTLPProto(request *collectortracepb.ExportTraceServiceRequest, fallbackSource string) ([]Observation, string, error) {
	if request == nil {
		return nil, "", fmt.Errorf("OTLP protobuf request is nil")
	}
	var observations []Observation
	documentSource := fallbackSource
	for _, resourceSpans := range request.GetResourceSpans() {
		resourceAttrs := protoAttributes(resourceSpans.GetResource().GetAttributes())
		if resourceSpans.GetSchemaUrl() != "" {
			resourceAttrs["otel.resource.schema_url"] = resourceSpans.GetSchemaUrl()
		}
		serviceName := firstNonEmpty(resourceAttrs["service.name"], fallbackSource, "otlp")
		if documentSource == "" {
			documentSource = serviceName
		}
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			scope := scopeSpans.GetScope()
			for _, span := range scopeSpans.GetSpans() {
				attrs := make(map[string]string, len(resourceAttrs)+len(span.GetAttributes())+5)
				for key, value := range resourceAttrs {
					attrs[key] = value
				}
				for key, value := range protoAttributes(span.GetAttributes()) {
					attrs[key] = value
				}
				attrs["otel.span.name"] = span.GetName()
				if scope.GetName() != "" {
					attrs["otel.scope.name"] = scope.GetName()
				}
				if scope.GetVersion() != "" {
					attrs["otel.scope.version"] = scope.GetVersion()
				}
				if scopeSpans.GetSchemaUrl() != "" {
					attrs["otel.scope.schema_url"] = scopeSpans.GetSchemaUrl()
				}
				observations = append(observations, Observation{
					Timestamp:  protoNanoTime(span.GetStartTimeUnixNano()),
					Level:      model.EvidenceObserved,
					Source:     serviceName,
					TraceID:    hex.EncodeToString(span.GetTraceId()),
					SpanID:     hex.EncodeToString(span.GetSpanId()),
					Attributes: attrs,
				})
			}
		}
	}
	if documentSource == "" {
		documentSource = "otlp"
	}
	return observations, documentSource, nil
}

func protoAttributes(values []*commonpb.KeyValue) map[string]string {
	out := make(map[string]string, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		out[value.GetKey()] = protoAnyValue(value.GetValue())
	}
	return out
}

func protoAnyValue(value *commonpb.AnyValue) string {
	if value == nil {
		return ""
	}
	switch typed := value.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return typed.StringValue
	case *commonpb.AnyValue_BoolValue:
		return strconv.FormatBool(typed.BoolValue)
	case *commonpb.AnyValue_IntValue:
		return strconv.FormatInt(typed.IntValue, 10)
	case *commonpb.AnyValue_DoubleValue:
		return strconv.FormatFloat(typed.DoubleValue, 'g', -1, 64)
	case *commonpb.AnyValue_ArrayValue:
		items := make([]string, 0, len(typed.ArrayValue.GetValues()))
		for _, item := range typed.ArrayValue.GetValues() {
			items = append(items, protoAnyValue(item))
		}
		encoded, _ := json.Marshal(items)
		return string(encoded)
	case *commonpb.AnyValue_KvlistValue:
		encoded, _ := json.Marshal(protoAttributes(typed.KvlistValue.GetValues()))
		return string(encoded)
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(typed.BytesValue)
	default:
		return ""
	}
}

func protoNanoTime(value uint64) time.Time {
	if value == 0 || value > math.MaxInt64 {
		return time.Now().UTC()
	}
	return time.Unix(0, int64(value)).UTC()
}
