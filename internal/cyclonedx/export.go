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
	BOMFormat       string          `json:"bomFormat"`
	SpecVersion     string          `json:"specVersion"`
	SerialNumber    string          `json:"serialNumber"`
	Version         int             `json:"version"`
	Metadata        Metadata        `json:"metadata"`
	Components      []Component     `json:"components,omitempty"`
	Dependencies    []Dependency    `json:"dependencies,omitempty"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities,omitempty"`
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

type Vulnerability struct {
	BOMRef     string              `json:"bom-ref,omitempty"`
	ID         string              `json:"id,omitempty"`
	Source     VulnerabilitySource `json:"source,omitempty"`
	Affects    []Affect            `json:"affects,omitempty"`
	Properties []Property          `json:"properties,omitempty"`
}

type VulnerabilitySource struct {
	Name string `json:"name,omitempty"`
}

type Affect struct {
	Ref string `json:"ref"`
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
		if node.Type == "finding" {
			continue
		}
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
		component.Properties = appendFieldEvidenceProperties(component.Properties, node.FieldEvidence)
		sort.Slice(component.Hashes, func(i, j int) bool { return component.Hashes[i].Algorithm < component.Hashes[j].Algorithm })
		sort.Slice(component.Properties, func(i, j int) bool {
			if component.Properties[i].Name == component.Properties[j].Name {
				return component.Properties[i].Value < component.Properties[j].Value
			}
			return component.Properties[i].Name < component.Properties[j].Name
		})
		bom.Components = append(bom.Components, component)
	}

	componentIDs := make(map[string]struct{})
	dependencies := make(map[string]map[string]struct{})
	for _, node := range graph.Nodes {
		if node.Type == "finding" {
			continue
		}
		componentIDs[node.ID] = struct{}{}
		dependencies[node.ID] = make(map[string]struct{})
	}
	for _, edge := range graph.Edges {
		if _, fromIsComponent := componentIDs[edge.From]; !fromIsComponent {
			continue
		}
		if _, toIsComponent := componentIDs[edge.To]; !toIsComponent {
			continue
		}
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

	affected := make(map[string]map[string]struct{})
	for _, edge := range graph.Edges {
		if edge.Relation != "affected_by" {
			continue
		}
		if _, ok := componentIDs[edge.From]; !ok {
			continue
		}
		if affected[edge.To] == nil {
			affected[edge.To] = make(map[string]struct{})
		}
		affected[edge.To][edge.From] = struct{}{}
	}
	for _, node := range graph.Nodes {
		if node.Type != "finding" {
			continue
		}
		vulnerability := Vulnerability{
			BOMRef: node.ID,
			ID:     node.Name,
			Source: VulnerabilitySource{Name: node.Provider},
			Properties: []Property{
				{Name: "aibom:evidence:level", Value: string(node.Evidence.Level)},
				{Name: "aibom:evidence:observation-count", Value: strconv.Itoa(node.Evidence.ObservationCount)},
			},
		}
		for key, value := range node.Properties {
			propertyName := "aibom:attribute:" + key
			switch key {
			case "aiebom.finding.sarif.level":
				propertyName = "aibom:sarif:level"
			case "aiebom.finding.scanner.version":
				propertyName = "aibom:scanner:version"
			case "aiebom.finding.artifact.uri":
				propertyName = "aibom:artifact:uri"
			case "aiebom.finding.artifact.sha256":
				propertyName = "aibom:artifact:sha256"
			case "aiebom.finding.assertion":
				propertyName = "aibom:finding:assertion"
			case "aiebom.finding.format":
				propertyName = "aibom:finding:format"
			}
			vulnerability.Properties = append(vulnerability.Properties, Property{Name: propertyName, Value: value})
		}
		vulnerability.Properties = appendFieldEvidenceProperties(vulnerability.Properties, node.FieldEvidence)
		for ref := range affected[node.ID] {
			vulnerability.Affects = append(vulnerability.Affects, Affect{Ref: ref})
		}
		sort.Slice(vulnerability.Affects, func(i, j int) bool { return vulnerability.Affects[i].Ref < vulnerability.Affects[j].Ref })
		sort.Slice(vulnerability.Properties, func(i, j int) bool {
			if vulnerability.Properties[i].Name == vulnerability.Properties[j].Name {
				return vulnerability.Properties[i].Value < vulnerability.Properties[j].Value
			}
			return vulnerability.Properties[i].Name < vulnerability.Properties[j].Name
		})
		bom.Vulnerabilities = append(bom.Vulnerabilities, vulnerability)
	}
	sort.Slice(bom.Vulnerabilities, func(i, j int) bool { return bom.Vulnerabilities[i].BOMRef < bom.Vulnerabilities[j].BOMRef })
	return bom
}

func appendFieldEvidenceProperties(properties []Property, claims []model.FieldEvidence) []Property {
	for _, claim := range claims {
		fieldName := claim.Field
		if claim.Key != "" {
			fieldName += ":" + claim.Key
		}
		baseName := "aibom:field-evidence:" + fieldName
		properties = append(properties, Property{Name: baseName + ":selected", Value: claim.SelectedValue})
		for _, candidate := range claim.Values {
			properties = append(properties, Property{
				Name: baseName + ":candidate:" + string(candidate.Evidence.Level), Value: candidate.Value,
			})
		}
		if claim.Conflict || len(claim.Values) > 1 {
			properties = append(properties, Property{Name: "aibom:field-conflict", Value: fieldName})
		}
	}
	return properties
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
