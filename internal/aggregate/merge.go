package aggregate

import (
	"sort"
	"strings"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

const maxTraceIDs = 20

// Merge combines a new evidence snapshot into an existing graph. Identity is
// stable across versions, so version drift remains visible to graph diffing.
func Merge(current, incoming model.Graph, generatedAt time.Time) model.Graph {
	result := cloneGraph(current)
	result.SchemaVersion = model.SchemaVersion
	result.GeneratedAt = generatedAt.UTC()
	if result.Source == "" {
		result.Source = incoming.Source
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	for key, value := range incoming.Metadata {
		result.Metadata[key] = value
	}
	result.Metadata["aggregation.mode"] = "continuous"

	nodes := make(map[string]int, len(result.Nodes))
	for index := range result.Nodes {
		result.Nodes[index].EnsureFieldEvidence()
		result.Nodes[index].ResolveFieldEvidence()
		nodes[result.Nodes[index].ID] = index
	}
	for _, candidate := range incoming.Nodes {
		candidate = cloneNode(candidate)
		candidate.EnsureFieldEvidence()
		candidate.ResolveFieldEvidence()
		index, exists := nodes[candidate.ID]
		if !exists {
			result.Nodes = append(result.Nodes, candidate)
			nodes[candidate.ID] = len(result.Nodes) - 1
			continue
		}
		mergeNode(&result.Nodes[index], candidate)
	}

	edges := make(map[string]int, len(result.Edges))
	for index := range result.Edges {
		edges[result.Edges[index].ID] = index
	}
	for _, candidate := range incoming.Edges {
		index, exists := edges[candidate.ID]
		if !exists {
			result.Edges = append(result.Edges, cloneEdge(candidate))
			edges[candidate.ID] = len(result.Edges) - 1
			continue
		}
		mergeEvidence(&result.Edges[index].Evidence, candidate.Evidence)
	}

	result.Canonicalize()
	for index := range result.Nodes {
		result.Nodes[index].Evidence.TraceIDs = limited(result.Nodes[index].Evidence.TraceIDs, maxTraceIDs)
		for fieldIndex := range result.Nodes[index].FieldEvidence {
			for valueIndex := range result.Nodes[index].FieldEvidence[fieldIndex].Values {
				evidence := &result.Nodes[index].FieldEvidence[fieldIndex].Values[valueIndex].Evidence
				evidence.TraceIDs = limited(evidence.TraceIDs, maxTraceIDs)
			}
		}
	}
	for index := range result.Edges {
		result.Edges[index].Evidence.TraceIDs = limited(result.Edges[index].Evidence.TraceIDs, maxTraceIDs)
	}
	return result
}

func mergeNode(current *model.Node, incoming model.Node) {
	for _, claim := range incoming.FieldEvidence {
		for _, value := range claim.Values {
			current.AddFieldEvidence(claim.Field, claim.Key, value.Value, value.Evidence)
		}
	}
	if current.Type == "" {
		current.Type = incoming.Type
	}
	if current.Name == "" {
		current.Name = incoming.Name
	}
	if current.Provider == "" {
		current.Provider = incoming.Provider
	}
	current.ObservedVersions = append(current.ObservedVersions, incoming.ObservedVersions...)
	if incoming.Version != "" {
		current.ObservedVersions = append(current.ObservedVersions, incoming.Version)
	}
	mergeEvidence(&current.Evidence, incoming.Evidence)
	current.ResolveFieldEvidence()
}

func mergeEvidence(current *model.EvidenceSummary, incoming model.EvidenceSummary) {
	model.MergeEvidenceSummary(current, incoming)
}

func cloneGraph(graph model.Graph) model.Graph {
	result := graph
	result.Metadata = cloneMap(graph.Metadata)
	result.Nodes = make([]model.Node, len(graph.Nodes))
	for index, node := range graph.Nodes {
		result.Nodes[index] = cloneNode(node)
	}
	result.Edges = make([]model.Edge, len(graph.Edges))
	for index, edge := range graph.Edges {
		result.Edges[index] = cloneEdge(edge)
	}
	return result
}

func cloneNode(node model.Node) model.Node {
	result := node
	result.ObservedVersions = append([]string(nil), node.ObservedVersions...)
	result.Digests = cloneMap(node.Digests)
	result.Properties = cloneMap(node.Properties)
	result.FieldEvidence = make([]model.FieldEvidence, len(node.FieldEvidence))
	for fieldIndex, claim := range node.FieldEvidence {
		result.FieldEvidence[fieldIndex] = claim
		result.FieldEvidence[fieldIndex].Values = make([]model.FieldValueEvidence, len(claim.Values))
		for valueIndex, value := range claim.Values {
			result.FieldEvidence[fieldIndex].Values[valueIndex] = value
			result.FieldEvidence[fieldIndex].Values[valueIndex].Evidence = cloneEvidence(value.Evidence)
		}
	}
	result.Evidence = cloneEvidence(node.Evidence)
	return result
}

func cloneEdge(edge model.Edge) model.Edge {
	result := edge
	result.Evidence = cloneEvidence(edge.Evidence)
	return result
}

func cloneEvidence(evidence model.EvidenceSummary) model.EvidenceSummary {
	result := evidence
	result.Sources = append([]string(nil), evidence.Sources...)
	result.TraceIDs = append([]string(nil), evidence.TraceIDs...)
	return result
}

func cloneMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return make(map[string]string)
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func limited(values []string, limit int) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}
