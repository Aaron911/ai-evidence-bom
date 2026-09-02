package normalize

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	inputpkg "github.com/Aaron911/ai-evidence-bom/internal/input"
	"github.com/Aaron911/ai-evidence-bom/internal/model"
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
	for _, observation := range prepareObservations(observations) {
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
	for nodeIndex := range graph.Nodes {
		for fieldIndex := range graph.Nodes[nodeIndex].FieldEvidence {
			for valueIndex := range graph.Nodes[nodeIndex].FieldEvidence[fieldIndex].Values {
				traceIDs := graph.Nodes[nodeIndex].FieldEvidence[fieldIndex].Values[valueIndex].Evidence.TraceIDs
				if len(traceIDs) > 20 {
					graph.Nodes[nodeIndex].FieldEvidence[fieldIndex].Values[valueIndex].Evidence.TraceIDs = traceIDs[:20]
				}
			}
		}
	}
	return graph
}

func (b *Builder) addObservation(observation inputpkg.Observation) {
	attrs := observation.Attributes
	operation := first(attrs, "gen_ai.operation.name")
	spanName := first(attrs, "otel.span.name")
	provider := first(attrs,
		"gen_ai.provider.name",
		"gen_ai.system",
		"llm.provider",
	)
	serviceName := first(attrs, "service.name", "service.namespace", "host.name")
	agentName := first(attrs, "gen_ai.agent.name", "agent.name")
	agentNameFromService := false
	mcpDeclaration := first(attrs, "aiebom.mcp.discovery.source") != ""
	if agentName == "" && operation == "invoke_agent" {
		agentName = spanEntityName(spanName, "invoke_agent")
	}
	if agentName == "" && hasGenAIAttributes(attrs) && !mcpDeclaration {
		agentName = serviceName
		agentNameFromService = agentName != ""
	}
	agentVersion := first(attrs, "gen_ai.agent.version")
	if agentVersion == "" && agentNameFromService {
		agentVersion = first(attrs, "service.version")
	}
	agentProvider := first(attrs, "gen_ai.agent.provider", "service.namespace")

	var agent *model.Node
	if agentName != "" {
		agent = b.addNode(nodeInput{
			Type:     "agent",
			Name:     agentName,
			Identity: firstNonEmpty(first(attrs, "gen_ai.agent.id"), agentName),
			Version:  agentVersion,
			Provider: agentProvider,
			Properties: selected(attrs,
				"gen_ai.agent.id",
				"gen_ai.framework",
				"dify.app_id",
				"dify.workflow_id",
				"deployment.environment.name",
				"service.namespace",
				"otel.scope.name",
				"otel.scope.version",
				"otel.scope.schema_url",
			),
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
				"server.address",
				"server.port",
				"otel.scope.name",
				"otel.scope.version",
				"otel.scope.schema_url",
			),
		}, observation)
		if agent != nil {
			b.addEdge(agent.ID, modelNode.ID, "uses", observation)
		}
	}

	mcpName := first(attrs,
		"aiebom.mcp.server.name",
		"aiebom.mcp.server.id",
		"mcp.server.name",
		"mcp.server.id",
		"mcp.server.url",
	)
	var mcpNode *model.Node
	if mcpName != "" {
		mcpDigests := make(map[string]string)
		if digest := first(attrs, "aiebom.artifact.sha256"); digest != "" {
			mcpDigests["sha256"] = strings.TrimPrefix(digest, "sha256:")
		}
		mcpNode = b.addNode(nodeInput{
			Type:     "mcp_server",
			Name:     mcpName,
			Identity: firstNonEmpty(first(attrs, "aiebom.mcp.server.id", "mcp.server.id"), mcpName),
			Version:  first(attrs, "aiebom.mcp.server.version", "mcp.server.version"),
			Provider: first(attrs, "aiebom.mcp.server.provider", "mcp.server.provider"),
			Digests:  mcpDigests,
			Properties: selected(attrs,
				"aiebom.artifact.uri",
				"aiebom.mcp.server.identity_source",
				"aiebom.mcp.discovery.source",
				"mcp.protocol.version",
				"mcp.method.name",
				"mcp.server.url",
				"mcp.transport",
				"network.transport",
				"server.address",
				"server.port",
			),
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
	if toolName == "" && operation == "execute_tool" {
		toolName = spanEntityName(spanName, "execute_tool")
	}
	if toolName != "" {
		toolNode := b.addNode(nodeInput{
			Type:     "tool",
			Name:     toolName,
			Version:  first(attrs, "gen_ai.tool.version", "aiebom.mcp.tool.version", "mcp.tool.version"),
			Provider: firstNonEmpty(mcpName, provider),
			Properties: selected(attrs,
				"gen_ai.tool.type",
				"aiebom.mcp.discovery.source",
				"aiebom.mcp.tool.input_schema_sha256",
				"aiebom.mcp.tool.read_only",
				"aiebom.mcp.tool.destructive",
				"aiebom.mcp.tool.annotations_untrusted",
				"mcp.tool.read_only",
				"mcp.tool.destructive",
				"mcp.method.name",
				"mcp.protocol.version",
				"network.transport",
			),
		}, observation)
		if agent != nil {
			b.addEdge(agent.ID, toolNode.ID, "invokes", observation)
		}
		if mcpNode != nil {
			b.addEdge(mcpNode.ID, toolNode.ID, "provides", observation)
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
	promptPresent := promptContent != "" || first(attrs, promptContentPresentAttribute) == "true"
	if promptName == "" && !promptPresent {
		return
	}
	if promptName == "" {
		promptName = "system-instructions"
	}
	digests := make(map[string]string)
	if fingerprint := first(attrs, promptHMACAttribute); fingerprint != "" {
		digests["hmac-sha256"] = fingerprint
	} else if promptContent != "" && len(b.options.SensitiveHMACKey) > 0 {
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
			"content.hashed":   truth(digests["hmac-sha256"] != ""),
		},
	}, observation)
	if agent != nil {
		b.addEdge(agent.ID, promptNode.ID, "uses_prompt", observation)
	}
}

type nodeInput struct {
	Type       string
	Name       string
	Identity   string
	Version    string
	Provider   string
	Digests    map[string]string
	Properties map[string]string
}

func (b *Builder) addNode(value nodeInput, observation inputpkg.Observation) *model.Node {
	identity := firstNonEmpty(value.Identity, value.Name)
	id := model.StableNodeID(value.Type, value.Provider, identity)
	node, exists := b.nodes[id]
	if !exists {
		node = &model.Node{
			ID:         id,
			Type:       value.Type,
			Name:       value.Name,
			Provider:   value.Provider,
			Digests:    make(map[string]string),
			Properties: make(map[string]string),
		}
		b.nodes[id] = node
	}
	claimEvidence := observationEvidence(observation)
	if value.Version != "" {
		node.ObservedVersions = append(node.ObservedVersions, value.Version)
		node.AddFieldEvidence(model.FieldVersion, "", value.Version, claimEvidence)
	}
	for key, candidate := range value.Digests {
		node.AddFieldEvidence(model.FieldDigest, key, candidate, claimEvidence)
	}
	for key, candidate := range value.Properties {
		node.AddFieldEvidence(model.FieldProperty, key, candidate, claimEvidence)
	}
	mergeEvidence(&node.Evidence, observation)
	node.ResolveFieldEvidence()
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
	model.MergeEvidenceSummary(summary, observationEvidence(observation))
	if len(summary.TraceIDs) > 20 {
		summary.TraceIDs = summary.TraceIDs[:20]
	}
}

func observationEvidence(observation inputpkg.Observation) model.EvidenceSummary {
	summary := model.EvidenceSummary{
		Level:            observation.Level,
		ObservationCount: 1,
		FirstSeen:        observation.Timestamp,
		LastSeen:         observation.Timestamp,
	}
	if observation.Source != "" {
		summary.Sources = []string{observation.Source}
	}
	if observation.TraceID != "" {
		summary.TraceIDs = []string{observation.TraceID}
	}
	return summary
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

func truth(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func spanEntityName(spanName, operation string) string {
	spanName = strings.TrimSpace(spanName)
	prefix := operation + " "
	if strings.HasPrefix(spanName, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(spanName, prefix))
	}
	return ""
}

// SortVersions is exported for tests and adapters that merge graphs.
func SortVersions(values []string) []string {
	sort.Strings(values)
	return values
}
