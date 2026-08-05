package normalize

import (
	"strings"
	"testing"
	"time"

	inputpkg "github.com/Aaron911/ai-evidence-bom/internal/input"
	"github.com/Aaron911/ai-evidence-bom/internal/model"
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

func TestBuildInfersStandardAgentAndToolSpanNames(t *testing.T) {
	timestamp := time.Unix(100, 0).UTC()
	graph := Build([]inputpkg.Observation{
		{
			Timestamp: timestamp,
			Level:     model.EvidenceObserved,
			Source:    "demo",
			Attributes: map[string]string{
				"service.name":          "orchestrator",
				"gen_ai.operation.name": "invoke_agent",
				"otel.span.name":        "invoke_agent reviewer",
			},
		},
		{
			Timestamp: timestamp,
			Level:     model.EvidenceObserved,
			Source:    "demo",
			Attributes: map[string]string{
				"service.name":          "orchestrator",
				"gen_ai.operation.name": "execute_tool",
				"otel.span.name":        "execute_tool search",
			},
		},
	}, "demo", timestamp)
	var foundAgent, foundTool bool
	for _, node := range graph.Nodes {
		foundAgent = foundAgent || node.Type == "agent" && node.Name == "reviewer"
		foundTool = foundTool || node.Type == "tool" && node.Name == "search"
	}
	if !foundAgent || !foundTool {
		t.Fatalf("standard span names were not normalized: %+v", graph.Nodes)
	}
}

func TestBuildCarriesAgentIdentityToChildSpans(t *testing.T) {
	timestamp := time.Unix(100, 0).UTC()
	graph := Build([]inputpkg.Observation{
		{
			Timestamp: timestamp,
			Level:     model.EvidenceObserved,
			Source:    "agent-service",
			TraceID:   "trace-1",
			SpanID:    "agent-span",
			Attributes: map[string]string{
				"service.name":          "agent-service",
				"gen_ai.operation.name": "invoke_agent",
				"gen_ai.agent.id":       "agent-1",
				"gen_ai.agent.name":     "travel-agent",
			},
		},
		{
			Timestamp:    timestamp,
			Level:        model.EvidenceObserved,
			Source:       "agent-service",
			TraceID:      "trace-1",
			SpanID:       "model-span",
			ParentSpanID: "agent-span",
			Attributes: map[string]string{
				"service.name":          "agent-service",
				"gen_ai.operation.name": "chat",
				"gen_ai.request.model":  "gpt-5",
				"gen_ai.provider.name":  "openai",
			},
		},
		{
			Timestamp:    timestamp,
			Level:        model.EvidenceObserved,
			Source:       "agent-service",
			TraceID:      "trace-1",
			SpanID:       "tool-span",
			ParentSpanID: "agent-span",
			Attributes: map[string]string{
				"service.name":          "agent-service",
				"gen_ai.operation.name": "execute_tool",
				"gen_ai.tool.name":      "weather.lookup",
			},
		},
	}, "agent-service", timestamp)

	if len(graph.Nodes) != 3 || len(graph.Edges) != 2 {
		t.Fatalf("unexpected graph size: nodes=%d edges=%d", len(graph.Nodes), len(graph.Edges))
	}
	agentID := model.StableNodeID("agent", "", "agent-1")
	modelID := model.StableNodeID("model", "openai", "gpt-5")
	toolID := model.StableNodeID("tool", "", "weather.lookup")
	assertNode(t, graph, agentID, "agent", "travel-agent", "")
	assertNode(t, graph, modelID, "model", "gpt-5", "openai")
	assertNode(t, graph, toolID, "tool", "weather.lookup", "")
	assertEdge(t, graph, agentID, modelID, "uses")
	assertEdge(t, graph, agentID, toolID, "invokes")
}

func TestBuildDoesNotTreatInvokeAgentSummaryAsSecondModel(t *testing.T) {
	timestamp := time.Unix(100, 0).UTC()
	graph := Build([]inputpkg.Observation{
		{
			Timestamp: timestamp,
			Level:     model.EvidenceObserved,
			Source:    "agent-service",
			TraceID:   "trace-1",
			SpanID:    "agent-span",
			Attributes: map[string]string{
				"gen_ai.operation.name": "invoke_agent",
				"gen_ai.agent.id":       "agent-1",
				"gen_ai.agent.name":     "travel-agent",
				"gen_ai.provider.name":  "framework-provider",
				"gen_ai.request.model":  "gpt-5",
			},
		},
		{
			Timestamp:    timestamp,
			Level:        model.EvidenceObserved,
			Source:       "agent-service",
			TraceID:      "trace-1",
			SpanID:       "model-span",
			ParentSpanID: "agent-span",
			Attributes: map[string]string{
				"gen_ai.operation.name": "chat",
				"gen_ai.provider.name":  "openai",
				"gen_ai.request.model":  "gpt-5",
			},
		},
	}, "agent-service", timestamp)

	models := 0
	for _, node := range graph.Nodes {
		if node.Type == "model" {
			models++
			if node.Provider != "openai" {
				t.Fatalf("model provider=%q want=openai", node.Provider)
			}
		}
	}
	if models != 1 {
		t.Fatalf("models=%d want=1; nodes=%+v", models, graph.Nodes)
	}
}

func TestConcreteModelSuppressesOnlyNearestAgentSummary(t *testing.T) {
	timestamp := time.Unix(100, 0).UTC()
	graph := Build([]inputpkg.Observation{
		{
			Timestamp: timestamp,
			Level:     model.EvidenceObserved,
			Source:    "agent-service",
			TraceID:   "trace-1",
			SpanID:    "outer-agent",
			Attributes: map[string]string{
				"gen_ai.operation.name": "invoke_agent",
				"gen_ai.agent.id":       "outer-agent",
				"gen_ai.agent.name":     "orchestrator",
				"gen_ai.provider.name":  "openai",
				"gen_ai.request.model":  "orchestrator-model",
			},
		},
		{
			Timestamp:    timestamp,
			Level:        model.EvidenceObserved,
			Source:       "agent-service",
			TraceID:      "trace-1",
			SpanID:       "inner-agent",
			ParentSpanID: "outer-agent",
			Attributes: map[string]string{
				"gen_ai.operation.name": "invoke_agent",
				"gen_ai.agent.id":       "inner-agent",
				"gen_ai.agent.name":     "specialist",
				"gen_ai.provider.name":  "framework-provider",
				"gen_ai.request.model":  "specialist-model",
			},
		},
		{
			Timestamp:    timestamp,
			Level:        model.EvidenceObserved,
			Source:       "agent-service",
			TraceID:      "trace-1",
			SpanID:       "model-span",
			ParentSpanID: "inner-agent",
			Attributes: map[string]string{
				"gen_ai.operation.name": "chat",
				"gen_ai.provider.name":  "openai",
				"gen_ai.request.model":  "specialist-model",
			},
		},
	}, "agent-service", timestamp)

	var models []model.Node
	for _, node := range graph.Nodes {
		if node.Type == "model" {
			models = append(models, node)
		}
	}
	if len(models) != 2 {
		t.Fatalf("models=%d want=2; nodes=%+v", len(models), graph.Nodes)
	}
	assertNode(t, graph, model.StableNodeID("model", "openai", "orchestrator-model"), "model", "orchestrator-model", "openai")
	assertNode(t, graph, model.StableNodeID("model", "openai", "specialist-model"), "model", "specialist-model", "openai")
}

func TestBuildMergesDeclaredMCPDiscoveryWithObservedToolCall(t *testing.T) {
	timestamp := time.Unix(100, 0).UTC()
	serverAttributes := map[string]string{
		"aiebom.mcp.server.name":            "demo-security-tools",
		"aiebom.mcp.server.version":         "1.0.0",
		"aiebom.mcp.server.identity_source": "server_info",
		"aiebom.mcp.discovery.source":       "tools/list",
		"mcp.protocol.version":              "2026-07-28",
		"network.transport":                 "pipe",
	}
	declaration := func(toolName string) inputpkg.Observation {
		attributes := clone(serverAttributes)
		attributes["service.name"] = "mcp-runtime-client"
		attributes["gen_ai.tool.name"] = toolName
		attributes["aiebom.mcp.tool.input_schema_sha256"] = "schema-digest"
		return inputpkg.Observation{
			Timestamp:  timestamp,
			Level:      model.EvidenceDeclared,
			Source:     "mcp-runtime-client",
			Attributes: attributes,
		}
	}
	graph := Build([]inputpkg.Observation{
		declaration("weather.lookup"),
		declaration("shell.execute"),
		{
			Timestamp: timestamp.Add(time.Second),
			Level:     model.EvidenceObserved,
			Source:    "mcp-runtime-client",
			TraceID:   "trace-1",
			SpanID:    "agent-span",
			Attributes: map[string]string{
				"service.name":          "mcp-runtime-client",
				"gen_ai.operation.name": "invoke_agent",
				"gen_ai.agent.id":       "security-agent",
				"gen_ai.agent.name":     "security-agent",
			},
		},
		{
			Timestamp:    timestamp.Add(2 * time.Second),
			Level:        model.EvidenceObserved,
			Source:       "mcp-runtime-client",
			TraceID:      "trace-1",
			SpanID:       "tool-span",
			ParentSpanID: "agent-span",
			Attributes: map[string]string{
				"service.name":                      "mcp-runtime-client",
				"gen_ai.operation.name":             "execute_tool",
				"gen_ai.tool.name":                  "weather.lookup",
				"aiebom.mcp.server.name":            "demo-security-tools",
				"aiebom.mcp.server.version":         "1.0.0",
				"aiebom.mcp.server.identity_source": "server_info",
				"mcp.method.name":                   "tools/call",
				"mcp.protocol.version":              "2026-07-28",
				"network.transport":                 "pipe",
			},
		},
	}, "mcp-runtime-client", timestamp.Add(3*time.Second))

	agentID := model.StableNodeID("agent", "", "security-agent")
	serverID := model.StableNodeID("mcp_server", "", "demo-security-tools")
	weatherID := model.StableNodeID("tool", "demo-security-tools", "weather.lookup")
	shellID := model.StableNodeID("tool", "demo-security-tools", "shell.execute")
	if len(graph.Nodes) != 4 || len(graph.Edges) != 4 {
		t.Fatalf("unexpected graph size: nodes=%d edges=%d\n%+v\n%+v", len(graph.Nodes), len(graph.Edges), graph.Nodes, graph.Edges)
	}
	assertEdge(t, graph, agentID, serverID, "connects_to")
	assertEdge(t, graph, agentID, weatherID, "invokes")
	assertEdge(t, graph, serverID, weatherID, "provides")
	assertEdge(t, graph, serverID, shellID, "provides")
	for _, node := range graph.Nodes {
		if node.ID == shellID && node.Evidence.Level != model.EvidenceDeclared {
			t.Fatalf("uninvoked capability evidence=%q want=declared", node.Evidence.Level)
		}
		if node.Type == "agent" && node.ID != agentID {
			t.Fatalf("MCP discovery invented a service fallback agent: %+v", node)
		}
	}
}

func assertNode(t *testing.T, graph model.Graph, id, nodeType, name, provider string) {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			if node.Type != nodeType || node.Name != name || node.Provider != provider {
				t.Fatalf("node %s mismatch: %+v", id, node)
			}
			return
		}
	}
	t.Fatalf("node %s not found: %+v", id, graph.Nodes)
}

func assertEdge(t *testing.T, graph model.Graph, from, to, relation string) {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.From == from && edge.To == to && edge.Relation == relation {
			return
		}
	}
	t.Fatalf("edge %s -[%s]-> %s not found: %+v", from, relation, to, graph.Edges)
}
