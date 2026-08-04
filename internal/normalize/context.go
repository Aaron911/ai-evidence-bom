package normalize

import (
	"strings"

	inputpkg "github.com/Aaron911/ai-evidence-bom/internal/input"
)

type traceAgentContext struct {
	id      string
	name    string
	version string
}

// prepareObservations uses OTLP parent links to carry stable agent identity to
// model and tool child spans. It also prevents an invoke_agent summary span
// from being mistaken for a second model call when a concrete model child span
// is present in the same trace.
func prepareObservations(input []inputpkg.Observation) []inputpkg.Observation {
	observations := make([]inputpkg.Observation, len(input))
	spanIndex := make(map[string]int, len(input))
	for index, observation := range input {
		observations[index] = observation
		observations[index].Attributes = clone(observation.Attributes)
		if observations[index].Attributes == nil {
			observations[index].Attributes = make(map[string]string)
		}
		if key := traceSpanKey(observation.TraceID, observation.SpanID); key != "" {
			spanIndex[key] = index
		}
	}

	resolved := make(map[int]traceAgentContext, len(observations))
	resolving := make(map[int]bool, len(observations))
	var resolveAgent func(int) traceAgentContext
	resolveAgent = func(index int) traceAgentContext {
		if context, ok := resolved[index]; ok {
			return context
		}
		if resolving[index] {
			return traceAgentContext{}
		}
		resolving[index] = true
		observation := &observations[index]
		context := directAgentContext(observation.Attributes)
		if context.id == "" && context.name == "" && observation.ParentSpanID != "" {
			if parentIndex, ok := spanIndex[traceSpanKey(observation.TraceID, observation.ParentSpanID)]; ok {
				context = resolveAgent(parentIndex)
			}
		}
		if context.id != "" || context.name != "" {
			if observation.Attributes["gen_ai.agent.id"] == "" {
				observation.Attributes["gen_ai.agent.id"] = context.id
			}
			if observation.Attributes["gen_ai.agent.name"] == "" {
				observation.Attributes["gen_ai.agent.name"] = context.name
			}
			if observation.Attributes["gen_ai.agent.version"] == "" && context.version != "" {
				observation.Attributes["gen_ai.agent.version"] = context.version
			}
		}
		delete(resolving, index)
		resolved[index] = context
		return context
	}
	for index := range observations {
		resolveAgent(index)
	}

	for index := range observations {
		if !isConcreteModelSpan(observations[index].Attributes) {
			continue
		}
		parentSpanID := observations[index].ParentSpanID
		visited := make(map[int]bool)
		for parentSpanID != "" {
			parentIndex, ok := spanIndex[traceSpanKey(observations[index].TraceID, parentSpanID)]
			if !ok || visited[parentIndex] {
				break
			}
			visited[parentIndex] = true
			parent := &observations[parentIndex]
			if strings.EqualFold(first(parent.Attributes, "gen_ai.operation.name"), "invoke_agent") {
				delete(parent.Attributes, "gen_ai.request.model")
				delete(parent.Attributes, "gen_ai.response.model")
				delete(parent.Attributes, "gen_ai.model")
				delete(parent.Attributes, "llm.request.model")
				delete(parent.Attributes, "llm.response.model")
				break
			}
			parentSpanID = parent.ParentSpanID
		}
	}
	return observations
}

func directAgentContext(attributes map[string]string) traceAgentContext {
	context := traceAgentContext{
		id:      first(attributes, "gen_ai.agent.id"),
		name:    first(attributes, "gen_ai.agent.name", "agent.name"),
		version: first(attributes, "gen_ai.agent.version"),
	}
	if difyAppID := first(attributes, "dify.app_id"); difyAppID != "" {
		if context.id == "" {
			context.id = difyAppID
		}
		if context.name == "" {
			context.name = firstNonEmpty(first(attributes, "dify.app_name"), difyAppID)
		}
	}
	if context.name == "" {
		context.name = context.id
	}
	if context.id == "" {
		context.id = context.name
	}
	return context
}

func isConcreteModelSpan(attributes map[string]string) bool {
	if strings.EqualFold(first(attributes, "gen_ai.span.kind"), "llm") {
		return true
	}
	operation := strings.ToLower(first(attributes, "gen_ai.operation.name"))
	switch operation {
	case "chat", "text_completion", "generate_content", "embeddings":
		return true
	default:
		return false
	}
}

func traceSpanKey(traceID, spanID string) string {
	traceID = strings.ToLower(strings.TrimSpace(traceID))
	spanID = strings.ToLower(strings.TrimSpace(spanID))
	if traceID == "" || spanID == "" {
		return ""
	}
	return traceID + "\x00" + spanID
}
