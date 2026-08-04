package model

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = "0.1.0"

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
	Evidence         EvidenceSummary   `json:"evidence"`
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
		g.Nodes[i].Evidence.Sources = uniqueSorted(g.Nodes[i].Evidence.Sources)
		g.Nodes[i].Evidence.TraceIDs = uniqueSorted(g.Nodes[i].Evidence.TraceIDs)
	}
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].ID < g.Edges[j].ID })
	for i := range g.Edges {
		g.Edges[i].Evidence.Sources = uniqueSorted(g.Edges[i].Evidence.Sources)
		g.Edges[i].Evidence.TraceIDs = uniqueSorted(g.Edges[i].Evidence.TraceIDs)
	}
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
