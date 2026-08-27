package sourceauth_test

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/Aaron911/ai-evidence-bom/internal/sourceauth"
)

func TestPolicyAuthenticatesExactSourceAndSupportsRotation(t *testing.T) {
	oldToken := "old-source-credential-with-at-least-32-bytes"
	newToken := "new-source-credential-with-at-least-32-bytes"
	policy := mustParsePolicy(t, fmt.Sprintf(`{
	  "version":"0.1.0",
	  "bindings":[
	    {"source":"model-signing-verifier","tokenSha256":"%s"},
	    {"source":"model-signing-verifier","tokenSha256":"%s"}
	  ]
	}`, tokenDigest(oldToken), tokenDigest(newToken)))

	for _, token := range []string{oldToken, newToken} {
		source, ok := policy.Authenticate(token)
		if !ok || source != "model-signing-verifier" {
			t.Fatalf("token was not bound to stable source: source=%q ok=%v", source, ok)
		}
	}
	if source, ok := policy.Authenticate("wrong-source-credential-with-at-least-32-bytes"); ok || source != "" {
		t.Fatalf("wrong token authenticated: source=%q ok=%v", source, ok)
	}
	if source, ok := policy.Authenticate("too-short"); ok || source != "" {
		t.Fatalf("short token authenticated: source=%q ok=%v", source, ok)
	}
	if !policy.Protects("model-signing-verifier") || policy.Protects("Model-signing-verifier") {
		t.Fatal("source protection was not exact and case-sensitive")
	}
}

func TestParseRejectsInvalidPolicies(t *testing.T) {
	validDigest := tokenDigest("valid-source-credential-with-at-least-32-bytes")
	tests := []string{
		`{"bindings":[]}`,
		`{"version":"9","bindings":[]}`,
		`{"version":"0.1.0","bindings":[]}`,
		`{"version":"0.1.0","unknown":true,"bindings":[]}`,
		fmt.Sprintf(`{"version":"0.1.0","bindings":[{"source":"","tokenSha256":"%s"}]}`, validDigest),
		fmt.Sprintf(`{"version":"0.1.0","bindings":[{"source":" padded ","tokenSha256":"%s"}]}`, validDigest),
		`{"version":"0.1.0","bindings":[{"source":"a","tokenSha256":"ABC"}]}`,
		fmt.Sprintf(`{"version":"0.1.0","bindings":[{"source":"a","tokenSha256":"%s"},{"source":"b","tokenSha256":"%s"}]}`, validDigest, validDigest),
		fmt.Sprintf(`{"version":"0.1.0","bindings":[{"source":"a","tokenSha256":"%s"}]} {}`, validDigest),
	}
	for _, input := range tests {
		if _, err := sourceauth.Parse([]byte(input)); err == nil {
			t.Fatalf("invalid policy was accepted: %s", input)
		}
	}
}

func mustParsePolicy(t *testing.T, input string) sourceauth.Policy {
	t.Helper()
	policy, err := sourceauth.Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func tokenDigest(token string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
}
