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
