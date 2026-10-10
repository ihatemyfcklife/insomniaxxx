package app

import (
	"bytes"
	"testing"
)

func TestReadLimitedBlob(t *testing.T) {
	if got, err := readLimitedBlob(bytes.NewReader([]byte("1234")), 4); err != nil || string(got) != "1234" {
		t.Fatalf("read at limit = %q, %v; want 1234, nil", got, err)
	}
	if got, err := readLimitedBlob(bytes.NewReader([]byte("12345")), 4); err == nil || got != nil {
		t.Fatalf("oversized blob = %q, %v; want nil and an error", got, err)
	}
	if _, err := readLimitedBlob(bytes.NewReader(nil), 0); err == nil {
		t.Fatal("zero size limit should be rejected")
	}
}
