package normalize

import (
	"strings"
	"testing"
	"time"

	inputpkg "aibom-evidence/internal/input"
	"aibom-evidence/internal/model"
)

func TestBuildHashesButDoesNotRetainPrompt(t *testing.T) {
	secretPrompt := "internal system instructions"
	graph := BuildWithOptions([]inputpkg.Observation{{
		Timestamp: time.Unix(100, 0).UTC(),
		Level:     model.EvidenceObserved,
		Source:    "demo",
		TraceID:   "trace-1",
		Attributes: map[string]string{
			"service.name":               "agent-a",
			"gen_ai.request.model":       "model-a",
			"gen_ai.provider.name":       "provider-a",
			"gen_ai.system_instructions": secretPrompt,
		},
	}}, "demo", time.Unix(200, 0).UTC(), Options{SensitiveHMACKey: []byte("01234567890123456789012345678901")})
	if len(graph.Nodes) != 3 {
		t.Fatalf("nodes=%d want=3", len(graph.Nodes))
	}
	foundPrompt := false
	for _, node := range graph.Nodes {
		for _, value := range node.Properties {
			if strings.Contains(value, secretPrompt) {
				t.Fatal("prompt content leaked into node properties")
			}
		}
		if node.Type == "prompt" {
			foundPrompt = true
			if node.Digests["hmac-sha256"] == "" || node.Properties["content.retained"] != "false" {
				t.Fatalf("prompt evidence incomplete: %#v", node)
			}
		}
	}
	if !foundPrompt {
		t.Fatal("prompt evidence node not found")
	}
}
