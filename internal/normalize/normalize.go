package normalize

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	inputpkg "aibom-evidence/internal/input"
	"aibom-evidence/internal/model"
)

type Builder struct {
	nodes   map[string]*model.Node
	edges   map[string]*model.Edge
	options Options
}

type Options struct {
	SensitiveHMACKey []byte
}

func Build(observations []inputpkg.Observation, source string, generatedAt time.Time) model.Graph {
	return BuildWithOptions(observations, source, generatedAt, Options{})
}

func BuildWithOptions(observations []inputpkg.Observation, source string, generatedAt time.Time, options Options) model.Graph {
	builder := &Builder{
		nodes:   make(map[string]*model.Node),
		edges:   make(map[string]*model.Edge),
		options: options,
	}
	for _, observation := range observations {
		builder.addObservation(observation)
	}
	graph := model.Graph{
		SchemaVersion: model.SchemaVersion,
		GeneratedAt:   generatedAt.UTC(),
		Source:        source,
		Nodes:         make([]model.Node, 0, len(builder.nodes)),
		Edges:         make([]model.Edge, 0, len(builder.edges)),
		Metadata: map[string]string{
			"privacy.mode":       "metadata-only",
			"normalizer.version": model.SchemaVersion,
		},
	}
	for _, node := range builder.nodes {
		graph.Nodes = append(graph.Nodes, *node)
	}
	for _, edge := range builder.edges {
		graph.Edges = append(graph.Edges, *edge)
	}
	graph.Canonicalize()
	return graph
}

func (b *Builder) addObservation(observation inputpkg.Observation) {
	attrs := observation.Attributes
	provider := first(attrs,
		"gen_ai.provider.name",
		"gen_ai.system",
		"llm.provider",
	)
	serviceName := first(attrs, "service.name", "service.namespace", "host.name")
	agentName := first(attrs, "gen_ai.agent.name", "agent.name")
	if agentName == "" && hasGenAIAttributes(attrs) {
		agentName = serviceName
	}
	agentVersion := first(attrs, "gen_ai.agent.version", "service.version")
	agentProvider := first(attrs, "gen_ai.agent.provider", "service.namespace")

	var agent *model.Node
	if agentName != "" {
		agent = b.addNode(nodeInput{
			Type:       "agent",
			Name:       agentName,
			Version:    agentVersion,
			Provider:   agentProvider,
			Properties: selected(attrs, "gen_ai.agent.id", "deployment.environment.name", "service.namespace"),
		}, observation)
	}

	requestedModel := first(attrs, "gen_ai.request.model", "llm.request.model", "gen_ai.model")
	responseModel := first(attrs, "gen_ai.response.model", "llm.response.model")
	modelName := firstNonEmpty(requestedModel, responseModel)
	if modelName != "" {
		version := ""
		if responseModel != "" && responseModel != modelName {
			version = responseModel
		}
		level := observation.Level
		if strings.EqualFold(first(attrs, "gen_ai.model.signature.verified", "model.signature.verified"), "true") {
			level = model.EvidenceVerified
		}
		modelObservation := observation
		modelObservation.Level = level
		digests := make(map[string]string)
		if digest := first(attrs, "gen_ai.model.digest", "model.digest"); digest != "" {
			digests["sha256"] = strings.TrimPrefix(digest, "sha256:")
		}
		modelNode := b.addNode(nodeInput{
			Type:     "model",
			Name:     modelName,
			Version:  version,
			Provider: provider,
			Digests:  digests,
			Properties: selected(attrs,
				"gen_ai.operation.name",
				"gen_ai.request.encoding_formats",
				"gen_ai.response.finish_reasons",
			),
		}, modelObservation)
		if agent != nil {
			b.addEdge(agent.ID, modelNode.ID, "uses", observation)
		}
	}

	mcpName := first(attrs, "mcp.server.name", "mcp.server.id", "mcp.server.url")
	var mcpNode *model.Node
	if mcpName != "" {
		mcpNode = b.addNode(nodeInput{
			Type:       "mcp_server",
			Name:       mcpName,
			Version:    first(attrs, "mcp.server.version"),
			Provider:   first(attrs, "mcp.server.provider"),
			Properties: selected(attrs, "mcp.server.url", "mcp.transport", "server.address", "server.port"),
		}, observation)
		if agent != nil {
			b.addEdge(agent.ID, mcpNode.ID, "connects_to", observation)
		}
	}

	toolName := first(attrs,
		"gen_ai.tool.name",
		"gen_ai.tool.call.name",
		"tool.name",
		"mcp.tool.name",
	)
	if toolName != "" {
		toolNode := b.addNode(nodeInput{
			Type:       "tool",
			Name:       toolName,
			Version:    first(attrs, "gen_ai.tool.version", "mcp.tool.version"),
			Provider:   firstNonEmpty(mcpName, provider),
			Properties: selected(attrs, "gen_ai.tool.type", "mcp.tool.read_only", "mcp.tool.destructive"),
		}, observation)
		if agent != nil {
			b.addEdge(agent.ID, toolNode.ID, "invokes", observation)
		}
		if mcpNode != nil {
			b.addEdge(toolNode.ID, mcpNode.ID, "provided_by", observation)
		}
	}

	dataSource := first(attrs,
		"gen_ai.data_source.id",
		"gen_ai.retrieval.data_source.id",
		"db.namespace",
		"data.source.name",
	)
	if dataSource != "" {
		dataNode := b.addNode(nodeInput{
			Type:       "data_source",
			Name:       dataSource,
			Provider:   first(attrs, "db.system.name", "server.address"),
			Properties: selected(attrs, "db.system.name", "server.address", "server.port"),
		}, observation)
		if agent != nil {
			b.addEdge(agent.ID, dataNode.ID, "reads_from", observation)
		}
	}

	b.addPromptEvidence(agent, observation)
}

func (b *Builder) addPromptEvidence(agent *model.Node, observation inputpkg.Observation) {
	attrs := observation.Attributes
	promptName := first(attrs, "gen_ai.prompt.template.name", "gen_ai.prompt.template.id")
	promptVersion := first(attrs, "gen_ai.prompt.template.version")
	promptContent := first(attrs, "gen_ai.system_instructions", "gen_ai.prompt")
	if promptName == "" && promptContent == "" {
		return
	}
	if promptName == "" {
		promptName = "system-instructions"
	}
	digests := make(map[string]string)
	if promptContent != "" && len(b.options.SensitiveHMACKey) > 0 {
		mac := hmac.New(sha256.New, b.options.SensitiveHMACKey)
		_, _ = mac.Write([]byte(promptContent))
		digests["hmac-sha256"] = hex.EncodeToString(mac.Sum(nil))
	}
	promptNode := b.addNode(nodeInput{
		Type:    "prompt",
		Name:    promptName,
		Version: promptVersion,
		Digests: digests,
		Properties: map[string]string{
			"content.retained": "false",
			"content.hashed":   truth(promptContent != "" && len(b.options.SensitiveHMACKey) > 0),
		},
	}, observation)
	if agent != nil {
		b.addEdge(agent.ID, promptNode.ID, "uses_prompt", observation)
	}
}

type nodeInput struct {
	Type       string
	Name       string
	Version    string
	Provider   string
	Digests    map[string]string
	Properties map[string]string
}

func (b *Builder) addNode(value nodeInput, observation inputpkg.Observation) *model.Node {
	id := model.StableNodeID(value.Type, value.Provider, value.Name)
	node, exists := b.nodes[id]
	if !exists {
		node = &model.Node{
			ID:         id,
			Type:       value.Type,
			Name:       value.Name,
			Version:    value.Version,
			Provider:   value.Provider,
			Digests:    clone(value.Digests),
			Properties: clone(value.Properties),
		}
		if node.Digests == nil {
			node.Digests = make(map[string]string)
		}
		if node.Properties == nil {
			node.Properties = make(map[string]string)
		}
		b.nodes[id] = node
	}
	if value.Version != "" {
		node.Version = value.Version
		node.ObservedVersions = append(node.ObservedVersions, value.Version)
	}
	mergeMap(node.Digests, value.Digests)
	mergeMap(node.Properties, value.Properties)
	mergeEvidence(&node.Evidence, observation)
	return node
}

func (b *Builder) addEdge(from, to, relation string, observation inputpkg.Observation) {
	id := model.StableEdgeID(from, relation, to)
	edge, exists := b.edges[id]
	if !exists {
		edge = &model.Edge{ID: id, From: from, To: to, Relation: relation}
		b.edges[id] = edge
	}
	mergeEvidence(&edge.Evidence, observation)
}

func mergeEvidence(summary *model.EvidenceSummary, observation inputpkg.Observation) {
	summary.Level = model.StrongerLevel(summary.Level, observation.Level)
	summary.ObservationCount++
	summary.Sources = append(summary.Sources, observation.Source)
	if observation.TraceID != "" && len(summary.TraceIDs) < 20 {
		summary.TraceIDs = append(summary.TraceIDs, observation.TraceID)
	}
	if summary.FirstSeen.IsZero() || observation.Timestamp.Before(summary.FirstSeen) {
		summary.FirstSeen = observation.Timestamp
	}
	if summary.LastSeen.IsZero() || observation.Timestamp.After(summary.LastSeen) {
		summary.LastSeen = observation.Timestamp
	}
}

func hasGenAIAttributes(attrs map[string]string) bool {
	for key := range attrs {
		if strings.HasPrefix(key, "gen_ai.") || strings.HasPrefix(key, "mcp.") {
			return true
		}
	}
	return false
}

func selected(attrs map[string]string, keys ...string) map[string]string {
	out := make(map[string]string)
	for _, key := range keys {
		if value := strings.TrimSpace(attrs[key]); value != "" {
			out[key] = value
		}
	}
	return out
}

func first(attrs map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(attrs[key]); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func clone(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	out := make(map[string]string, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func mergeMap(destination, source map[string]string) {
	for key, value := range source {
		destination[key] = value
	}
}

func truth(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// SortVersions is exported for tests and adapters that merge graphs.
func SortVersions(values []string) []string {
	sort.Strings(values)
	return values
}
