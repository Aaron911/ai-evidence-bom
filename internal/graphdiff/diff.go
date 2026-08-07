package graphdiff

import (
	"reflect"
	"sort"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

type Diff struct {
	GeneratedAt  time.Time    `json:"generatedAt"`
	BeforeSource string       `json:"beforeSource,omitempty"`
	AfterSource  string       `json:"afterSource,omitempty"`
	AddedNodes   []model.Node `json:"addedNodes,omitempty"`
	RemovedNodes []model.Node `json:"removedNodes,omitempty"`
	ChangedNodes []NodeChange `json:"changedNodes,omitempty"`
	AddedEdges   []model.Edge `json:"addedEdges,omitempty"`
	RemovedEdges []model.Edge `json:"removedEdges,omitempty"`
	ChangedEdges []EdgeChange `json:"changedEdges,omitempty"`
}

type NodeChange struct {
	ID     string     `json:"id"`
	Fields []string   `json:"fields"`
	Before model.Node `json:"before"`
	After  model.Node `json:"after"`
}

type EdgeChange struct {
	ID     string     `json:"id"`
	Fields []string   `json:"fields"`
	Before model.Edge `json:"before"`
	After  model.Edge `json:"after"`
}

func Compare(before, after model.Graph, generatedAt time.Time) Diff {
	result := Diff{
		GeneratedAt:  generatedAt.UTC(),
		BeforeSource: before.Source,
		AfterSource:  after.Source,
	}
	beforeNodes := indexNodes(before.Nodes)
	afterNodes := indexNodes(after.Nodes)
	for id, node := range afterNodes {
		previous, exists := beforeNodes[id]
		if !exists {
			result.AddedNodes = append(result.AddedNodes, node)
			continue
		}
		if fields := changedFields(previous, node); len(fields) > 0 {
			result.ChangedNodes = append(result.ChangedNodes, NodeChange{
				ID: id, Fields: fields, Before: previous, After: node,
			})
		}
	}
	for id, node := range beforeNodes {
		if _, exists := afterNodes[id]; !exists {
			result.RemovedNodes = append(result.RemovedNodes, node)
		}
	}

	beforeEdges := indexEdges(before.Edges)
	afterEdges := indexEdges(after.Edges)
	for id, edge := range afterEdges {
		previous, exists := beforeEdges[id]
		if !exists {
			result.AddedEdges = append(result.AddedEdges, edge)
		} else if previous.Evidence.Level != edge.Evidence.Level {
			result.ChangedEdges = append(result.ChangedEdges, EdgeChange{
				ID: id, Fields: []string{"evidence.level"}, Before: previous, After: edge,
			})
		}
	}
	for id, edge := range beforeEdges {
		if _, exists := afterEdges[id]; !exists {
			result.RemovedEdges = append(result.RemovedEdges, edge)
		}
	}
	sort.Slice(result.AddedNodes, func(i, j int) bool { return result.AddedNodes[i].ID < result.AddedNodes[j].ID })
	sort.Slice(result.RemovedNodes, func(i, j int) bool { return result.RemovedNodes[i].ID < result.RemovedNodes[j].ID })
	sort.Slice(result.ChangedNodes, func(i, j int) bool { return result.ChangedNodes[i].ID < result.ChangedNodes[j].ID })
	sort.Slice(result.AddedEdges, func(i, j int) bool { return result.AddedEdges[i].ID < result.AddedEdges[j].ID })
	sort.Slice(result.RemovedEdges, func(i, j int) bool { return result.RemovedEdges[i].ID < result.RemovedEdges[j].ID })
	sort.Slice(result.ChangedEdges, func(i, j int) bool { return result.ChangedEdges[i].ID < result.ChangedEdges[j].ID })
	return result
}

func (d Diff) HasChanges() bool {
	return len(d.AddedNodes)+len(d.RemovedNodes)+len(d.ChangedNodes)+len(d.AddedEdges)+len(d.RemovedEdges)+len(d.ChangedEdges) > 0
}

func indexNodes(nodes []model.Node) map[string]model.Node {
	out := make(map[string]model.Node, len(nodes))
	for _, node := range nodes {
		out[node.ID] = node
	}
	return out
}

func indexEdges(edges []model.Edge) map[string]model.Edge {
	out := make(map[string]model.Edge, len(edges))
	for _, edge := range edges {
		out[edge.ID] = edge
	}
	return out
}

func changedFields(before, after model.Node) []string {
	var fields []string
	if before.Name != after.Name {
		fields = append(fields, "name")
	}
	if before.Version != after.Version || !reflect.DeepEqual(before.ObservedVersions, after.ObservedVersions) {
		fields = append(fields, "version")
	}
	if before.Provider != after.Provider {
		fields = append(fields, "provider")
	}
	if !reflect.DeepEqual(before.Digests, after.Digests) {
		fields = append(fields, "digests")
	}
	if !reflect.DeepEqual(before.Properties, after.Properties) {
		fields = append(fields, "properties")
	}
	if !reflect.DeepEqual(before.FieldEvidence, after.FieldEvidence) {
		fields = append(fields, "fieldEvidence")
	}
	if before.Evidence.Level != after.Evidence.Level {
		fields = append(fields, "evidence.level")
	}
	return fields
}
