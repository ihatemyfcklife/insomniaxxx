package cryptbox

import (
	"bytes"
	"testing"
)

func TestOuterHeaderRoundTrip(t *testing.T) {
	header := MakeOuterHeader(1 << 20)
	got, ok := ParseOuterHeader(header)
	if !ok || got != 1<<20 {
		t.Fatalf("outer header round-trip failed: chunk=%d ok=%v", got, ok)
	}

	header[0] = 'x'
	if _, ok := ParseOuterHeader(header); ok {
		t.Fatal("tampered outer header accepted")
	}
}

func TestManifestRoundTrip(t *testing.T) {
	want := FileMeta{Name: "résumé.pdf", Type: "application/pdf", Size: 12345}
	prefix, err := MakeManifestPrefix(want)
	if err != nil {
		t.Fatal(err)
	}

	plain := append(append([]byte{}, prefix...), []byte("payload")...)
	got, rest, err := ParseManifestPrefix(plain)
	if err != nil {
		t.Fatal(err)
	}
	if got != want || !bytes.Equal(rest, []byte("payload")) {
		t.Fatalf("manifest mismatch: %#v %q", got, rest)
	}
}

func TestDownloadPath(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	key := bytes.Repeat([]byte{0x01}, 32)

	gotHash, gotKey, ok := ParseDownloadPath("/d/" + hash + "/" + ShareKeyText(key))
	if !ok || gotHash != hash || !bytes.Equal(gotKey, key) {
		t.Fatalf("download path rejected: hash=%s key=%d ok=%v", gotHash, len(gotKey), ok)
	}
	if _, _, ok := ParseDownloadPath("/d/" + hash + "/not-a-key"); ok {
		t.Fatal("invalid key accepted")
	}
}

func TestCleanFilename(t *testing.T) {
	if got := CleanFilename("../../secret.txt"); got != "secret.txt" {
		t.Fatalf("unexpected filename: %q", got)
	}
	if got := CleanFilename("a\x00b.txt"); got != "ab.txt" {
		t.Fatalf("unexpected control-char cleanup: %q", got)
	}
}
