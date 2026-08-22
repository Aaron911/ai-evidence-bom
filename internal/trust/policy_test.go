package trust_test

import (
	"reflect"
	"testing"
	"time"

	inputpkg "github.com/Aaron911/ai-evidence-bom/internal/input"
	"github.com/Aaron911/ai-evidence-bom/internal/model"
	"github.com/Aaron911/ai-evidence-bom/internal/normalize"
	"github.com/Aaron911/ai-evidence-bom/internal/trust"
)

func TestDefaultPolicyCapsSelfReportedVerifiedEvidence(t *testing.T) {
	original := []inputpkg.Observation{{
		Level: model.EvidenceVerified, Source: "malicious-adapter",
	}}
	result := (trust.Policy{}).Apply(original)
	if result.Downgraded != 1 || result.Observations[0].Level != model.EvidenceObserved {
		t.Fatalf("untrusted verified claim was not capped: %+v", result)
	}
	if original[0].Level != model.EvidenceVerified {
		t.Fatal("source observation was mutated")
	}
}

func TestPolicyUsesExactSourceAndCanApplyStricterCaps(t *testing.T) {
	policy := mustParsePolicy(t, `{
	  "version":"0.1.0",
	  "sources":[
	    {"source":"model-signing-verifier","maxEvidence":"verified"},
	    {"source":"deployment-config","maxEvidence":"declared"}
	  ]
	}`)
	result := policy.Apply([]inputpkg.Observation{
		{Level: model.EvidenceVerified, Source: "model-signing-verifier"},
		{Level: model.EvidenceVerified, Source: "Model-signing-verifier"},
		{Level: model.EvidenceObserved, Source: "deployment-config"},
	})
	want := []model.EvidenceLevel{model.EvidenceVerified, model.EvidenceObserved, model.EvidenceDeclared}
	for index, level := range want {
		if result.Observations[index].Level != level {
			t.Fatalf("observation %d level=%q want=%q", index, result.Observations[index].Level, level)
		}
	}
	if result.Downgraded != 2 {
		t.Fatalf("downgraded=%d want=2", result.Downgraded)
	}
}

func TestCappedEvidenceMergeIsIndependentOfArrivalOrder(t *testing.T) {
	policy := mustParsePolicy(t, `{
	  "version":"0.1.0",
	  "sources":[{"source":"model-signing-verifier","maxEvidence":"verified"}]
	}`)
	verifiedAt := time.Date(2026, 8, 22, 8, 0, 0, 0, time.UTC)
	maliciousAt := verifiedAt.Add(time.Hour)
	trusted := modelObservation(verifiedAt, "model-signing-verifier", "weights-v1")
	malicious := modelObservation(maliciousAt, "malicious-adapter", "weights-v2")

	forward := policy.Apply([]inputpkg.Observation{trusted, malicious})
	reverse := policy.Apply([]inputpkg.Observation{malicious, trusted})
	forwardGraph := normalize.Build(forward.Observations, "trust-test", maliciousAt)
	reverseGraph := normalize.Build(reverse.Observations, "trust-test", maliciousAt)
	if !reflect.DeepEqual(forwardGraph.Nodes, reverseGraph.Nodes) {
		t.Fatalf("arrival order changed capped evidence:\nforward=%+v\nreverse=%+v", forwardGraph.Nodes, reverseGraph.Nodes)
	}
	if len(forwardGraph.Nodes) != 1 || forwardGraph.Nodes[0].Version != "weights-v1" {
		t.Fatalf("trusted version was not selected: %+v", forwardGraph.Nodes)
	}
	claim := forwardGraph.Nodes[0].FieldEvidence[0]
	if !claim.Conflict || len(claim.Values) != 2 {
		t.Fatalf("conflict was not retained: %+v", claim)
	}
	levels := map[string]model.EvidenceLevel{}
	for _, candidate := range claim.Values {
		levels[candidate.Value] = candidate.Evidence.Level
	}
	if levels["weights-v1"] != model.EvidenceVerified || levels["weights-v2"] != model.EvidenceObserved {
		t.Fatalf("unexpected capped candidates: %+v", claim.Values)
	}
}

func TestParseRejectsInvalidPolicies(t *testing.T) {
	tests := []string{
		`{"sources":[]}`,
		`{"version":"9","sources":[]}`,
		`{"version":"0.1.0","unknown":true}`,
		`{"version":"0.1.0","sources":[{"source":"","maxEvidence":"verified"}]}`,
		`{"version":"0.1.0","sources":[{"source":"a","maxEvidence":"invalid"}]}`,
		`{"version":"0.1.0","sources":[{"source":"a","maxEvidence":"observed"},{"source":"a","maxEvidence":"verified"}]}`,
		`{"version":"0.1.0"} {"version":"0.1.0"}`,
	}
	for _, input := range tests {
		if _, err := trust.Parse([]byte(input)); err == nil {
			t.Fatalf("invalid policy was accepted: %s", input)
		}
	}
}

func TestPolicyRecapsPersistedGraphAndResolvesFields(t *testing.T) {
	policy := mustParsePolicy(t, `{
	  "version":"0.1.0",
	  "sources":[{"source":"trusted-verifier","maxEvidence":"verified"}]
	}`)
	maliciousEvidence := model.EvidenceSummary{
		Level: model.EvidenceVerified, Sources: []string{"malicious-adapter"}, ObservationCount: 1,
	}
	trustedEvidence := model.EvidenceSummary{
		Level: model.EvidenceVerified, Sources: []string{"trusted-verifier"}, ObservationCount: 1,
	}
	node := model.Node{
		ID: "model:1", Type: "model", Name: "stable-model", Version: "weights-v2",
		Evidence: maliciousEvidence,
	}
	node.AddFieldEvidence(model.FieldVersion, "", "weights-v1", trustedEvidence)
	node.AddFieldEvidence(model.FieldVersion, "", "weights-v2", maliciousEvidence)
	node.ResolveFieldEvidence()
	graph := model.Graph{
		Nodes: []model.Node{node},
		Edges: []model.Edge{{
			ID: "edge:1", From: "agent:1", To: "model:1", Relation: "uses", Evidence: maliciousEvidence,
		}},
	}
	downgraded := policy.CapGraphInPlace(&graph)
	if downgraded != 3 {
		t.Fatalf("downgraded summaries=%d want=3", downgraded)
	}
	if graph.Nodes[0].Evidence.Level != model.EvidenceObserved || graph.Edges[0].Evidence.Level != model.EvidenceObserved {
		t.Fatalf("persisted aggregate evidence was not capped: %+v %+v", graph.Nodes[0].Evidence, graph.Edges[0].Evidence)
	}
	if graph.Nodes[0].Version != "weights-v1" {
		t.Fatalf("trusted persisted field was not reselected: %+v", graph.Nodes[0].FieldEvidence)
	}
	levels := map[string]model.EvidenceLevel{}
	for _, candidate := range graph.Nodes[0].FieldEvidence[0].Values {
		levels[candidate.Value] = candidate.Evidence.Level
	}
	if levels["weights-v1"] != model.EvidenceVerified || levels["weights-v2"] != model.EvidenceObserved {
		t.Fatalf("unexpected persisted field levels: %+v", graph.Nodes[0].FieldEvidence)
	}
}

func mustParsePolicy(t *testing.T, input string) trust.Policy {
	t.Helper()
	policy, err := trust.Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func modelObservation(timestamp time.Time, source, version string) inputpkg.Observation {
	return inputpkg.Observation{
		Timestamp: timestamp, Level: model.EvidenceVerified, Source: source,
		Attributes: map[string]string{
			"gen_ai.provider.name":  "example-provider",
			"gen_ai.request.model":  "stable-alias",
			"gen_ai.response.model": version,
		},
	}
}
