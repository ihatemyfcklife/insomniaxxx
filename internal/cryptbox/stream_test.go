package cryptbox

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestStreamEncryptorRoundTripAndHash(t *testing.T) {
	for _, size := range []int{0, 1, 123, 64<<10 - 64, 64 << 10, 3<<20 + 123} {
		for _, kind := range []string{"application/octet-stream", "application/x-anonblossom-manifest+json"} {
			t.Run(kind+"/"+humanStreamSize(size), func(t *testing.T) {
				plain := bytes.Repeat([]byte{0x42, 0x69, 0x13, 0xa7}, (size+3)/4)[:size]
				meta := FileMeta{Name: "testing.bin", Type: kind, Size: int64(size)}
				key := bytes.Repeat([]byte{0x23}, 32)
				stream, err := NewStreamEncryptor(key, meta, 64<<10)
				if err != nil {
					t.Fatal(err)
				}
				defer stream.Close()

				var ciphertext bytes.Buffer
				ciphertext.Write(stream.Header())
				emit := func(chunk []byte) error {
					_, err := ciphertext.Write(chunk)
					return err
				}
				// Feed pieces deliberately not aligned with the AEAD packet size.
				for i := 0; i < len(plain); i += 16381 {
					end := i + 16381
					if end > len(plain) {
						end = len(plain)
					}
					if err := stream.Write(plain[i:end], emit); err != nil {
						t.Fatal(err)
					}
				}
				digest, err := stream.Finish(emit)
				if err != nil {
					t.Fatal(err)
				}
				expected := sha256.Sum256(ciphertext.Bytes())
				if digest != hex.EncodeToString(expected[:]) {
					t.Fatal("incremental ciphertext SHA-256 differs from full SHA-256")
				}
				var result bytes.Buffer
				gotMeta, err := DecryptTo(&result, bytes.NewReader(ciphertext.Bytes()), int64(ciphertext.Len()), key, 64<<20)
				if err != nil {
					t.Fatalf("stream ciphertext could not be decrypted by existing reader: %v", err)
				}
				if !bytes.Equal(result.Bytes(), plain) || gotMeta.Name != meta.Name || gotMeta.Type != kind {
					t.Fatal("stream encryption is not wire-compatible with classic encrypted shares")
				}
			})
		}
	}
}

func humanStreamSize(size int) string {
	if size == 0 {
		return "empty"
	}
	if size >= 1<<20 {
		return "multi-megabyte"
	}
	return stringSize(size)
}

func stringSize(size int) string {
	switch size {
	case 1:
		return "one-byte"
	case 123:
		return "tiny"
	case 64<<10 - 64:
		return "near-boundary"
	default:
		return "one-packet"
	}
}

func TestStreamEncryptorRejectsLengthMismatch(t *testing.T) {
	key := bytes.Repeat([]byte{0x56}, 32)
	meta := FileMeta{Name: "incomplete.txt", Type: "text/plain", Size: 10}
	s, err := NewStreamEncryptor(key, meta, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Write(bytes.Repeat([]byte{3}, 11), func([]byte) error { return nil }); err == nil {
		t.Fatal("accepted plaintext longer than metadata")
	}
	if _, err := s.Finish(func([]byte) error { return nil }); err == nil {
		t.Fatal("accepted truncated plaintext")
	}
	s.Close()
	if err := s.Write([]byte{1}, func([]byte) error { return nil }); err == nil {
		t.Fatal("accepted write after close")
	}
}
