package normalize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	inputpkg "github.com/Aaron911/ai-evidence-bom/internal/input"
	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

func TestSourceDerivedFrameworkFixturesShareSemanticContract(t *testing.T) {
	fixtures := []struct {
		name string
		path string
	}{
		{name: "dify", path: filepath.Join("..", "..", "examples", "frameworks", "dify-otlp.json")},
		{name: "microsoft-agent-framework", path: filepath.Join("..", "..", "examples", "frameworks", "microsoft-agent-framework-otlp.json")},
	}

	var baseline graphContract
	for index, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			data, err := os.ReadFile(fixture.path)
			if err != nil {
				t.Fatal(err)
			}
			observations, source, err := inputpkg.Parse(data, "")
			if err != nil {
				t.Fatal(err)
			}
			graph := Build(observations, source, time.Unix(200, 0).UTC())
			if len(graph.Nodes) < 3 || len(graph.Edges) < 2 {
				t.Fatalf("core graph is incomplete: nodes=%d edges=%d", len(graph.Nodes), len(graph.Edges))
			}

			encoded, err := json.Marshal(graph)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "MUST_NOT_LEAK") {
				t.Fatalf("sensitive fixture content leaked into graph: %s", encoded)
			}

			contract := semanticContract(graph)
			if index == 0 {
				baseline = contract
			} else if !reflect.DeepEqual(contract, baseline) {
				t.Fatalf("semantic contract differs\n%s: %#v\nbaseline: %#v", fixture.name, contract, baseline)
			}

			agentID := model.StableNodeID("agent", "", "travel-assistant-v1")
			assertNodeTypeProvider(t, graph, agentID, "agent", "")
			assertNode(t, graph, model.StableNodeID("model", "openai", "gpt-5"), "model", "gpt-5", "openai")
			assertNode(t, graph, model.StableNodeID("tool", "", "weather.lookup"), "tool", "weather.lookup", "")

			for _, node := range graph.Nodes {
				if node.Type == "model" && node.Provider == "microsoft.agent_framework" {
					t.Fatalf("framework provider was misclassified as model provider: %+v", node)
				}
			}
			for _, node := range graph.Nodes {
				if node.ID == agentID {
					if node.Version != "" {
						t.Fatalf("service version was misclassified as agent version: %+v", node)
					}
					if fixture.name == "dify" {
						if node.Properties["dify.app_id"] != "travel-assistant-v1" {
							t.Fatalf("Dify app identity missing: %+v", node)
						}
					}
				}
			}
		})
	}
}

type graphContract struct {
	nodes []string
	edges []string
}

func semanticContract(graph model.Graph) graphContract {
	contract := graphContract{
		nodes: make([]string, 0, len(graph.Nodes)),
		edges: make([]string, 0, len(graph.Edges)),
	}
	for _, node := range graph.Nodes {
		switch node.Type {
		case "agent", "model", "tool":
			contract.nodes = append(contract.nodes, node.ID+"|"+node.Type+"|"+node.Provider)
		}
	}
	for _, edge := range graph.Edges {
		switch edge.Relation {
		case "uses", "invokes":
			contract.edges = append(contract.edges, edge.From+"|"+edge.Relation+"|"+edge.To)
		}
	}
	return contract
}

func assertNodeTypeProvider(t *testing.T, graph model.Graph, id, nodeType, provider string) {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.ID == id {
			if node.Type != nodeType || node.Provider != provider {
				t.Fatalf("node %s mismatch: %+v", id, node)
			}
			return
		}
	}
	t.Fatalf("node %s not found: %+v", id, graph.Nodes)
}
