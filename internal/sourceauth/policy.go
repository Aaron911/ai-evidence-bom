package sourceauth

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	PolicyVersion          = "0.1.0"
	MinimumCredentialBytes = 32
)

// Policy binds high-entropy bearer credential digests to exact evidence
// source names. Multiple bindings may name the same source so credentials can
// overlap during rotation without changing graph identity.
type Policy struct {
	Version  string    `json:"version"`
	Bindings []Binding `json:"bindings"`
}

type Binding struct {
	Source      string `json:"source"`
	TokenSHA256 string `json:"tokenSha256"`
}

// Parse strictly decodes and validates a source authentication policy.
func Parse(data []byte) (Policy, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, fmt.Errorf("decode source authentication policy: %w", err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Policy{}, err
	}
	if policy.Version == "" {
		return Policy{}, errors.New("source authentication policy version is required")
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

// Validate permits the zero-value policy when source authentication is not
// configured. A configured policy requires at least one unique digest.
func (policy Policy) Validate() error {
	if policy.Version == "" && len(policy.Bindings) == 0 {
		return nil
	}
	if policy.Version != PolicyVersion {
		return fmt.Errorf("source authentication policy version must be %q", PolicyVersion)
	}
	if len(policy.Bindings) == 0 {
		return errors.New("source authentication policy requires at least one binding")
	}
	seenDigests := make(map[string]struct{}, len(policy.Bindings))
	for index, binding := range policy.Bindings {
		source := strings.TrimSpace(binding.Source)
		if source == "" {
			return fmt.Errorf("source authentication binding %d has an empty source", index)
		}
		if source != binding.Source {
			return fmt.Errorf("source authentication binding %d source must not have surrounding whitespace", index)
		}
		if len(binding.TokenSHA256) != sha256.Size*2 || strings.ToLower(binding.TokenSHA256) != binding.TokenSHA256 {
			return fmt.Errorf("source authentication binding %d tokenSha256 must be 64 lowercase hexadecimal characters", index)
		}
		if _, err := hex.DecodeString(binding.TokenSHA256); err != nil {
			return fmt.Errorf("source authentication binding %d tokenSha256 must be 64 lowercase hexadecimal characters", index)
		}
		if _, exists := seenDigests[binding.TokenSHA256]; exists {
			return fmt.Errorf("source authentication policy has duplicate tokenSha256 in binding %d", index)
		}
		seenDigests[binding.TokenSHA256] = struct{}{}
	}
	return nil
}

func (policy Policy) Enabled() bool {
	return len(policy.Bindings) > 0
}

// Authenticate returns the exact source bound to a high-entropy token. It
// evaluates all configured digests and never returns or stores the raw token.
func (policy Policy) Authenticate(token string) (string, bool) {
	if len([]byte(token)) < MinimumCredentialBytes {
		return "", false
	}
	digest := sha256.Sum256([]byte(token))
	matchedSource := ""
	matched := 0
	for _, binding := range policy.Bindings {
		expected, err := hex.DecodeString(binding.TokenSHA256)
		if err != nil {
			continue
		}
		equal := subtle.ConstantTimeCompare(digest[:], expected)
		if equal == 1 {
			matchedSource = binding.Source
		}
		matched |= equal
	}
	return matchedSource, matched == 1
}

// Protects reports whether a source can only be asserted by a credential
// bound in this policy.
func (policy Policy) Protects(source string) bool {
	for _, binding := range policy.Bindings {
		if binding.Source == source {
			return true
		}
	}
	return false
}

func ensureEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode source authentication policy trailing data: %w", err)
	}
	return errors.New("decode source authentication policy: multiple JSON values")
}
