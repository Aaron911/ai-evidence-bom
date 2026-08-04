package signing

import (
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
