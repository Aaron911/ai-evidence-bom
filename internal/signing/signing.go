package signing

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"time"
	"unicode/utf8"

	"github.com/Aaron911/ai-evidence-bom/internal/model"
	jsoncanonicalizer "github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
)

const (
	rawEnvelopeVersion         = "0.1.0"
	canonicalEnvelopeVersion   = "0.2.0"
	CanonicalPayloadType       = "aiebom-evidence-graph"
	CanonicalizationEvidenceV1 = "aiebom-evidence-v1+jcs-rfc8785"
	canonicalSignatureDomain   = "AI Evidence BOM canonical signature\x00v0.2.0\x00aiebom-evidence-graph\x00aiebom-evidence-v1+jcs-rfc8785\x00"
)

type Envelope struct {
	Version              string    `json:"version"`
	Algorithm            string    `json:"algorithm"`
	PayloadType          string    `json:"payloadType,omitempty"`
	Canonicalization     string    `json:"canonicalization,omitempty"`
	CreatedAt            time.Time `json:"createdAt"`
	PayloadDigest        string    `json:"payloadDigest"`
	PublicKeyFingerprint string    `json:"publicKeyFingerprint"`
	Signature            string    `json:"signature"`
}

func GenerateKeyPair() (privatePEM, publicPEM []byte, err error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate ed25519 key: %w", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal private key: %w", err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal public key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}),
		pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), nil
}

func Sign(payload, privatePEM []byte, createdAt time.Time) (Envelope, error) {
	return sign(payload, payload, privatePEM, createdAt, Envelope{Version: rawEnvelopeVersion})
}

// SignCanonicalEvidence signs the canonical identity of an AI Evidence BOM
// graph. Transport whitespace, JSON object member order, and graph collection
// order do not affect this identity. The signature envelope declares the
// canonicalization profile so verification never silently guesses a mode.
func SignCanonicalEvidence(payload, privatePEM []byte, createdAt time.Time) (Envelope, error) {
	canonical, err := CanonicalizeEvidence(payload)
	if err != nil {
		return Envelope{}, err
	}
	return sign(canonical, canonicalSignatureMessage(canonical), privatePEM, createdAt, Envelope{
		Version:          canonicalEnvelopeVersion,
		PayloadType:      CanonicalPayloadType,
		Canonicalization: CanonicalizationEvidenceV1,
	})
}

func sign(payload, signatureMessage, privatePEM []byte, createdAt time.Time, envelope Envelope) (Envelope, error) {
	privateKey, err := parsePrivateKey(privatePEM)
	if err != nil {
		return Envelope{}, err
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	digest := sha256.Sum256(payload)
	publicDigest := sha256.Sum256(publicKey)
	envelope.Algorithm = "ed25519"
	envelope.CreatedAt = createdAt.UTC()
	envelope.PayloadDigest = "sha256:" + hex.EncodeToString(digest[:])
	envelope.PublicKeyFingerprint = "sha256:" + hex.EncodeToString(publicDigest[:])
	envelope.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, signatureMessage))
	return envelope, nil
}

func Verify(payload []byte, envelope Envelope, publicPEM []byte) error {
	if envelope.Algorithm != "ed25519" {
		return fmt.Errorf("unsupported signature algorithm %q", envelope.Algorithm)
	}
	verifiedPayload, signatureMessage, err := verificationPayload(payload, envelope)
	if err != nil {
		return err
	}
	publicKey, err := parsePublicKey(publicPEM)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(verifiedPayload)
	if envelope.PayloadDigest != "sha256:"+hex.EncodeToString(digest[:]) {
		return fmt.Errorf("payload digest mismatch")
	}
	publicDigest := sha256.Sum256(publicKey)
	if envelope.PublicKeyFingerprint != "sha256:"+hex.EncodeToString(publicDigest[:]) {
		return fmt.Errorf("public key fingerprint mismatch")
	}
	signature, err := base64.StdEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if !ed25519.Verify(publicKey, signatureMessage, signature) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func verificationPayload(payload []byte, envelope Envelope) ([]byte, []byte, error) {
	switch envelope.Version {
	case rawEnvelopeVersion:
		if envelope.PayloadType != "" || envelope.Canonicalization != "" {
			return nil, nil, fmt.Errorf("raw signature envelope must not declare canonical payload metadata")
		}
		return payload, payload, nil
	case canonicalEnvelopeVersion:
		if envelope.PayloadType != CanonicalPayloadType {
			return nil, nil, fmt.Errorf("unsupported canonical payload type %q", envelope.PayloadType)
		}
		if envelope.Canonicalization != CanonicalizationEvidenceV1 {
			return nil, nil, fmt.Errorf("unsupported canonicalization %q", envelope.Canonicalization)
		}
		canonical, err := CanonicalizeEvidence(payload)
		if err != nil {
			return nil, nil, err
		}
		return canonical, canonicalSignatureMessage(canonical), nil
	default:
		return nil, nil, fmt.Errorf("unsupported signature envelope version %q", envelope.Version)
	}
}

// CanonicalizeEvidence returns the canonical bytes used by the v1 evidence
// signing profile. It first validates the input as strict RFC 8785-compatible
// JSON, then applies the graph's set/order semantics, normalizes timestamps to
// UTC, and finally serializes with RFC 8785 JCS.
func CanonicalizeEvidence(payload []byte) ([]byte, error) {
	if !utf8.Valid(payload) {
		return nil, fmt.Errorf("canonicalize evidence: input is not valid UTF-8")
	}
	if _, err := jsoncanonicalizer.Transform(payload); err != nil {
		return nil, fmt.Errorf("canonicalize evidence: invalid JCS input: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var graph model.Graph
	if err := decoder.Decode(&graph); err != nil {
		return nil, fmt.Errorf("canonicalize evidence: decode evidence graph: %w", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	if err := validateGraphIdentity(graph); err != nil {
		return nil, err
	}

	graph.Canonicalize()
	normalizeGraphTimes(&graph)
	encoded, err := json.Marshal(graph)
	if err != nil {
		return nil, fmt.Errorf("canonicalize evidence: encode evidence graph: %w", err)
	}
	canonical, err := jsoncanonicalizer.Transform(encoded)
	if err != nil {
		return nil, fmt.Errorf("canonicalize evidence: apply RFC 8785: %w", err)
	}
	return canonical, nil
}

func validateGraphIdentity(graph model.Graph) error {
	nodeIDs := make(map[string]struct{}, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if node.ID == "" {
			return fmt.Errorf("canonicalize evidence: node ID must not be empty")
		}
		if _, exists := nodeIDs[node.ID]; exists {
			return fmt.Errorf("canonicalize evidence: duplicate node ID %q", node.ID)
		}
		nodeIDs[node.ID] = struct{}{}
	}
	edgeIDs := make(map[string]struct{}, len(graph.Edges))
	for _, edge := range graph.Edges {
		if edge.ID == "" {
			return fmt.Errorf("canonicalize evidence: edge ID must not be empty")
		}
		if _, exists := edgeIDs[edge.ID]; exists {
			return fmt.Errorf("canonicalize evidence: duplicate edge ID %q", edge.ID)
		}
		edgeIDs[edge.ID] = struct{}{}
	}
	return nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("canonicalize evidence: multiple JSON values")
		}
		return fmt.Errorf("canonicalize evidence: trailing JSON: %w", err)
	}
	return nil
}

func normalizeGraphTimes(graph *model.Graph) {
	graph.GeneratedAt = graph.GeneratedAt.UTC()
	for index := range graph.Nodes {
		graph.Nodes[index].Evidence.FirstSeen = graph.Nodes[index].Evidence.FirstSeen.UTC()
		graph.Nodes[index].Evidence.LastSeen = graph.Nodes[index].Evidence.LastSeen.UTC()
	}
	for index := range graph.Edges {
		graph.Edges[index].Evidence.FirstSeen = graph.Edges[index].Evidence.FirstSeen.UTC()
		graph.Edges[index].Evidence.LastSeen = graph.Edges[index].Evidence.LastSeen.UTC()
	}
}

func canonicalSignatureMessage(canonical []byte) []byte {
	message := make([]byte, 0, len(canonicalSignatureDomain)+len(canonical))
	message = append(message, canonicalSignatureDomain...)
	return append(message, canonical...)
}

func parsePrivateKey(data []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("decode private key PEM")
	}
	value, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	privateKey, ok := value.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ed25519")
	}
	return privateKey, nil
}

func parsePublicKey(data []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("decode public key PEM")
	}
	value, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	publicKey, ok := value.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("public key is not ed25519")
	}
	return publicKey, nil
}
