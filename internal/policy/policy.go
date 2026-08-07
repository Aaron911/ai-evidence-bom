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
	Version              string                         `json:"version"`
	MinimumEvidence      map[string]model.EvidenceLevel `json:"minimumEvidence,omitempty"`
	AllowedProviders     []string                       `json:"allowedProviders,omitempty"`
	RequireProvidersFor  []string                       `json:"requireProvidersFor,omitempty"`
	DeniedNamePatterns   []string                       `json:"deniedNamePatterns,omitempty"`
	RequireVersionsFor   []string                       `json:"requireVersionsFor,omitempty"`
	ForbidInferred       bool                           `json:"forbidInferred,omitempty"`
	ForbidFieldConflicts bool                           `json:"forbidFieldConflicts,omitempty"`
	DeniedPaths          []PathRule                     `json:"deniedPaths,omitempty"`
}

// PathRule denies a directed graph path with an exact relationship sequence.
// Via selectors, when present, correspond to the intermediate nodes between
// From and To.
type PathRule struct {
	Name      string         `json:"name,omitempty"`
	From      NodeSelector   `json:"from"`
	Via       []NodeSelector `json:"via,omitempty"`
	Relations []string       `json:"relations"`
	To        NodeSelector   `json:"to"`
}

type NodeSelector struct {
	Type            string              `json:"type,omitempty"`
	NamePattern     string              `json:"namePattern,omitempty"`
	Provider        string              `json:"provider,omitempty"`
	MinimumEvidence model.EvidenceLevel `json:"minimumEvidence,omitempty"`
}

type Report struct {
	GeneratedAt time.Time   `json:"generatedAt"`
	Passed      bool        `json:"passed"`
	Violations  []Violation `json:"violations,omitempty"`
}

type Violation struct {
	Rule          string   `json:"rule"`
	NodeID        string   `json:"nodeId,omitempty"`
	NodeType      string   `json:"nodeType,omitempty"`
	NodeName      string   `json:"nodeName,omitempty"`
	PathNodeIDs   []string `json:"pathNodeIds,omitempty"`
	PathRelations []string `json:"pathRelations,omitempty"`
	Message       string   `json:"message"`
}

const maxPathRelations = 8

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
	pathRules := make([]compiledPathRule, 0, len(policy.DeniedPaths))
	for index, rule := range policy.DeniedPaths {
		compiled, err := compilePathRule(rule, index)
		if err != nil {
			return Report{}, err
		}
		pathRules = append(pathRules, compiled)
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
		if policy.ForbidFieldConflicts {
			for _, claim := range node.FieldEvidence {
				if claim.Conflict || len(claim.Values) > 1 {
					field := claim.Field
					if claim.Key != "" {
						field += ":" + claim.Key
					}
					report.add(node, "forbid-field-conflict", fmt.Sprintf("field %q has %d competing values", field, len(claim.Values)))
				}
			}
		}
		for index, pattern := range patterns {
			if pattern.MatchString(node.Name) {
				report.add(node, fmt.Sprintf("denied-name-pattern[%d]", index),
					fmt.Sprintf("name matches denied pattern %q", policy.DeniedNamePatterns[index]))
			}
		}
	}
	for _, rule := range pathRules {
		for _, match := range rule.matches(graph) {
			report.addPath(match.start, rule, match.nodeIDs)
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

type compiledSelector struct {
	nodeType        string
	namePattern     *regexp.Regexp
	provider        string
	minimumEvidence model.EvidenceLevel
}

type compiledPathRule struct {
	name      string
	from      compiledSelector
	via       []compiledSelector
	relations []string
	to        compiledSelector
}

type pathMatch struct {
	start   model.Node
	nodeIDs []string
}

func compilePathRule(rule PathRule, index int) (compiledPathRule, error) {
	name := strings.TrimSpace(rule.Name)
	if name == "" {
		name = fmt.Sprintf("%d", index)
	}
	if len(rule.Relations) == 0 || len(rule.Relations) > maxPathRelations {
		return compiledPathRule{}, fmt.Errorf("denied path %q must contain 1-%d relations", name, maxPathRelations)
	}
	if len(rule.Via) != 0 && len(rule.Via) != len(rule.Relations)-1 {
		return compiledPathRule{}, fmt.Errorf("denied path %q has %d via selectors for %d relations", name, len(rule.Via), len(rule.Relations))
	}
	compiled := compiledPathRule{name: name, relations: make([]string, len(rule.Relations))}
	for relationIndex, relation := range rule.Relations {
		relation = strings.TrimSpace(relation)
		if relation == "" {
			return compiledPathRule{}, fmt.Errorf("denied path %q contains an empty relation", name)
		}
		compiled.relations[relationIndex] = relation
	}
	var err error
	if compiled.from, err = compileSelector(rule.From, "from", name); err != nil {
		return compiledPathRule{}, err
	}
	if compiled.to, err = compileSelector(rule.To, "to", name); err != nil {
		return compiledPathRule{}, err
	}
	compiled.via = make([]compiledSelector, len(rule.Via))
	for viaIndex, selector := range rule.Via {
		compiled.via[viaIndex], err = compileSelector(selector, fmt.Sprintf("via[%d]", viaIndex), name)
		if err != nil {
			return compiledPathRule{}, err
		}
	}
	return compiled, nil
}

func compileSelector(selector NodeSelector, position, ruleName string) (compiledSelector, error) {
	compiled := compiledSelector{
		nodeType:        strings.ToLower(strings.TrimSpace(selector.Type)),
		provider:        strings.ToLower(strings.TrimSpace(selector.Provider)),
		minimumEvidence: selector.MinimumEvidence,
	}
	if selector.MinimumEvidence != "" && !selector.MinimumEvidence.Valid() {
		return compiledSelector{}, fmt.Errorf("denied path %q %s selector has invalid minimum evidence %q", ruleName, position, selector.MinimumEvidence)
	}
	if selector.NamePattern != "" {
		pattern, err := regexp.Compile(selector.NamePattern)
		if err != nil {
			return compiledSelector{}, fmt.Errorf("compile denied path %q %s name pattern %q: %w", ruleName, position, selector.NamePattern, err)
		}
		compiled.namePattern = pattern
	}
	return compiled, nil
}

func (selector compiledSelector) matches(node model.Node) bool {
	if selector.nodeType != "" && strings.ToLower(node.Type) != selector.nodeType {
		return false
	}
	if selector.provider != "" && strings.ToLower(node.Provider) != selector.provider {
		return false
	}
	if selector.namePattern != nil && !selector.namePattern.MatchString(node.Name) {
		return false
	}
	return selector.minimumEvidence == "" || node.Evidence.Level.Rank() >= selector.minimumEvidence.Rank()
}

func (rule compiledPathRule) matches(graph model.Graph) []pathMatch {
	nodes := make(map[string]model.Node, len(graph.Nodes))
	starts := make([]model.Node, 0)
	for _, node := range graph.Nodes {
		nodes[node.ID] = node
		if rule.from.matches(node) {
			starts = append(starts, node)
		}
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i].ID < starts[j].ID })
	adjacency := make(map[string][]model.Edge)
	for _, edge := range graph.Edges {
		adjacency[edge.From] = append(adjacency[edge.From], edge)
	}
	for from := range adjacency {
		sort.Slice(adjacency[from], func(i, j int) bool {
			if adjacency[from][i].Relation == adjacency[from][j].Relation {
				return adjacency[from][i].To < adjacency[from][j].To
			}
			return adjacency[from][i].Relation < adjacency[from][j].Relation
		})
	}

	result := make([]pathMatch, 0)
	for _, start := range starts {
		if path, found := rule.findPath(start.ID, nodes, adjacency, 0, []string{start.ID}); found {
			result = append(result, pathMatch{start: start, nodeIDs: path})
		}
	}
	return result
}

func (rule compiledPathRule) findPath(current string, nodes map[string]model.Node, adjacency map[string][]model.Edge, relationIndex int, path []string) ([]string, bool) {
	if relationIndex == len(rule.relations) {
		node, ok := nodes[current]
		return path, ok && rule.to.matches(node)
	}
	for _, edge := range adjacency[current] {
		if edge.Relation != rule.relations[relationIndex] || contains(path, edge.To) {
			continue
		}
		next, ok := nodes[edge.To]
		if !ok {
			continue
		}
		if relationIndex < len(rule.relations)-1 && len(rule.via) > 0 && !rule.via[relationIndex].matches(next) {
			continue
		}
		candidate := append(append([]string(nil), path...), edge.To)
		if found, ok := rule.findPath(edge.To, nodes, adjacency, relationIndex+1, candidate); ok {
			return found, true
		}
	}
	return nil, false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (r *Report) add(node model.Node, rule, message string) {
	r.Violations = append(r.Violations, Violation{
		Rule: rule, NodeID: node.ID, NodeType: node.Type, NodeName: node.Name, Message: message,
	})
}

func (r *Report) addPath(start model.Node, rule compiledPathRule, nodeIDs []string) {
	r.Violations = append(r.Violations, Violation{
		Rule:          "denied-path:" + rule.name,
		NodeID:        start.ID,
		NodeType:      start.Type,
		NodeName:      start.Name,
		PathNodeIDs:   append([]string(nil), nodeIDs...),
		PathRelations: append([]string(nil), rule.relations...),
		Message:       fmt.Sprintf("graph path matches denied rule %q", rule.name),
	})
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	return out
}
