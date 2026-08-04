package collector

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestReceiverRejectsBinaryProtobuf(t *testing.T) {
	receiver, err := New(Config{GraphOut: filepath.Join(t.TempDir(), "evidence.json")})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/traces", strings.NewReader("binary"))
	request.Header.Set("Content-Type", "application/x-protobuf")
	response := httptest.NewRecorder()
	receiver.ServeHTTP(response, request)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status=%d want=%d", response.Code, http.StatusUnsupportedMediaType)
	}
}
