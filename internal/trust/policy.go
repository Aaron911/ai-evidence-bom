package trust

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	inputpkg "github.com/Aaron911/ai-evidence-bom/internal/input"
	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

const (
	PolicyVersion          = "0.1.0"
	DefaultMaximumEvidence = model.EvidenceObserved
)

// Policy caps the evidence authority of exact source names. Sources without a
// matching rule cannot exceed observed evidence, so self-reported compact
// input cannot obtain verified authority by default.
type Policy struct {
	Version string       `json:"version"`
	Sources []SourceRule `json:"sources,omitempty"`
}

type SourceRule struct {
	Source      string              `json:"source"`
	MaxEvidence model.EvidenceLevel `json:"maxEvidence"`
}

type Result struct {
	Observations []inputpkg.Observation
	Downgraded   int
}

// Parse strictly decodes and validates a source trust policy.
func Parse(data []byte) (Policy, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, fmt.Errorf("decode source trust policy: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Policy{}, err
	}
	if policy.Version == "" {
		return Policy{}, errors.New("source trust policy version is required")
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

// Validate permits the zero-value policy as the secure default used when no
// policy file is configured. Non-empty policies must use the supported
// version, unique exact source names, and valid evidence levels.
func (policy Policy) Validate() error {
	if policy.Version == "" && len(policy.Sources) == 0 {
		return nil
	}
	if policy.Version != PolicyVersion {
		return fmt.Errorf("source trust policy version must be %q", PolicyVersion)
	}
	seen := make(map[string]struct{}, len(policy.Sources))
	for index, rule := range policy.Sources {
		source := strings.TrimSpace(rule.Source)
		if source == "" {
			return fmt.Errorf("source trust rule %d has an empty source", index)
		}
		if source != rule.Source {
			return fmt.Errorf("source trust rule %d source must not have surrounding whitespace", index)
		}
		if !rule.MaxEvidence.Valid() {
			return fmt.Errorf("source trust rule %q has invalid maxEvidence %q", source, rule.MaxEvidence)
		}
		if _, exists := seen[source]; exists {
			return fmt.Errorf("source trust policy has duplicate source %q", source)
		}
		seen[source] = struct{}{}
	}
	return nil
}

// Apply returns a shallow copy of the observations with evidence levels
// capped by exact source rules. Attribute maps are not modified.
func (policy Policy) Apply(observations []inputpkg.Observation) Result {
	result := Result{Observations: append([]inputpkg.Observation(nil), observations...)}
	for index := range result.Observations {
		observation := &result.Observations[index]
		maximum := policy.maximumFor(observation.Source)
		if observation.Level.Rank() > maximum.Rank() {
			observation.Level = maximum
			result.Downgraded++
		}
	}
	return result
}

// CapGraphInPlace reapplies the current source policy to persisted evidence.
// It is used when a live receiver loads a graph produced before source caps
// existed or under a different policy. The return value counts summaries whose
// effective level changed; it is not an observation count.
func (policy Policy) CapGraphInPlace(graph *model.Graph) int {
	if graph == nil {
		return 0
	}
	downgraded := 0
	for nodeIndex := range graph.Nodes {
		node := &graph.Nodes[nodeIndex]
		if policy.capSummary(&node.Evidence) {
			downgraded++
		}
		for fieldIndex := range node.FieldEvidence {
			for valueIndex := range node.FieldEvidence[fieldIndex].Values {
				if policy.capSummary(&node.FieldEvidence[fieldIndex].Values[valueIndex].Evidence) {
					downgraded++
				}
			}
		}
		node.ResolveFieldEvidence()
	}
	for edgeIndex := range graph.Edges {
		if policy.capSummary(&graph.Edges[edgeIndex].Evidence) {
			downgraded++
		}
	}
	graph.Canonicalize()
	return downgraded
}

func (policy Policy) maximumFor(source string) model.EvidenceLevel {
	for _, rule := range policy.Sources {
		if rule.Source == source {
			return rule.MaxEvidence
		}
	}
	return DefaultMaximumEvidence
}

func (policy Policy) capSummary(summary *model.EvidenceSummary) bool {
	maximum := model.EvidenceLevel("")
	for _, source := range summary.Sources {
		maximum = model.StrongerLevel(maximum, policy.maximumFor(source))
	}
	if !maximum.Valid() {
		maximum = DefaultMaximumEvidence
	}
	if summary.Level.Rank() <= maximum.Rank() {
		return false
	}
	summary.Level = maximum
	return true
}

func ensureEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode source trust policy trailing data: %w", err)
	}
	return errors.New("decode source trust policy: multiple JSON values")
}
