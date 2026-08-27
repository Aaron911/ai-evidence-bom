package model

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = "0.10.0"

const (
	FieldVersion  = "version"
	FieldDigest   = "digest"
	FieldProperty = "property"
)

type EvidenceLevel string

const (
	EvidenceInferred EvidenceLevel = "inferred"
	EvidenceDeclared EvidenceLevel = "declared"
	EvidenceObserved EvidenceLevel = "observed"
	EvidenceVerified EvidenceLevel = "verified"
)

func (l EvidenceLevel) Rank() int {
	switch l {
	case EvidenceVerified:
		return 4
	case EvidenceObserved:
		return 3
	case EvidenceDeclared:
		return 2
	case EvidenceInferred:
		return 1
	default:
		return 0
	}
}

func (l EvidenceLevel) Valid() bool {
	return l == EvidenceInferred || l == EvidenceDeclared || l == EvidenceObserved || l == EvidenceVerified
}

func StrongerLevel(a, b EvidenceLevel) EvidenceLevel {
	if b.Rank() > a.Rank() {
		return b
	}
	return a
}

type Graph struct {
	SchemaVersion string            `json:"schemaVersion"`
	GeneratedAt   time.Time         `json:"generatedAt"`
	Source        string            `json:"source,omitempty"`
	Nodes         []Node            `json:"nodes"`
	Edges         []Edge            `json:"edges"`
	Metadata      map[string]string `json:"metadata,omitempty"`
}

type Node struct {
	ID               string            `json:"id"`
	Type             string            `json:"type"`
	Name             string            `json:"name"`
	Version          string            `json:"version,omitempty"`
	ObservedVersions []string          `json:"observedVersions,omitempty"`
	Provider         string            `json:"provider,omitempty"`
	Digests          map[string]string `json:"digests,omitempty"`
	Properties       map[string]string `json:"properties,omitempty"`
	FieldEvidence    []FieldEvidence   `json:"fieldEvidence,omitempty"`
	Evidence         EvidenceSummary   `json:"evidence"`
}

// FieldEvidence retains every distinct value observed for a mutable node
// field. SelectedValue is resolved deterministically from evidence strength,
// recency, and finally lexical order; Conflict is true when values disagree.
type FieldEvidence struct {
	Field         string               `json:"field"`
	Key           string               `json:"key,omitempty"`
	SelectedValue string               `json:"selectedValue"`
	Conflict      bool                 `json:"conflict,omitempty"`
	Values        []FieldValueEvidence `json:"values"`
}

type FieldValueEvidence struct {
	Value    string          `json:"value"`
	Evidence EvidenceSummary `json:"evidence"`
}

type Edge struct {
	ID       string          `json:"id"`
	From     string          `json:"from"`
	To       string          `json:"to"`
	Relation string          `json:"relation"`
	Evidence EvidenceSummary `json:"evidence"`
}

type EvidenceSummary struct {
	Level            EvidenceLevel `json:"level"`
	Sources          []string      `json:"sources,omitempty"`
	FirstSeen        time.Time     `json:"firstSeen,omitempty"`
	LastSeen         time.Time     `json:"lastSeen,omitempty"`
	ObservationCount int           `json:"observationCount"`
	TraceIDs         []string      `json:"traceIds,omitempty"`
}

func StableNodeID(nodeType, provider, name string) string {
	identity := strings.ToLower(strings.TrimSpace(nodeType)) + "\x00" +
		strings.ToLower(strings.TrimSpace(provider)) + "\x00" +
		strings.ToLower(strings.TrimSpace(name))
	sum := sha256.Sum256([]byte(identity))
	return nodeType + ":" + hex.EncodeToString(sum[:12])
}

func StableEdgeID(from, relation, to string) string {
	sum := sha256.Sum256([]byte(from + "\x00" + relation + "\x00" + to))
	return "edge:" + hex.EncodeToString(sum[:12])
}

func (g *Graph) Canonicalize() {
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	for i := range g.Nodes {
		g.Nodes[i].ObservedVersions = uniqueSorted(g.Nodes[i].ObservedVersions)
		canonicalizeEvidence(&g.Nodes[i].Evidence)
		g.Nodes[i].ResolveFieldEvidence()
	}
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].ID < g.Edges[j].ID })
	for i := range g.Edges {
		canonicalizeEvidence(&g.Edges[i].Evidence)
	}
}

// AddFieldEvidence merges one value claim into a node without selecting by
// arrival order. Call ResolveFieldEvidence after all claims have been added.
func (n *Node) AddFieldEvidence(field, key, value string, evidence EvidenceSummary) {
	field = strings.TrimSpace(field)
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if field == "" || value == "" {
		return
	}
	for fieldIndex := range n.FieldEvidence {
		claim := &n.FieldEvidence[fieldIndex]
		if claim.Field != field || claim.Key != key {
			continue
		}
		for valueIndex := range claim.Values {
			if claim.Values[valueIndex].Value == value {
				MergeEvidenceSummary(&claim.Values[valueIndex].Evidence, evidence)
				return
			}
		}
		claim.Values = append(claim.Values, FieldValueEvidence{Value: value, Evidence: cloneEvidence(evidence)})
		return
	}
	n.FieldEvidence = append(n.FieldEvidence, FieldEvidence{
		Field: field,
		Key:   key,
		Values: []FieldValueEvidence{{
			Value: value, Evidence: cloneEvidence(evidence),
		}},
	})
}

// EnsureFieldEvidence upgrades a legacy node in memory. Because a node-level
// summary cannot prove which observation supplied a field, legacy values are
// conservatively treated as inferred rather than inheriting a stronger level.
func (n *Node) EnsureFieldEvidence() {
	legacyEvidence := n.legacyFieldEvidence()
	if n.Version != "" && !n.hasFieldValue(FieldVersion, "", n.Version) {
		n.AddFieldEvidence(FieldVersion, "", n.Version, legacyEvidence)
	}
	historicEvidence := legacyEvidence
	historicEvidence.FirstSeen = time.Time{}
	historicEvidence.LastSeen = time.Time{}
	for _, version := range n.ObservedVersions {
		if version != "" && !n.hasFieldValue(FieldVersion, "", version) {
			n.AddFieldEvidence(FieldVersion, "", version, historicEvidence)
		}
	}
	for key, value := range n.Digests {
		if !n.hasFieldValue(FieldDigest, key, value) {
			n.AddFieldEvidence(FieldDigest, key, value, legacyEvidence)
		}
	}
	for key, value := range n.Properties {
		if !n.hasFieldValue(FieldProperty, key, value) {
			n.AddFieldEvidence(FieldProperty, key, value, legacyEvidence)
		}
	}
}

// ResolveFieldEvidence canonicalizes claims and copies each selected value to
// the legacy top-level field used by exporters and existing consumers.
func (n *Node) ResolveFieldEvidence() {
	if len(n.FieldEvidence) == 0 {
		return
	}
	type fieldKey struct{ field, key string }
	grouped := make(map[fieldKey]map[string]EvidenceSummary)
	for _, claim := range n.FieldEvidence {
		key := fieldKey{field: strings.TrimSpace(claim.Field), key: strings.TrimSpace(claim.Key)}
		if key.field == "" {
			continue
		}
		if grouped[key] == nil {
			grouped[key] = make(map[string]EvidenceSummary)
		}
		for _, candidate := range claim.Values {
			value := strings.TrimSpace(candidate.Value)
			if value == "" {
				continue
			}
			summary := grouped[key][value]
			MergeEvidenceSummary(&summary, candidate.Evidence)
			grouped[key][value] = summary
		}
	}

	claims := make([]FieldEvidence, 0, len(grouped))
	for key, values := range grouped {
		claim := FieldEvidence{Field: key.field, Key: key.key}
		for value, evidence := range values {
			canonicalizeEvidence(&evidence)
			claim.Values = append(claim.Values, FieldValueEvidence{Value: value, Evidence: evidence})
		}
		sort.Slice(claim.Values, func(i, j int) bool { return claim.Values[i].Value < claim.Values[j].Value })
		if len(claim.Values) == 0 {
			continue
		}
		selected := claim.Values[0]
		for _, candidate := range claim.Values[1:] {
			if fieldValuePreferred(candidate, selected) {
				selected = candidate
			}
		}
		claim.SelectedValue = selected.Value
		claim.Conflict = len(claim.Values) > 1
		claims = append(claims, claim)
	}
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].Field == claims[j].Field {
			return claims[i].Key < claims[j].Key
		}
		return claims[i].Field < claims[j].Field
	})
	n.FieldEvidence = claims

	for _, claim := range claims {
		switch claim.Field {
		case FieldVersion:
			if claim.Key == "" {
				n.Version = claim.SelectedValue
				n.ObservedVersions = append(n.ObservedVersions, valuesOf(claim.Values)...)
			}
		case FieldDigest:
			if n.Digests == nil {
				n.Digests = make(map[string]string)
			}
			n.Digests[claim.Key] = claim.SelectedValue
		case FieldProperty:
			if n.Properties == nil {
				n.Properties = make(map[string]string)
			}
			n.Properties[claim.Key] = claim.SelectedValue
		}
	}
	n.ObservedVersions = uniqueSorted(n.ObservedVersions)
}

// MergeEvidenceSummary combines summaries using associative and commutative
// operations. Observation counts are cumulative rather than deduplicated here.
func MergeEvidenceSummary(current *EvidenceSummary, incoming EvidenceSummary) {
	current.Level = StrongerLevel(current.Level, incoming.Level)
	current.ObservationCount += incoming.ObservationCount
	current.Sources = append(current.Sources, incoming.Sources...)
	current.TraceIDs = append(current.TraceIDs, incoming.TraceIDs...)
	if current.FirstSeen.IsZero() || (!incoming.FirstSeen.IsZero() && incoming.FirstSeen.Before(current.FirstSeen)) {
		current.FirstSeen = incoming.FirstSeen
	}
	if incoming.LastSeen.After(current.LastSeen) {
		current.LastSeen = incoming.LastSeen
	}
}

func (n *Node) hasFieldValue(field, key, value string) bool {
	for _, claim := range n.FieldEvidence {
		if claim.Field != field || claim.Key != key {
			continue
		}
		for _, candidate := range claim.Values {
			if candidate.Value == value {
				return true
			}
		}
	}
	return false
}

func (n *Node) legacyFieldEvidence() EvidenceSummary {
	evidence := cloneEvidence(n.Evidence)
	evidence.Level = EvidenceInferred
	evidence.ObservationCount = 1
	return evidence
}

func fieldValuePreferred(candidate, selected FieldValueEvidence) bool {
	if candidate.Evidence.Level.Rank() != selected.Evidence.Level.Rank() {
		return candidate.Evidence.Level.Rank() > selected.Evidence.Level.Rank()
	}
	if !candidate.Evidence.LastSeen.Equal(selected.Evidence.LastSeen) {
		return candidate.Evidence.LastSeen.After(selected.Evidence.LastSeen)
	}
	return candidate.Value < selected.Value
}

func valuesOf(values []FieldValueEvidence) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value.Value)
	}
	return out
}

func cloneEvidence(evidence EvidenceSummary) EvidenceSummary {
	result := evidence
	result.Sources = append([]string(nil), evidence.Sources...)
	result.TraceIDs = append([]string(nil), evidence.TraceIDs...)
	return result
}

func canonicalizeEvidence(evidence *EvidenceSummary) {
	evidence.Sources = uniqueSorted(evidence.Sources)
	evidence.TraceIDs = uniqueSorted(evidence.TraceIDs)
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
