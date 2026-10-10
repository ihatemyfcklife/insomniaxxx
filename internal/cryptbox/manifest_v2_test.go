package cryptbox

import (
	"bytes"
	"testing"

	"github.com/ihatemyfcklife/badcrypt"
)

// Both the client and no-JS server use this encoding, including for RLNC
// manifests. Existing version-1 manifest shares remain decryptable.
func TestManifestEncryptionUsesDomainSeparatedKey(t *testing.T) {
	key := bytes.Repeat([]byte{0x73}, 32)
	content := []byte(`{"v":1,"shards":[]}`)
	meta := FileMeta{Name: "manifest.json", Type: "application/x-anonblossom-manifest+json", Size: int64(len(content))}
	var out bytes.Buffer
	if err := EncryptShareWithKey(key, bytes.NewReader(content), &out, meta, 64<<10); err != nil {
		t.Fatal(err)
	}
	header := out.Bytes()[:16]
	if !UsesManifestKey(header) {
		t.Fatal("manifest encryption did not select isolated v2 key")
	}
	if _, ok := ParseOuterHeader(header); !ok {
		t.Fatal("v2 header rejected")
	}
	derived := ManifestKey(key)
	defer badcrypt.Zeroize(derived)
	if bytes.Equal(derived, key) {
		t.Fatal("manifest and file encryption keys are identical")
	}
	inspected, err := InspectShare(bytes.NewReader(out.Bytes()), key)
	if err != nil || inspected != meta {
		t.Fatalf("v2 inspection: %#v, %v", inspected, err)
	}
	var plaintext bytes.Buffer
	decodedMeta, err := DecryptTo(&plaintext, bytes.NewReader(out.Bytes()), int64(out.Len()), key, 1<<20)
	if err != nil || decodedMeta != meta || !bytes.Equal(plaintext.Bytes(), content) {
		t.Fatalf("v2 manifest roundtrip failed: %#v %v", decodedMeta, err)
	}
}

func TestLegacyV1ManifestCanStillDecrypt(t *testing.T) {
	key := bytes.Repeat([]byte{0x29}, 32)
	content := []byte(`{"v":1,"shards":[]}`)
	meta := FileMeta{Name: "manifest.json", Type: "application/x-anonblossom-manifest+json", Size: int64(len(content))}
	prefix, err := MakeManifestPrefix(meta)
	if err != nil {
		t.Fatal(err)
	}
	outer := MakeOuterHeader(64 << 10)
	aead, err := badcrypt.NewShardAEADWithSession(key, 123)
	if err != nil {
		t.Fatal(err)
	}
	defer aead.Close()
	var old bytes.Buffer
	if err := writeEncrypted(&old, bytes.NewReader(content), aead, outer, prefix, 64<<10); err != nil {
		t.Fatal(err)
	}
	if UsesManifestKey(old.Bytes()[:16]) {
		t.Fatal("legacy header changed")
	}
	var plain bytes.Buffer
	decoded, err := DecryptTo(&plain, bytes.NewReader(old.Bytes()), int64(old.Len()), key, 1<<20)
	if err != nil || decoded != meta || !bytes.Equal(plain.Bytes(), content) {
		t.Fatalf("legacy manifest not decryptable: %#v %v", decoded, err)
	}
}
