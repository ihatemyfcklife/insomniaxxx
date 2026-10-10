package cryptbox

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"

	"github.com/ihatemyfcklife/badcrypt"
)

const defaultSessionBytes = 8

// EncryptShare writes one encrypted blob and returns its random share key.
// dst accepts any io.Writer so the same flow works with *os.File on the
// server and bytes.Buffer under WebAssembly.
func EncryptShare(src io.Reader, dst io.Writer, meta FileMeta, chunkSize int) ([]byte, error) {
	key, err := NewShareKey()
	if err != nil {
		return nil, err
	}
	if err := EncryptShareWithKey(key, src, dst, meta, chunkSize); err != nil {
		badcrypt.Zeroize(key)
		return nil, err
	}
	return key, nil
}

// EncryptShareWithKey encrypts a stream with an existing 256-bit key.
func EncryptShareWithKey(key []byte, src io.Reader, dst io.Writer, meta FileMeta, chunkSize int) error {
	var sessionBytes [defaultSessionBytes]byte
	if _, err := rand.Read(sessionBytes[:]); err != nil {
		return err
	}
	sessionID := binary.BigEndian.Uint64(sessionBytes[:])

	outer := MakeOuterHeader(chunkSize)
	if meta.Type == "application/x-anonblossom-manifest+json" {
		// The file and manifest must never share a ChaCha20-Poly1305 key/nonce.
		derivedKey := ManifestKey(key)
		if derivedKey == nil {
			return errors.New("invalid manifest key")
		}
		defer badcrypt.Zeroize(derivedKey)
		key = derivedKey
		outer = MakeManifestOuterHeader(chunkSize)
	}
	aead, err := badcrypt.NewShardAEADWithSession(key, sessionID)
	if err != nil {
		return err
	}
	defer aead.Close()

	prefix, err := MakeManifestPrefix(meta)
	if err != nil {
		return err
	}
	return writeEncrypted(dst, src, aead, outer, prefix, chunkSize)
}

func writeEncrypted(dst io.Writer, src io.Reader, aead *badcrypt.ShardAEAD, outer, prefix []byte, chunkSize int) error {
	if _, err := dst.Write(outer); err != nil {
		return err
	}

	buffer := make([]byte, chunkSize)
	reader := io.MultiReader(bytes.NewReader(prefix), src)
	for {
		n, err := io.ReadFull(reader, buffer)
		if n > 0 {
			sealed, sealErr := aead.Seal(nil, buffer[:n], AAD(outer))
			if sealErr != nil {
				return sealErr
			}
			if _, writeErr := dst.Write(sealed); writeErr != nil {
				return writeErr
			}
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
