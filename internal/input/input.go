package input

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

type Observation struct {
	Timestamp    time.Time
	Level        model.EvidenceLevel
	Source       string
	TraceID      string
	SpanID       string
	ParentSpanID string
	Attributes   map[string]string
}

type simpleDocument struct {
	Source       string              `json:"source"`
	Observations []simpleObservation `json:"observations"`
}

type simpleObservation struct {
	Timestamp    time.Time           `json:"timestamp"`
	Level        model.EvidenceLevel `json:"level"`
	Source       string              `json:"source"`
	TraceID      string              `json:"traceId"`
	SpanID       string              `json:"spanId"`
	ParentSpanID string              `json:"parentSpanId"`
	Attributes   map[string]any      `json:"attributes"`
}

// Parse accepts either a compact observation document or OTLP JSON containing resourceSpans.
func Parse(data []byte, fallbackSource string) ([]Observation, string, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, "", fmt.Errorf("decode input JSON: %w", err)
	}
	if _, ok := probe["observations"]; ok {
		return parseSimple(data, fallbackSource)
	}
	if _, ok := probe["resourceSpans"]; ok {
		return ParseOTLP(data, fallbackSource)
	}
	return nil, "", fmt.Errorf("unsupported input: expected observations or resourceSpans")
}

func parseSimple(data []byte, fallbackSource string) ([]Observation, string, error) {
	var doc simpleDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, "", fmt.Errorf("decode observation document: %w", err)
	}
	source := firstNonEmpty(doc.Source, fallbackSource, "json-observations")
	out := make([]Observation, 0, len(doc.Observations))
	for _, raw := range doc.Observations {
		level := raw.Level
		if level.Rank() == 0 {
			level = model.EvidenceDeclared
		}
		timestamp := raw.Timestamp
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}
		out = append(out, Observation{
			Timestamp:    timestamp.UTC(),
			Level:        level,
			Source:       firstNonEmpty(raw.Source, source),
			TraceID:      raw.TraceID,
			SpanID:       raw.SpanID,
			ParentSpanID: raw.ParentSpanID,
			Attributes:   flattenMap(raw.Attributes),
		})
	}
	return out, source, nil
}

type otlpDocument struct {
	ResourceSpans []otlpResourceSpans `json:"resourceSpans"`
}

type otlpResourceSpans struct {
	Resource   otlpResource     `json:"resource"`
	ScopeSpans []otlpScopeSpans `json:"scopeSpans"`
	SchemaURL  string           `json:"schemaUrl"`
}

type otlpResource struct {
	Attributes []otlpKeyValue `json:"attributes"`
}

type otlpScopeSpans struct {
	Spans     []otlpSpan `json:"spans"`
	Scope     otlpScope  `json:"scope"`
	SchemaURL string     `json:"schemaUrl"`
}

type otlpScope struct {
	Name       string         `json:"name"`
	Version    string         `json:"version"`
	Attributes []otlpKeyValue `json:"attributes"`
}

type otlpSpan struct {
	TraceID           string          `json:"traceId"`
	SpanID            string          `json:"spanId"`
	ParentSpanID      string          `json:"parentSpanId"`
	Name              string          `json:"name"`
	StartTimeUnixNano json.RawMessage `json:"startTimeUnixNano"`
	Attributes        []otlpKeyValue  `json:"attributes"`
}

type otlpKeyValue struct {
	Key   string       `json:"key"`
	Value otlpAnyValue `json:"value"`
}

type otlpAnyValue struct {
	StringValue string           `json:"stringValue"`
	BoolValue   *bool            `json:"boolValue"`
	IntValue    json.Number      `json:"intValue"`
	DoubleValue *float64         `json:"doubleValue"`
	ArrayValue  *otlpArrayValue  `json:"arrayValue"`
	KVListValue *otlpKVListValue `json:"kvlistValue"`
	BytesValue  string           `json:"bytesValue"`
}

type otlpArrayValue struct {
	Values []otlpAnyValue `json:"values"`
}

type otlpKVListValue struct {
	Values []otlpKeyValue `json:"values"`
}

// ParseOTLP parses an OTLP ExportTraceServiceRequest encoded using the OTLP
// JSON mapping. An empty JSON message is a valid request with zero spans.
func ParseOTLP(data []byte, fallbackSource string) ([]Observation, string, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.UseNumber()
	var doc otlpDocument
	if err := decoder.Decode(&doc); err != nil {
		return nil, "", fmt.Errorf("decode OTLP JSON: %w", err)
	}
	var observations []Observation
	documentSource := fallbackSource
	for _, resourceSpans := range doc.ResourceSpans {
		resourceAttrs := attributes(resourceSpans.Resource.Attributes)
		if resourceSpans.SchemaURL != "" {
			resourceAttrs["otel.resource.schema_url"] = resourceSpans.SchemaURL
		}
		serviceName := firstNonEmpty(resourceAttrs["service.name"], fallbackSource, "otlp")
		if documentSource == "" {
			documentSource = serviceName
		}
		for _, scopeSpans := range resourceSpans.ScopeSpans {
			for _, span := range scopeSpans.Spans {
				attrs := make(map[string]string, len(resourceAttrs)+len(span.Attributes)+5)
				for key, value := range resourceAttrs {
					attrs[key] = value
				}
				for key, value := range attributes(span.Attributes) {
					attrs[key] = value
				}
				attrs["otel.span.name"] = span.Name
				if scopeSpans.Scope.Name != "" {
					attrs["otel.scope.name"] = scopeSpans.Scope.Name
				}
				if scopeSpans.Scope.Version != "" {
					attrs["otel.scope.version"] = scopeSpans.Scope.Version
				}
				if scopeSpans.SchemaURL != "" {
					attrs["otel.scope.schema_url"] = scopeSpans.SchemaURL
				}
				observations = append(observations, Observation{
					Timestamp:    nanoTime(span.StartTimeUnixNano),
					Level:        otlpEvidenceLevel(attrs),
					Source:       serviceName,
					TraceID:      span.TraceID,
					SpanID:       span.SpanID,
					ParentSpanID: span.ParentSpanID,
					Attributes:   attrs,
				})
			}
		}
	}
	if documentSource == "" {
		documentSource = "otlp"
	}
	return observations, documentSource, nil
}

// otlpEvidenceLevel accepts a project extension only when it lowers the
// default strength of runtime OTLP evidence. An exporter cannot promote a span
// to verified evidence merely by setting an attribute.
func otlpEvidenceLevel(attributes map[string]string) model.EvidenceLevel {
	switch model.EvidenceLevel(strings.TrimSpace(attributes["aiebom.evidence.level"])) {
	case model.EvidenceInferred:
		return model.EvidenceInferred
	case model.EvidenceDeclared:
		return model.EvidenceDeclared
	default:
		return model.EvidenceObserved
	}
}

func attributes(values []otlpKeyValue) map[string]string {
	out := make(map[string]string, len(values))
	for _, value := range values {
		out[value.Key] = anyValue(value.Value)
	}
	return out
}

func anyValue(value otlpAnyValue) string {
	switch {
	case value.StringValue != "":
		return value.StringValue
	case value.BoolValue != nil:
		return strconv.FormatBool(*value.BoolValue)
	case value.IntValue != "":
		return value.IntValue.String()
	case value.DoubleValue != nil:
		return strconv.FormatFloat(*value.DoubleValue, 'g', -1, 64)
	case value.ArrayValue != nil:
		items := make([]string, 0, len(value.ArrayValue.Values))
		for _, item := range value.ArrayValue.Values {
			items = append(items, anyValue(item))
		}
		encoded, _ := json.Marshal(items)
		return string(encoded)
	case value.KVListValue != nil:
		encoded, _ := json.Marshal(attributes(value.KVListValue.Values))
		return string(encoded)
	case value.BytesValue != "":
		return value.BytesValue
	default:
		return ""
	}
}

func flattenMap(values map[string]any) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		switch typed := value.(type) {
		case string:
			out[key] = typed
		case float64:
			out[key] = strconv.FormatFloat(typed, 'g', -1, 64)
		case bool:
			out[key] = strconv.FormatBool(typed)
		default:
			encoded, _ := json.Marshal(typed)
			out[key] = string(encoded)
		}
	}
	return out
}

func nanoTime(value json.RawMessage) time.Time {
	raw := strings.Trim(string(value), "\"")
	if raw == "" || raw == "null" {
		return time.Now().UTC()
	}
	nanos, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Now().UTC()
	}
	return time.Unix(0, nanos).UTC()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
