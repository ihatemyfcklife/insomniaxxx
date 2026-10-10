package blossom

import (
	"context"
	"testing"
)

func TestPublicRelayURLRejectsUnsafeDestinations(t *testing.T) {
	tests := []string{
		"http://127.0.0.1:8080",
		"http://10.0.0.1",
		"http://192.168.1.10",
		"http://169.254.169.254",
		"http://100.64.0.1",
		"https://192.0.2.1",
		"https://198.18.0.1",
		"https://[2001:db8::1]",
		"http://[64:ff9b::a00:1]", // NAT64 encoding of 10.0.0.1
		"http://[2002:0a00:0001::]", // 6to4 encoding of 10.0.0.1
		"http://[::1]",
		"http://localhost",
		"http://service.local",
		"file:///etc/passwd",
		"https://user:pass@example.com",
	}
	for _, raw := range tests {
		t.Run(raw, func(t *testing.T) {
			if err := publicRelayURL(context.Background(), raw); err == nil {
				t.Fatalf("expected unsafe relay URL to be rejected: %q", raw)
			}
		})
	}
}

func TestPublicRelayURLAcceptsPublicIP(t *testing.T) {
	if err := publicRelayURL(context.Background(), "https://8.8.8.8"); err != nil {
		t.Fatalf("expected public IP relay URL to pass: %v", err)
	}
}

func TestCleanBlossomURLRejectsLocalAndSpecialUseDestinations(t *testing.T) {
	for _, raw := range []string{
		"http://localhost",
		"http://service.local",
		"http://service.internal",
		"http://100.64.0.1",
		"https://[::1]",
		"https://[2001:db8::1]",
	} {
		t.Run(raw, func(t *testing.T) {
			if cleaned, ok := cleanBlossomURL(raw); ok {
				t.Fatalf("expected unsafe discovery URL %q to be rejected; got %q", raw, cleaned)
			}
		})
	}
}
