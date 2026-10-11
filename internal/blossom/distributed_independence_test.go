package blossom

import (
	"context"
	"strings"
	"testing"
)

func TestDistributedUploadRejectsSamePublicHostOnDifferentPorts(t *testing.T) {
	client := NewClient([]string{
		"https://same-relay.example:443",
		"https://same-relay.example:8443",
	}, nil, "test-agent")
	_, err := client.UploadDistributedShards(context.Background(), [][]byte{[]byte("ABSHfake")}, []string{strings.Repeat("a", 64)})
	if err == nil || !strings.Contains(err.Error(), "independent") {
		t.Fatalf("two ports of one public relay are not independent storage hosts: %v", err)
	}
}
