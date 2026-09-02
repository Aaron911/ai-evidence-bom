// Package sarif imports a deliberately small, metadata-only subset of SARIF
// 2.1.0 and binds scanner findings to an existing evidence-graph component.
package sarif

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Aaron911/ai-evidence-bom/internal/aggregate"
	"github.com/Aaron911/ai-evidence-bom/internal/model"
)

const (
	DefaultMaxSARIFBytes     int64 = 16 << 20
	DefaultMaxArtifactBytes  int64 = 16 << 20
	maxRuns                        = 32
	maxRulesPerRun                 = 20_000
	maxResultsPerRun               = 10_000
	maxArtifactsPerRun             = 20_000
	maxInvocationsPerRun           = 128
	maxLocationsPerResult          = 32
	maxSuppressionsPerResult       = 32
	maxScannerNameLength           = 200
	maxScannerVersionLength        = 200
	maxRuleIDLength                = 512
	maxArtifactURILength           = 2_048
)

const (
	PropertyArtifactURI    = "aiebom.artifact.uri"
	PropertyFindingFormat  = "aiebom.finding.format"
	PropertyFindingLevel   = "aiebom.finding.sarif.level"
	PropertyFindingState   = "aiebom.finding.assertion"
	PropertyScannerVersion = "aiebom.finding.scanner.version"
	PropertyTargetDigest   = "aiebom.finding.artifact.sha256"
	PropertyTargetURI      = "aiebom.finding.artifact.uri"
)

type ImportResult struct {
	Graph            model.Graph
	Attached         int
	SkippedResults   int
	ArtifactURI      string
	SARIFArtifactURI string
	ArtifactSHA256   string
}

// ArtifactIdentity derives a repository-relative URI and SHA-256 from a
// regular source artifact. Absolute paths and parent traversal are rejected so
// machine-specific paths cannot become component identity.
func ArtifactIdentity(path string, maxBytes int64) (string, string, error) {
	if maxBytes <= 0 {
		return "", "", errors.New("maximum artifact bytes must be positive")
	}
	if path == "" || path != strings.TrimSpace(path) {
		return "", "", errors.New("artifact path must be non-empty and must not have surrounding whitespace")
	}
	if filepath.IsAbs(path) {
		return "", "", errors.New("artifact path must be relative to the evidence root")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", "", errors.New("artifact path must not traverse outside the evidence root")
	}
	info, err := os.Lstat(clean)
	if err != nil {
		return "", "", fmt.Errorf("inspect artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", "", errors.New("artifact must be a regular file, not a symlink")
	}
	if info.Size() > maxBytes {
		return "", "", fmt.Errorf("artifact exceeds %d byte limit", maxBytes)
	}
	file, err := os.Open(clean)
	if err != nil {
		return "", "", fmt.Errorf("open artifact: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	written, err := io.Copy(hash, io.LimitReader(file, maxBytes+1))
	if err != nil {
		return "", "", fmt.Errorf("hash artifact: %w", err)
	}
	if written > maxBytes {
		return "", "", fmt.Errorf("artifact exceeds %d byte limit", maxBytes)
	}
	pathURI := filepath.ToSlash(clean)
	uri := (&url.URL{Path: pathURI}).EscapedPath()
	parsed, err := url.Parse(uri)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", errors.New("artifact path does not form a stable relative URI")
	}
	if len(uri) > maxArtifactURILength {
		return "", "", fmt.Errorf("artifact URI exceeds %d characters", maxArtifactURILength)
	}
	return uri, hex.EncodeToString(hash.Sum(nil)), nil
}

// Import attaches only unsuppressed failing results whose physical artifact
// URI equals sarifArtifactURI. The graph target must have artifactURI and the
// selected SHA-256 without a conflicting claim. Keeping the two URIs explicit
// avoids guessing when a scanner reports a path relative to its scan root.
func Import(graph model.Graph, data []byte, artifactURI, sarifArtifactURI, artifactSHA256 string, observedAt time.Time) (ImportResult, error) {
	target, err := findTarget(graph, artifactURI, artifactSHA256)
	if err != nil {
		return ImportResult{}, err
	}
	if err := validateRelativeArtifactURI("SARIF artifact URI", sarifArtifactURI); err != nil {
		return ImportResult{}, err
	}
	doc, err := parse(data)
	if err != nil {
		return ImportResult{}, err
	}

	nodes := make(map[string]*model.Node)
	edges := make(map[string]*model.Edge)
	skipped := 0
	for runIndex, run := range doc.Runs {
		findings, ignored, err := findingsForRun(run, sarifArtifactURI, observedAt)
		if err != nil {
			return ImportResult{}, fmt.Errorf("SARIF run %d: %w", runIndex, err)
		}
		skipped += ignored
		for _, finding := range findings {
			identity := artifactURI + "\x00" + artifactSHA256 + "\x00" + finding.RuleID
			nodeID := model.StableNodeID("finding", finding.Scanner, identity)
			node := nodes[nodeID]
			if node == nil {
				node = &model.Node{
					ID:         nodeID,
					Type:       "finding",
					Name:       finding.RuleID,
					Provider:   finding.Scanner,
					Digests:    make(map[string]string),
					Properties: make(map[string]string),
				}
				nodes[nodeID] = node
			}
			model.MergeEvidenceSummary(&node.Evidence, finding.Evidence)
			node.AddFieldEvidence(model.FieldProperty, PropertyFindingFormat, "sarif-2.1.0", finding.Evidence)
			node.AddFieldEvidence(model.FieldProperty, PropertyFindingLevel, finding.Level, finding.Evidence)
			node.AddFieldEvidence(model.FieldProperty, PropertyFindingState, "scanner-reported", finding.Evidence)
			node.AddFieldEvidence(model.FieldProperty, PropertyTargetURI, artifactURI, finding.Evidence)
			node.AddFieldEvidence(model.FieldProperty, PropertyTargetDigest, artifactSHA256, finding.Evidence)
			if finding.ScannerVersion != "" {
				node.AddFieldEvidence(model.FieldProperty, PropertyScannerVersion, finding.ScannerVersion, finding.Evidence)
			}
			node.ResolveFieldEvidence()

			edgeID := model.StableEdgeID(target.ID, "affected_by", nodeID)
			edge := edges[edgeID]
			if edge == nil {
				edge = &model.Edge{ID: edgeID, From: target.ID, To: nodeID, Relation: "affected_by"}
				edges[edgeID] = edge
			}
			model.MergeEvidenceSummary(&edge.Evidence, finding.Evidence)
		}
	}

	incoming := model.Graph{
		SchemaVersion: model.SchemaVersion,
		GeneratedAt:   observedAt.UTC(),
		Source:        graph.Source,
	}
	for _, node := range nodes {
		incoming.Nodes = append(incoming.Nodes, *node)
	}
	for _, edge := range edges {
		incoming.Edges = append(incoming.Edges, *edge)
	}
	incoming.Canonicalize()
	resultGraph := aggregate.Merge(graph, incoming, observedAt)
	return ImportResult{
		Graph:            resultGraph,
		Attached:         len(nodes),
		SkippedResults:   skipped,
		ArtifactURI:      artifactURI,
		SARIFArtifactURI: sarifArtifactURI,
		ArtifactSHA256:   artifactSHA256,
	}, nil
}

type document struct {
	Version string `json:"version"`
	Runs    []run  `json:"runs"`
}

type run struct {
	Tool        tool         `json:"tool"`
	Artifacts   []artifact   `json:"artifacts"`
	Invocations []invocation `json:"invocations"`
	Results     []result     `json:"results"`
}

type tool struct {
	Driver toolComponent `json:"driver"`
}

type toolComponent struct {
	Name            string                `json:"name"`
	Version         string                `json:"version"`
	SemanticVersion string                `json:"semanticVersion"`
	Rules           []reportingDescriptor `json:"rules"`
}

type reportingDescriptor struct {
	ID                   string        `json:"id"`
	DefaultConfiguration configuration `json:"defaultConfiguration"`
}

type configuration struct {
	Level string `json:"level"`
}

type artifact struct {
	Location artifactLocation `json:"location"`
}

type invocation struct {
	ExecutionSuccessful *bool `json:"executionSuccessful"`
}

type result struct {
	RuleID         string                        `json:"ruleId"`
	RuleIndex      *int                          `json:"ruleIndex"`
	Rule           *reportingDescriptorReference `json:"rule"`
	Kind           string                        `json:"kind"`
	Level          string                        `json:"level"`
	AnalysisTarget *artifactLocation             `json:"analysisTarget"`
	Locations      []location                    `json:"locations"`
	Suppressions   []suppression                 `json:"suppressions"`
}

type suppression struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type reportingDescriptorReference struct {
	ID    string `json:"id"`
	Index *int   `json:"index"`
}

type location struct {
	PhysicalLocation *physicalLocation `json:"physicalLocation"`
}

type physicalLocation struct {
	ArtifactLocation artifactLocation `json:"artifactLocation"`
}

type artifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
	Index     *int   `json:"index"`
}

type finding struct {
	RuleID         string
	Level          string
	Scanner        string
	ScannerVersion string
	Evidence       model.EvidenceSummary
}

func parse(data []byte) (document, error) {
	if int64(len(data)) > DefaultMaxSARIFBytes {
		return document{}, fmt.Errorf("SARIF exceeds %d byte limit", DefaultMaxSARIFBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var doc document
	if err := decoder.Decode(&doc); err != nil {
		return document{}, fmt.Errorf("decode SARIF: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return document{}, errors.New("decode SARIF: multiple JSON values")
		}
		return document{}, fmt.Errorf("decode SARIF trailing data: %w", err)
	}
	if doc.Version != "2.1.0" {
		return document{}, fmt.Errorf("SARIF version must be %q", "2.1.0")
	}
	if len(doc.Runs) == 0 || len(doc.Runs) > maxRuns {
		return document{}, fmt.Errorf("SARIF must contain 1-%d runs", maxRuns)
	}
	return doc, nil
}

func findingsForRun(run run, artifactURI string, observedAt time.Time) ([]finding, int, error) {
	driver := run.Tool.Driver
	if err := boundedValue("scanner name", driver.Name, maxScannerNameLength, true); err != nil {
		return nil, 0, err
	}
	version := firstNonEmpty(driver.SemanticVersion, driver.Version)
	if err := boundedValue("scanner version", version, maxScannerVersionLength, false); err != nil {
		return nil, 0, err
	}
	if len(driver.Rules) > maxRulesPerRun {
		return nil, 0, fmt.Errorf("rule count exceeds %d", maxRulesPerRun)
	}
	if len(run.Results) > maxResultsPerRun {
		return nil, 0, fmt.Errorf("result count exceeds %d", maxResultsPerRun)
	}
	if len(run.Artifacts) > maxArtifactsPerRun {
		return nil, 0, fmt.Errorf("artifact count exceeds %d", maxArtifactsPerRun)
	}
	if len(run.Invocations) > maxInvocationsPerRun {
		return nil, 0, fmt.Errorf("invocation count exceeds %d", maxInvocationsPerRun)
	}
	for index, invocation := range run.Invocations {
		if invocation.ExecutionSuccessful != nil && !*invocation.ExecutionSuccessful {
			return nil, 0, fmt.Errorf("invocation %d reports executionSuccessful=false", index)
		}
	}
	rules := make(map[string]reportingDescriptor, len(driver.Rules))
	for index, rule := range driver.Rules {
		if err := boundedValue(fmt.Sprintf("rule %d id", index), rule.ID, maxRuleIDLength, true); err != nil {
			return nil, 0, err
		}
		if _, exists := rules[rule.ID]; exists {
			return nil, 0, fmt.Errorf("duplicate rule id %q", rule.ID)
		}
		if rule.DefaultConfiguration.Level != "" && !validLevel(rule.DefaultConfiguration.Level) {
			return nil, 0, fmt.Errorf("rule %q has invalid default level %q", rule.ID, rule.DefaultConfiguration.Level)
		}
		rules[rule.ID] = rule
	}

	grouped := make(map[string]finding)
	skipped := 0
	for resultIndex, raw := range run.Results {
		if len(raw.Locations) > maxLocationsPerResult {
			return nil, 0, fmt.Errorf("result %d location count exceeds %d", resultIndex, maxLocationsPerResult)
		}
		if len(raw.Suppressions) > maxSuppressionsPerResult {
			return nil, 0, fmt.Errorf("result %d suppression count exceeds %d", resultIndex, maxSuppressionsPerResult)
		}
		isSuppressed, err := activeSuppression(raw.Suppressions)
		if err != nil {
			return nil, 0, fmt.Errorf("result %d: %w", resultIndex, err)
		}
		if isSuppressed {
			skipped++
			continue
		}
		kind := firstNonEmpty(raw.Kind, "fail")
		if !validKind(kind) {
			return nil, 0, fmt.Errorf("result %d has invalid kind %q", resultIndex, kind)
		}
		if kind != "fail" {
			skipped++
			continue
		}
		ruleID, err := resolveRuleID(raw, driver.Rules)
		if err != nil {
			return nil, 0, fmt.Errorf("result %d: %w", resultIndex, err)
		}
		if err := boundedValue(fmt.Sprintf("result %d ruleId", resultIndex), ruleID, maxRuleIDLength, true); err != nil {
			return nil, 0, err
		}
		level := raw.Level
		if level == "" {
			if descriptor, exists := rules[ruleID]; exists {
				level = descriptor.DefaultConfiguration.Level
			}
			if level == "" {
				level = "warning"
			}
		}
		if !validLevel(level) || level == "none" {
			return nil, 0, fmt.Errorf("result %d has invalid fail level %q", resultIndex, level)
		}
		matches, err := resultMatchesArtifact(raw, run.Artifacts, artifactURI)
		if err != nil {
			return nil, 0, fmt.Errorf("result %d: %w", resultIndex, err)
		}
		if !matches {
			skipped++
			continue
		}
		source := "sarif:" + driver.Name
		if version != "" {
			source += "@" + version
		}
		evidence := model.EvidenceSummary{
			Level:            model.EvidenceObserved,
			Sources:          []string{source},
			FirstSeen:        observedAt.UTC(),
			LastSeen:         observedAt.UTC(),
			ObservationCount: 1,
		}
		key := driver.Name + "\x00" + ruleID
		current := grouped[key]
		if current.RuleID == "" {
			current = finding{RuleID: ruleID, Level: level, Scanner: driver.Name, ScannerVersion: version}
		}
		if current.Level != level {
			return nil, 0, fmt.Errorf("rule %q has conflicting levels %q and %q in one run", ruleID, current.Level, level)
		}
		model.MergeEvidenceSummary(&current.Evidence, evidence)
		grouped[key] = current
	}
	findings := make([]finding, 0, len(grouped))
	for _, item := range grouped {
		findings = append(findings, item)
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Scanner == findings[j].Scanner {
			return findings[i].RuleID < findings[j].RuleID
		}
		return findings[i].Scanner < findings[j].Scanner
	})
	return findings, skipped, nil
}

func findTarget(graph model.Graph, artifactURI, digest string) (model.Node, error) {
	if err := validateRelativeArtifactURI("artifact URI", artifactURI); err != nil {
		return model.Node{}, err
	}
	if len(digest) != sha256.Size*2 || strings.ToLower(digest) != digest {
		return model.Node{}, errors.New("artifact SHA-256 must be 64 lowercase hexadecimal characters")
	}
	if _, err := hex.DecodeString(digest); err != nil {
		return model.Node{}, errors.New("artifact SHA-256 must be 64 lowercase hexadecimal characters")
	}
	uriMatches := 0
	exact := make([]model.Node, 0, 1)
	for _, node := range graph.Nodes {
		if node.Type == "finding" || node.Properties[PropertyArtifactURI] != artifactURI {
			continue
		}
		uriMatches++
		if !fieldEvidenceIsExact(node, model.FieldProperty, PropertyArtifactURI, artifactURI) ||
			!fieldEvidenceIsExact(node, model.FieldDigest, "sha256", digest) {
			continue
		}
		if node.Digests["sha256"] == digest {
			exact = append(exact, node)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		return model.Node{}, fmt.Errorf("artifact URI and digest match %d components; target is ambiguous", len(exact))
	}
	if uriMatches > 0 {
		return model.Node{}, errors.New("artifact URI matched but SHA-256 did not; finding was not attached")
	}
	return model.Node{}, errors.New("no component matches the artifact URI; finding was not attached")
}

func fieldEvidenceIsExact(node model.Node, field, key, selected string) bool {
	found := false
	for _, claim := range node.FieldEvidence {
		if claim.Field == field && claim.Key == key {
			if found || claim.Conflict || claim.SelectedValue != selected || len(claim.Values) != 1 || claim.Values[0].Value != selected {
				return false
			}
			found = true
		}
	}
	// Graphs produced before fieldEvidence was introduced remain importable;
	// when a claim exists, however, it must agree exactly with the top-level
	// selected binding instead of being silently ignored.
	return true
}

func resolveRuleID(result result, rules []reportingDescriptor) (string, error) {
	ruleID := result.RuleID
	if result.Rule != nil && result.Rule.ID != "" {
		if ruleID != "" && ruleID != result.Rule.ID {
			return "", errors.New("ruleId and rule.id disagree")
		}
		ruleID = result.Rule.ID
	}
	index := result.RuleIndex
	if result.Rule != nil && result.Rule.Index != nil {
		if index != nil && *index != *result.Rule.Index {
			return "", errors.New("ruleIndex and rule.index disagree")
		}
		index = result.Rule.Index
	}
	if index != nil {
		if *index < 0 || *index >= len(rules) {
			return "", errors.New("rule index is out of range")
		}
		indexedID := rules[*index].ID
		if ruleID != "" && ruleID != indexedID {
			return "", errors.New("rule identifier and index disagree")
		}
		ruleID = indexedID
	}
	if ruleID == "" {
		return "", errors.New("rule identifier is required")
	}
	return ruleID, nil
}

func resultMatchesArtifact(result result, artifacts []artifact, artifactURI string) (bool, error) {
	locations := make([]artifactLocation, 0, len(result.Locations)+1)
	if result.AnalysisTarget != nil {
		locations = append(locations, *result.AnalysisTarget)
	}
	for _, location := range result.Locations {
		if location.PhysicalLocation != nil {
			locations = append(locations, location.PhysicalLocation.ArtifactLocation)
		}
	}
	if len(locations) == 0 {
		return false, nil
	}
	matched := false
	for _, location := range locations {
		uri, err := resolveArtifactURI(location, artifacts)
		if err != nil {
			return false, err
		}
		if uri == "" {
			return false, nil
		}
		if uri != artifactURI {
			return false, nil
		}
		matched = true
	}
	return matched, nil
}

func activeSuppression(suppressions []suppression) (bool, error) {
	active := false
	for index, item := range suppressions {
		if item.Kind != "inSource" && item.Kind != "external" {
			return false, fmt.Errorf("suppression %d has invalid kind %q", index, item.Kind)
		}
		switch item.Status {
		case "", "accepted":
			active = true
		case "underReview", "rejected":
		default:
			return false, fmt.Errorf("suppression %d has invalid status %q", index, item.Status)
		}
	}
	return active, nil
}

func resolveArtifactURI(location artifactLocation, artifacts []artifact) (string, error) {
	if location.URIBaseID != "" {
		return "", errors.New("artifact uriBaseId is unsupported; provide a resolved relative URI")
	}
	uri := location.URI
	if location.Index != nil {
		if *location.Index < 0 || *location.Index >= len(artifacts) {
			return "", errors.New("artifact index is out of range")
		}
		indexedLocation := artifacts[*location.Index].Location
		if indexedLocation.URIBaseID != "" {
			return "", errors.New("indexed artifact uriBaseId is unsupported; provide a resolved relative URI")
		}
		indexedURI := indexedLocation.URI
		if uri != "" && indexedURI != "" && uri != indexedURI {
			return "", errors.New("artifact URI and index disagree")
		}
		if uri == "" {
			uri = indexedURI
		}
	}
	if uri != "" {
		if err := boundedValue("result artifact URI", uri, maxArtifactURILength, true); err != nil {
			return "", err
		}
	}
	return uri, nil
}

func validKind(value string) bool {
	switch value {
	case "pass", "open", "informational", "notApplicable", "review", "fail":
		return true
	default:
		return false
	}
}

func validLevel(value string) bool {
	switch value {
	case "none", "note", "warning", "error":
		return true
	default:
		return false
	}
}

func boundedValue(name, value string, limit int, required bool) error {
	if required && value == "" {
		return fmt.Errorf("%s is required", name)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not have surrounding whitespace", name)
	}
	if len(value) > limit {
		return fmt.Errorf("%s exceeds %d characters", name, limit)
	}
	return nil
}

func validateRelativeArtifactURI(name, value string) error {
	if err := boundedValue(name, value, maxArtifactURILength, true); err != nil {
		return err
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must be a stable relative URI", name)
	}
	if parsed.EscapedPath() != value {
		return fmt.Errorf("%s must use canonical URI path escaping", name)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(parsed.Path)))
	canonical := (&url.URL{Path: clean}).EscapedPath()
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || canonical != value {
		return fmt.Errorf("%s must be a clean relative URI", name)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
