package cyclonedx

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

type BOM struct {
	BOMFormat    string       `json:"bomFormat"`
	SpecVersion  string       `json:"specVersion"`
	SerialNumber string       `json:"serialNumber"`
	Version      int          `json:"version"`
	Metadata     Metadata     `json:"metadata"`
	Components   []Component  `json:"components,omitempty"`
	Dependencies []Dependency `json:"dependencies,omitempty"`
}

type Metadata struct {
	Timestamp time.Time  `json:"timestamp"`
	Tools     Tools      `json:"tools"`
	Component *Component `json:"component,omitempty"`
}

type Tools struct {
	Components []Component `json:"components,omitempty"`
}

type Component struct {
	Type       string     `json:"type"`
	BOMRef     string     `json:"bom-ref,omitempty"`
	Name       string     `json:"name"`
	Version    string     `json:"version,omitempty"`
	Publisher  string     `json:"publisher,omitempty"`
	Hashes     []Hash     `json:"hashes,omitempty"`
	Properties []Property `json:"properties,omitempty"`
}

type Hash struct {
	Algorithm string `json:"alg"`
	Content   string `json:"content"`
}

type Property struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Dependency struct {
	Ref       string   `json:"ref"`
	DependsOn []string `json:"dependsOn"`
}

func Export(graph model.Graph) BOM {
	serialSeed := graph.Source + "\x00" + graph.GeneratedAt.UTC().Format(time.RFC3339Nano)
	serialHash := sha256.Sum256([]byte(serialSeed))
	bom := BOM{
		BOMFormat:   "CycloneDX",
		SpecVersion: "1.7",
		SerialNumber: fmt.Sprintf("urn:uuid:%s-%s-%s-%s-%s",
			hex.EncodeToString(serialHash[0:4]),
			hex.EncodeToString(serialHash[4:6]),
			hex.EncodeToString(serialHash[6:8]),
			hex.EncodeToString(serialHash[8:10]),
			hex.EncodeToString(serialHash[10:16])),
		Version: 1,
		Metadata: Metadata{
			Timestamp: graph.GeneratedAt,
			Tools: Tools{Components: []Component{{
				Type:      "application",
				Name:      "aiebom",
				Version:   model.SchemaVersion,
				Publisher: "AI Evidence BOM contributors",
			}}},
		},
	}
	for _, node := range graph.Nodes {
		component := Component{
			Type:      componentType(node.Type),
			BOMRef:    node.ID,
			Name:      node.Name,
			Version:   node.Version,
			Publisher: node.Provider,
			Properties: []Property{
				{Name: "aibom:node:type", Value: node.Type},
				{Name: "aibom:evidence:level", Value: string(node.Evidence.Level)},
				{Name: "aibom:evidence:observation-count", Value: strconv.Itoa(node.Evidence.ObservationCount)},
			},
		}
		if !node.Evidence.FirstSeen.IsZero() {
			component.Properties = append(component.Properties,
				Property{Name: "aibom:evidence:first-seen", Value: node.Evidence.FirstSeen.UTC().Format(time.RFC3339Nano)})
		}
		if !node.Evidence.LastSeen.IsZero() {
			component.Properties = append(component.Properties,
				Property{Name: "aibom:evidence:last-seen", Value: node.Evidence.LastSeen.UTC().Format(time.RFC3339Nano)})
		}
		for algorithm, content := range node.Digests {
			if algorithm == "hmac-sha256" {
				component.Properties = append(component.Properties,
					Property{Name: "aibom:fingerprint:hmac-sha256", Value: content})
				continue
			}
			component.Hashes = append(component.Hashes, Hash{Algorithm: hashAlgorithm(algorithm), Content: content})
		}
		for key, value := range node.Properties {
			component.Properties = append(component.Properties, Property{Name: "aibom:attribute:" + key, Value: value})
		}
		for _, version := range node.ObservedVersions {
			component.Properties = append(component.Properties, Property{Name: "aibom:observed-version", Value: version})
		}
		sort.Slice(component.Hashes, func(i, j int) bool { return component.Hashes[i].Algorithm < component.Hashes[j].Algorithm })
		sort.Slice(component.Properties, func(i, j int) bool {
			if component.Properties[i].Name == component.Properties[j].Name {
				return component.Properties[i].Value < component.Properties[j].Value
			}
			return component.Properties[i].Name < component.Properties[j].Name
		})
		bom.Components = append(bom.Components, component)
	}

	dependencies := make(map[string]map[string]struct{})
	for _, node := range graph.Nodes {
		dependencies[node.ID] = make(map[string]struct{})
	}
	for _, edge := range graph.Edges {
		if _, ok := dependencies[edge.From]; !ok {
			dependencies[edge.From] = make(map[string]struct{})
		}
		dependencies[edge.From][edge.To] = struct{}{}
	}
	for ref, targets := range dependencies {
		dependency := Dependency{Ref: ref, DependsOn: make([]string, 0, len(targets))}
		for target := range targets {
			dependency.DependsOn = append(dependency.DependsOn, target)
		}
		sort.Strings(dependency.DependsOn)
		bom.Dependencies = append(bom.Dependencies, dependency)
	}
	sort.Slice(bom.Dependencies, func(i, j int) bool { return bom.Dependencies[i].Ref < bom.Dependencies[j].Ref })
	return bom
}

func componentType(nodeType string) string {
	switch nodeType {
	case "model":
		return "machine-learning-model"
	case "data_source", "prompt":
		return "data"
	case "software":
		return "library"
	default:
		return "application"
	}
}

func hashAlgorithm(value string) string {
	switch value {
	case "sha256", "SHA-256":
		return "SHA-256"
	case "sha512", "SHA-512":
		return "SHA-512"
	default:
		return value
	}
}
