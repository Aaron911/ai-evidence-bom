package signing

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSignAndVerify(t *testing.T) {
	privatePEM, publicPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("evidence")
	envelope, err := Sign(payload, privatePEM, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(payload, envelope, publicPEM); err != nil {
		t.Fatal(err)
	}
	if err := Verify([]byte("tampered"), envelope, publicPEM); err == nil {
		t.Fatal("tampered payload unexpectedly verified")
	}
}

func TestCanonicalEvidenceIsReproducibleAndFormattingIndependent(t *testing.T) {
	privatePEM, publicPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	first := []byte(`{
  "schemaVersion": "0.7.0",
  "generatedAt": "2026-08-07T08:00:00+08:00",
  "source": "otlp",
  "nodes": [
    {
      "id": "tool:b",
      "type": "tool",
      "name": "weather.lookup",
      "observedVersions": ["2", "1", "2"],
      "properties": {"z": "last", "a": "first"},
      "evidence": {
        "level": "observed",
        "sources": ["runtime", "otel", "runtime"],
        "firstSeen": "2026-08-07T08:00:00+08:00",
        "lastSeen": "2026-08-07T08:01:00+08:00",
        "observationCount": 2,
        "traceIds": ["trace-b", "trace-a", "trace-b"]
      }
    },
    {
      "id": "agent:a",
      "type": "agent",
      "name": "demo-agent",
      "evidence": {"level": "observed", "observationCount": 1}
    }
  ],
  "edges": [{
    "id": "edge:1",
    "from": "agent:a",
    "to": "tool:b",
    "relation": "invokes",
    "evidence": {
      "level": "observed",
      "sources": ["runtime", "otel"],
      "observationCount": 1,
      "traceIds": ["trace-b", "trace-a"]
    }
  }],
  "metadata": {"z": "last", "a": "first"}
}`)
	second := []byte(`{"metadata":{"a":"first","z":"last"},"edges":[{"relation":"invokes","to":"tool:b","from":"agent:a","id":"edge:1","evidence":{"traceIds":["trace-a","trace-b"],"observationCount":1,"sources":["otel","runtime"],"level":"observed"}}],"nodes":[{"name":"demo-agent","type":"agent","id":"agent:a","evidence":{"observationCount":1,"level":"observed"}},{"properties":{"a":"first","z":"last"},"observedVersions":["1","2"],"name":"weather.lookup","type":"tool","id":"tool:b","evidence":{"traceIds":["trace-a","trace-b"],"observationCount":2,"lastSeen":"2026-08-07T00:01:00Z","firstSeen":"2026-08-07T00:00:00Z","sources":["otel","runtime"],"level":"observed"}}],"source":"otlp","generatedAt":"2026-08-07T00:00:00Z","schemaVersion":"0.7.0"}`)

	firstCanonical, err := CanonicalizeEvidence(first)
	if err != nil {
		t.Fatal(err)
	}
	secondCanonical, err := CanonicalizeEvidence(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstCanonical, secondCanonical) {
		t.Fatalf("equivalent evidence produced different canonical bytes:\n%s\n%s", firstCanonical, secondCanonical)
	}

	firstEnvelope, err := SignCanonicalEvidence(first, privatePEM, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	secondEnvelope, err := SignCanonicalEvidence(second, privatePEM, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if firstEnvelope.Version != canonicalEnvelopeVersionV2 || firstEnvelope.PayloadType != CanonicalPayloadType || firstEnvelope.Canonicalization != CanonicalizationEvidenceV2 {
		t.Fatalf("canonical metadata missing from envelope: %+v", firstEnvelope)
	}
	if firstEnvelope.PayloadDigest != secondEnvelope.PayloadDigest {
		t.Fatalf("canonical digest changed: %s != %s", firstEnvelope.PayloadDigest, secondEnvelope.PayloadDigest)
	}
	if firstEnvelope.Signature != secondEnvelope.Signature {
		t.Fatal("deterministic Ed25519 signature changed for the same canonical evidence and key")
	}
	if err := Verify(second, firstEnvelope, publicPEM); err != nil {
		t.Fatalf("equivalent transport serialization did not verify: %v", err)
	}

	changed := []byte(strings.Replace(string(second), `"observationCount":2`, `"observationCount":3`, 1))
	if err := Verify(changed, firstEnvelope, publicPEM); err == nil || !strings.Contains(err.Error(), "payload digest mismatch") {
		t.Fatalf("one-field evidence change returned %v, want payload digest mismatch", err)
	}
}

func TestCanonicalEvidenceV2NormalizesAndSignsFieldEvidence(t *testing.T) {
	first := []byte(`{"schemaVersion":"0.8.0","generatedAt":"2026-08-07T00:00:00Z","nodes":[{"id":"model:1","type":"model","name":"model","version":"v1","observedVersions":["v1","v2"],"fieldEvidence":[{"field":"version","selectedValue":"v1","conflict":true,"values":[{"value":"v2","evidence":{"level":"declared","lastSeen":"2026-08-07T09:00:00+08:00","observationCount":1}},{"value":"v1","evidence":{"level":"verified","lastSeen":"2026-08-07T08:00:00+08:00","observationCount":1}}]}],"evidence":{"level":"verified","observationCount":2}}],"edges":[]}`)
	second := []byte(`{"edges":[],"nodes":[{"evidence":{"observationCount":2,"level":"verified"},"fieldEvidence":[{"conflict":true,"values":[{"evidence":{"observationCount":1,"lastSeen":"2026-08-07T01:00:00Z","level":"declared"},"value":"v2"},{"evidence":{"lastSeen":"2026-08-07T00:00:00Z","level":"verified","observationCount":1},"value":"v1"}],"selectedValue":"v1","field":"version"}],"version":"v1","name":"model","type":"model","id":"model:1","observedVersions":["v2","v1"]}],"generatedAt":"2026-08-07T00:00:00Z","schemaVersion":"0.8.0"}`)
	firstCanonical, err := CanonicalizeEvidence(first)
	if err != nil {
		t.Fatal(err)
	}
	secondCanonical, err := CanonicalizeEvidence(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstCanonical, secondCanonical) {
		t.Fatalf("equivalent field evidence produced different canonical bytes:\n%s\n%s", firstCanonical, secondCanonical)
	}
	if _, err := canonicalizeEvidence(first, false); err == nil || !strings.Contains(err.Error(), "v1 profile") {
		t.Fatalf("v1 profile accepted v0.8 field evidence: %v", err)
	}
}

func TestCanonicalEvidenceV1EnvelopeRemainsVerifiable(t *testing.T) {
	privatePEM, publicPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"schemaVersion":"0.7.0","generatedAt":"2026-08-07T00:00:00Z","nodes":[],"edges":[]}`)
	canonical, err := canonicalizeEvidence(payload, false)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := sign(canonical, canonicalSignatureMessageV1(canonical), privatePEM, time.Unix(1, 0), Envelope{
		Version: canonicalEnvelopeVersionV1, PayloadType: CanonicalPayloadType, Canonicalization: CanonicalizationEvidenceV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(payload, envelope, publicPEM); err != nil {
		t.Fatalf("legacy canonical envelope failed verification: %v", err)
	}
}

func TestCanonicalEvidenceRejectsAmbiguousOrUnknownJSON(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "duplicate member",
			payload: `{"schemaVersion":"0.7.0","schemaVersion":"0.7.0","generatedAt":"2026-08-07T00:00:00Z","nodes":[],"edges":[]}`,
		},
		{
			name:    "unknown member",
			payload: `{"schemaVersion":"0.7.0","generatedAt":"2026-08-07T00:00:00Z","nodes":[],"edges":[],"unexpected":true}`,
		},
		{
			name:    "multiple values",
			payload: `{"schemaVersion":"0.7.0","generatedAt":"2026-08-07T00:00:00Z","nodes":[],"edges":[]} {}`,
		},
		{
			name:    "duplicate node identity",
			payload: `{"schemaVersion":"0.7.0","generatedAt":"2026-08-07T00:00:00Z","nodes":[{"id":"node:1","type":"tool","name":"one","evidence":{"level":"declared","observationCount":1}},{"id":"node:1","type":"tool","name":"two","evidence":{"level":"declared","observationCount":1}}],"edges":[]}`,
		},
		{
			name:    "inconsistent selected field evidence",
			payload: `{"schemaVersion":"0.8.0","generatedAt":"2026-08-07T00:00:00Z","nodes":[{"id":"model:1","type":"model","name":"model","version":"v2","observedVersions":["v1","v2"],"fieldEvidence":[{"field":"version","selectedValue":"v2","conflict":true,"values":[{"value":"v1","evidence":{"level":"verified","observationCount":1}},{"value":"v2","evidence":{"level":"declared","observationCount":1}}]}],"evidence":{"level":"verified","observationCount":2}}],"edges":[]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := CanonicalizeEvidence([]byte(test.payload)); err == nil {
				t.Fatal("invalid canonical evidence unexpectedly accepted")
			}
		})
	}
}

func TestRawSignatureRemainsByteExactAndBackwardCompatible(t *testing.T) {
	privatePEM, publicPEM, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("{\"evidence\":true}\n")
	envelope, err := Sign(payload, privatePEM, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Version != rawEnvelopeVersion || envelope.PayloadType != "" || envelope.Canonicalization != "" {
		t.Fatalf("raw envelope compatibility changed: %+v", envelope)
	}
	if err := Verify(payload, envelope, publicPEM); err != nil {
		t.Fatal(err)
	}
	if err := Verify([]byte("{ \"evidence\": true }\n"), envelope, publicPEM); err == nil {
		t.Fatal("reformatted raw payload unexpectedly verified")
	}
}
