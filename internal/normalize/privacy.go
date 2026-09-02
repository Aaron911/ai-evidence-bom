package normalize

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"

	inputpkg "github.com/Aaron911/ai-evidence-bom/internal/input"
)

const (
	promptContentPresentAttribute = "aiebom.prompt.content_present"
	promptHMACAttribute           = "aiebom.prompt.hmac_sha256"
)

var retainedObservationAttributes = map[string]struct{}{
	"aiebom.artifact.sha256":                {},
	"aiebom.artifact.uri":                   {},
	"aiebom.evidence.level":                 {},
	"aiebom.mcp.discovery.source":           {},
	"aiebom.mcp.server.id":                  {},
	"aiebom.mcp.server.identity_source":     {},
	"aiebom.mcp.server.name":                {},
	"aiebom.mcp.server.provider":            {},
	"aiebom.mcp.server.version":             {},
	"aiebom.mcp.tool.annotations_untrusted": {},
	"aiebom.mcp.tool.destructive":           {},
	"aiebom.mcp.tool.input_schema_sha256":   {},
	"aiebom.mcp.tool.read_only":             {},
	"aiebom.mcp.tool.version":               {},
	"agent.name":                            {},
	"data.source.name":                      {},
	"db.namespace":                          {},
	"db.system.name":                        {},
	"deployment.environment.name":           {},
	"dify.app_id":                           {},
	"dify.app_name":                         {},
	"dify.workflow_id":                      {},
	"gen_ai.agent.id":                       {},
	"gen_ai.agent.name":                     {},
	"gen_ai.agent.provider":                 {},
	"gen_ai.agent.version":                  {},
	"gen_ai.data_source.id":                 {},
	"gen_ai.framework":                      {},
	"gen_ai.model":                          {},
	"gen_ai.model.digest":                   {},
	"gen_ai.operation.name":                 {},
	"gen_ai.prompt.template.id":             {},
	"gen_ai.prompt.template.name":           {},
	"gen_ai.prompt.template.version":        {},
	"gen_ai.provider.name":                  {},
	"gen_ai.request.encoding_formats":       {},
	"gen_ai.request.model":                  {},
	"gen_ai.response.finish_reasons":        {},
	"gen_ai.response.model":                 {},
	"gen_ai.retrieval.data_source.id":       {},
	"gen_ai.span.kind":                      {},
	"gen_ai.system":                         {},
	"gen_ai.tool.call.name":                 {},
	"gen_ai.tool.name":                      {},
	"gen_ai.tool.type":                      {},
	"gen_ai.tool.version":                   {},
	"host.name":                             {},
	"llm.provider":                          {},
	"llm.request.model":                     {},
	"llm.response.model":                    {},
	"mcp.server.id":                         {},
	"mcp.server.name":                       {},
	"mcp.server.provider":                   {},
	"mcp.server.url":                        {},
	"mcp.server.version":                    {},
	"mcp.method.name":                       {},
	"mcp.protocol.version":                  {},
	"mcp.tool.destructive":                  {},
	"mcp.tool.name":                         {},
	"mcp.tool.read_only":                    {},
	"mcp.tool.version":                      {},
	"mcp.transport":                         {},
	"model.digest":                          {},
	"network.transport":                     {},
	"otel.scope.name":                       {},
	"otel.scope.schema_url":                 {},
	"otel.scope.version":                    {},
	"otel.span.name":                        {},
	"server.address":                        {},
	"server.port":                           {},
	"service.name":                          {},
	"service.namespace":                     {},
	"service.version":                       {},
	"tool.name":                             {},
}

// MetadataOnlyObservation produces the bounded, content-free representation
// retained while the live receiver waits for parent spans from another OTLP
// export batch. Prompt content is reduced to presence plus an optional HMAC.
func MetadataOnlyObservation(observation inputpkg.Observation, sensitiveHMACKey []byte) inputpkg.Observation {
	attributes := make(map[string]string, len(retainedObservationAttributes)+2)
	for key := range retainedObservationAttributes {
		if value := observation.Attributes[key]; value != "" {
			attributes[key] = value
		}
	}
	promptContent := first(observation.Attributes, "gen_ai.system_instructions", "gen_ai.prompt")
	if promptContent != "" {
		attributes[promptContentPresentAttribute] = "true"
		if len(sensitiveHMACKey) > 0 {
			mac := hmac.New(sha256.New, sensitiveHMACKey)
			_, _ = mac.Write([]byte(promptContent))
			attributes[promptHMACAttribute] = hex.EncodeToString(mac.Sum(nil))
		}
	}
	observation.Attributes = attributes
	return observation
}
