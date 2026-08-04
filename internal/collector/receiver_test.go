package collector

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

const tracePayload = `{
  "resourceSpans": [{
    "resource": {"attributes": [{"key":"service.name","value":{"stringValue":"review-agent"}}]},
    "scopeSpans": [{"spans": [{
      "traceId":"5B8EFFF798038103D269B633813FC60C",
      "spanId":"EEE19B7EC3C1B174",
      "name":"chat gpt-5",
      "startTimeUnixNano":"1785768000000000000",
      "attributes": [
        {"key":"gen_ai.operation.name","value":{"stringValue":"chat"}},
        {"key":"gen_ai.provider.name","value":{"stringValue":"openai"}},
        {"key":"gen_ai.request.model","value":{"stringValue":"gpt-5"}},
        {"key":"gen_ai.input.messages","value":{"stringValue":"private user content"}},
        {"key":"gen_ai.output.messages","value":{"stringValue":"private model content"}},
        {"key":"gen_ai.tool.call.arguments","value":{"stringValue":"secret tool argument"}},
        {"key":"gen_ai.tool.call.result","value":{"stringValue":"secret tool result"}}
      ]
    }]}]
  }]
}`

func TestReceiverAcceptsAndDeduplicatesOTLPJSON(t *testing.T) {
	directory := t.TempDir()
	graphPath := filepath.Join(directory, "evidence.json")
	bomPath := filepath.Join(directory, "bom.json")
	receiver, err := New(Config{
		GraphOut:  graphPath,
		BOMOut:    bomPath,
		AuthToken: "secret-token",
		Now: func() time.Time {
			return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(tracePayload))
		request.Header.Set("Content-Type", "application/json; charset=utf-8")
		request.Header.Set("Authorization", "Bearer secret-token")
		response := httptest.NewRecorder()
		receiver.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("attempt %d status=%d body=%s", attempt, response.Code, response.Body.String())
		}
	}

	var graph model.Graph
	data, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &graph); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private user content", "private model content", "secret tool argument", "secret tool result"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("sensitive telemetry leaked into graph: %q", secret)
		}
	}
	if len(graph.Nodes) != 2 || graph.Nodes[0].Evidence.ObservationCount != 1 || graph.Nodes[1].Evidence.ObservationCount != 1 {
		t.Fatalf("duplicate changed evidence graph: %+v", graph.Nodes)
	}
	if _, err := os.Stat(bomPath); err != nil {
		t.Fatalf("CycloneDX snapshot was not written: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	request.Header.Set("Authorization", "Bearer secret-token")
	response := httptest.NewRecorder()
	receiver.ServeHTTP(response, request)
	var stats Stats
	if err := json.Unmarshal(response.Body.Bytes(), &stats); err != nil {
		t.Fatal(err)
	}
	if stats.Requests != 2 || stats.AcceptedSpans != 1 || stats.DuplicateSpans != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestReceiverRequiresToken(t *testing.T) {
	receiver, err := New(Config{GraphOut: filepath.Join(t.TempDir(), "evidence.json"), AuthToken: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	for _, authorization := range []string{"", "secret", "Basic secret"} {
		request := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(tracePayload))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", authorization)
		response := httptest.NewRecorder()
		receiver.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("authorization=%q status=%d want=%d", authorization, response.Code, http.StatusUnauthorized)
		}
	}
}

func TestReceiverAcceptsGzipAndRejectsOversizedPayload(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte(tracePayload)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	receiver, err := New(Config{GraphOut: filepath.Join(t.TempDir(), "evidence.json")})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(compressed.Bytes()))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Encoding", "gzip")
	response := httptest.NewRecorder()
	receiver.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("gzip status=%d body=%s", response.Code, response.Body.String())
	}

	smallReceiver, err := New(Config{
		GraphOut:        filepath.Join(t.TempDir(), "evidence.json"),
		MaxRequestBytes: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader(tracePayload))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	smallReceiver.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized status=%d want=%d body=%s", response.Code, http.StatusRequestEntityTooLarge, response.Body.String())
	}
}

func TestReceiverAcceptsBinaryProtobuf(t *testing.T) {
	graphPath := filepath.Join(t.TempDir(), "evidence.json")
	receiver, err := New(Config{GraphOut: graphPath})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := proto.Marshal(protoTraceRequest())
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	receiver.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/x-protobuf" {
		t.Fatalf("status=%d content-type=%q body=%x", response.Code, response.Header().Get("Content-Type"), response.Body.Bytes())
	}
	var exportResponse collectortracepb.ExportTraceServiceResponse
	if err := proto.Unmarshal(response.Body.Bytes(), &exportResponse); err != nil {
		t.Fatalf("decode protobuf response: %v", err)
	}
	data, err := os.ReadFile(graphPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("secret protobuf input")) {
		t.Fatal("sensitive protobuf attribute leaked into evidence graph")
	}
	if _, err := receiver.Export(context.Background(), protoTraceRequest()); err != nil {
		t.Fatalf("export same span over gRPC handler: %v", err)
	}
	receiver.mu.Lock()
	stats := receiver.stats
	receiver.mu.Unlock()
	if stats.Requests != 2 || stats.AcceptedSpans != 1 || stats.DuplicateSpans != 1 {
		t.Fatalf("cross-transport retry was not deduplicated: %+v", stats)
	}
}

func TestReceiverReturnsProtobufStatusForMalformedProtobuf(t *testing.T) {
	receiver, err := New(Config{GraphOut: filepath.Join(t.TempDir(), "evidence.json")})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/traces", bytes.NewReader([]byte{0xff}))
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	receiver.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || response.Header().Get("Content-Type") != "application/x-protobuf" {
		t.Fatalf("status=%d content-type=%q body=%x", response.Code, response.Header().Get("Content-Type"), response.Body.Bytes())
	}
	responseStatus := status.New(codes.Unknown, "").Proto()
	if err := proto.Unmarshal(response.Body.Bytes(), responseStatus); err != nil {
		t.Fatalf("decode protobuf status: %v", err)
	}
	if responseStatus.GetCode() != int32(codes.InvalidArgument) || !strings.Contains(responseStatus.GetMessage(), "decode OTLP protobuf") {
		t.Fatalf("unexpected protobuf status: code=%d message=%q", responseStatus.GetCode(), responseStatus.GetMessage())
	}
}

func TestReceiverServesOTLPGRPCWithAuthAndDeduplication(t *testing.T) {
	receiver, err := New(Config{
		GraphOut:  filepath.Join(t.TempDir(), "evidence.json"),
		AuthToken: "grpc-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	collectortracepb.RegisterTraceServiceServer(server, receiver)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	connection, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	client := collectortracepb.NewTraceServiceClient(connection)

	if _, err := client.Export(context.Background(), protoTraceRequest()); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("unauthenticated export error=%v", err)
	}
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Bearer grpc-secret"))
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := client.Export(ctx, protoTraceRequest()); err != nil {
			t.Fatalf("authenticated export %d: %v", attempt, err)
		}
	}
	receiver.mu.Lock()
	stats := receiver.stats
	receiver.mu.Unlock()
	if stats.Requests != 2 || stats.AcceptedSpans != 1 || stats.DuplicateSpans != 1 {
		t.Fatalf("unexpected gRPC stats: %+v", stats)
	}
	if stats.FailedRequests != 0 {
		t.Fatalf("authentication failures should not be counted as ingest failures: %+v", stats)
	}
}

func protoTraceRequest() *collectortracepb.ExportTraceServiceRequest {
	return &collectortracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
				protoStringKeyValue("service.name", "protobuf-agent"),
			}},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{Name: "protobuf.instrumentation", Version: "1.0.0"},
				Spans: []*tracepb.Span{{
					TraceId:           []byte{0x01, 0x02, 0x03, 0x04},
					SpanId:            []byte{0x05, 0x06, 0x07, 0x08},
					Name:              "chat gpt-5",
					StartTimeUnixNano: uint64(time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC).UnixNano()),
					Attributes: []*commonpb.KeyValue{
						protoStringKeyValue("gen_ai.operation.name", "chat"),
						protoStringKeyValue("gen_ai.provider.name", "openai"),
						protoStringKeyValue("gen_ai.request.model", "gpt-5"),
						protoStringKeyValue("gen_ai.input.messages", "secret protobuf input"),
					},
				}},
			}},
		}},
	}
}

func protoStringKeyValue(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key: key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{
			StringValue: value,
		}},
	}
}
