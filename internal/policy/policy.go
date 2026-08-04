package policy

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

type Policy struct {
	Version             string                         `json:"version"`
	MinimumEvidence     map[string]model.EvidenceLevel `json:"minimumEvidence,omitempty"`
	AllowedProviders    []string                       `json:"allowedProviders,omitempty"`
	RequireProvidersFor []string                       `json:"requireProvidersFor,omitempty"`
	DeniedNamePatterns  []string                       `json:"deniedNamePatterns,omitempty"`
	RequireVersionsFor  []string                       `json:"requireVersionsFor,omitempty"`
	ForbidInferred      bool                           `json:"forbidInferred,omitempty"`
}

type Report struct {
	GeneratedAt time.Time   `json:"generatedAt"`
	Passed      bool        `json:"passed"`
	Violations  []Violation `json:"violations,omitempty"`
}

type Violation struct {
	Rule     string `json:"rule"`
	NodeID   string `json:"nodeId,omitempty"`
	NodeType string `json:"nodeType,omitempty"`
	NodeName string `json:"nodeName,omitempty"`
	Message  string `json:"message"`
}

func Evaluate(graph model.Graph, policy Policy, generatedAt time.Time) (Report, error) {
	report := Report{GeneratedAt: generatedAt.UTC(), Passed: true}
	allowedProviders := stringSet(policy.AllowedProviders)
	requiredProviders := stringSet(policy.RequireProvidersFor)
	requiredVersions := stringSet(policy.RequireVersionsFor)
	patterns := make([]*regexp.Regexp, 0, len(policy.DeniedNamePatterns))
	for _, raw := range policy.DeniedNamePatterns {
		pattern, err := regexp.Compile(raw)
		if err != nil {
			return Report{}, fmt.Errorf("compile denied name pattern %q: %w", raw, err)
		}
		patterns = append(patterns, pattern)
	}
	for nodeType, level := range policy.MinimumEvidence {
		if !level.Valid() {
			return Report{}, fmt.Errorf("minimum evidence for %q is invalid: %q", nodeType, level)
		}
	}

	for _, node := range graph.Nodes {
		if policy.ForbidInferred && node.Evidence.Level == model.EvidenceInferred {
			report.add(node, "forbid-inferred", "inferred evidence is not allowed")
		}
		if minimum, ok := policy.MinimumEvidence[node.Type]; ok && node.Evidence.Level.Rank() < minimum.Rank() {
			report.add(node, "minimum-evidence",
				fmt.Sprintf("requires %s evidence or stronger; found %s", minimum, node.Evidence.Level))
		}
		if len(allowedProviders) > 0 && node.Provider != "" {
			if _, ok := allowedProviders[strings.ToLower(node.Provider)]; !ok {
				report.add(node, "allowed-providers", fmt.Sprintf("provider %q is not allowed", node.Provider))
			}
		}
		if _, ok := requiredProviders[strings.ToLower(node.Type)]; ok && strings.TrimSpace(node.Provider) == "" {
			report.add(node, "require-provider", "component provider is required")
		}
		if _, ok := requiredVersions[strings.ToLower(node.Type)]; ok && strings.TrimSpace(node.Version) == "" && len(node.ObservedVersions) == 0 {
			report.add(node, "require-version", "component version is required")
		}
		for index, pattern := range patterns {
			if pattern.MatchString(node.Name) {
				report.add(node, fmt.Sprintf("denied-name-pattern[%d]", index),
					fmt.Sprintf("name matches denied pattern %q", policy.DeniedNamePatterns[index]))
			}
		}
	}
	report.Passed = len(report.Violations) == 0
	sort.Slice(report.Violations, func(i, j int) bool {
		if report.Violations[i].NodeID == report.Violations[j].NodeID {
			return report.Violations[i].Rule < report.Violations[j].Rule
		}
		return report.Violations[i].NodeID < report.Violations[j].NodeID
	})
	return report, nil
}

func (r *Report) add(node model.Node, rule, message string) {
	r.Violations = append(r.Violations, Violation{
		Rule: rule, NodeID: node.ID, NodeType: node.Type, NodeName: node.Name, Message: message,
	})
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	return out
}
