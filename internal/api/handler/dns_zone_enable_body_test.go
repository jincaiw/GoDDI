package handler

import (
	"strings"
	"testing"
)

func TestDecodeDNSSECEnableRequestAllowsEmptyBody(t *testing.T) {
	req, err := decodeDNSSECEnableRequest(strings.NewReader(""))
	if err != nil {
		t.Fatalf("empty body: %v", err)
	}
	if req.Algorithm != "" {
		t.Fatalf("algorithm = %q, want empty", req.Algorithm)
	}

	req, err = decodeDNSSECEnableRequest(strings.NewReader(`{"algorithm":"ED25519"}`))
	if err != nil {
		t.Fatalf("json body: %v", err)
	}
	if req.Algorithm != "ED25519" {
		t.Fatalf("algorithm = %q, want ED25519", req.Algorithm)
	}
}
