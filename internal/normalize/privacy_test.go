package normalize

import (
	"strings"
	"testing"
	"time"

	inputpkg "github.com/Aaron911/ai-evidence-bom/internal/input"
	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

func TestMetadataOnlyObservationRetainsPromptPresenceAndHMACWithoutContent(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	observation := inputpkg.Observation{
		Timestamp: time.Unix(100, 0).UTC(),
		Level:     model.EvidenceObserved,
		Source:    "test",
		TraceID:   "trace",
		SpanID:    "span",
		Attributes: map[string]string{
			"service.name":               "review-agent",
			"gen_ai.request.model":       "gpt-5",
			"gen_ai.prompt":              "PRIVATE_PROMPT_MUST_NOT_LEAK",
			"gen_ai.input.messages":      "PRIVATE_INPUT_MUST_NOT_LEAK",
			"gen_ai.tool.call.arguments": "PRIVATE_ARGUMENT_MUST_NOT_LEAK",
		},
	}

	safe := MetadataOnlyObservation(observation, key)
	if safe.Attributes["service.name"] != "review-agent" || safe.Attributes["gen_ai.request.model"] != "gpt-5" {
		t.Fatalf("required identity metadata was removed: %+v", safe.Attributes)
	}
	if safe.Attributes[promptContentPresentAttribute] != "true" || safe.Attributes[promptHMACAttribute] == "" {
		t.Fatalf("prompt presence or HMAC missing: %+v", safe.Attributes)
	}
	for _, value := range safe.Attributes {
		if strings.Contains(value, "MUST_NOT_LEAK") {
			t.Fatalf("sensitive value retained: %+v", safe.Attributes)
		}
	}

	graph := BuildWithOptions([]inputpkg.Observation{safe}, "test", time.Unix(200, 0).UTC(), Options{
		SensitiveHMACKey: key,
	})
	for _, node := range graph.Nodes {
		if node.Type == "prompt" {
			if node.Properties["content.retained"] != "false" || node.Properties["content.hashed"] != "true" {
				t.Fatalf("unexpected prompt privacy properties: %+v", node)
			}
			if node.Digests["hmac-sha256"] != safe.Attributes[promptHMACAttribute] {
				t.Fatalf("prompt HMAC changed across delayed normalization: %+v", node.Digests)
			}
			return
		}
	}
	t.Fatal("sanitized prompt observation did not produce prompt evidence")
}
