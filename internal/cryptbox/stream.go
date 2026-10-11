package cryptbox

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"

	"github.com/ihatemyfcklife/badcrypt"
)

// StreamEncryptor keeps only one AEAD packet of plaintext in Go memory.
// It emits the same ABLOBSH1 wire format as EncryptShareWithKey and keeps
// only the incremental SHA-256 state, not a full ciphertext copy.
type StreamEncryptor struct {
	aead     *badcrypt.ShardAEAD
	outer    []byte
	aad      []byte
	buffer   []byte
	used     int
	expected int64
	received int64
	hasher   hash.Hash
	closed   bool
}

func NewStreamEncryptor(shareKey []byte, meta FileMeta, chunkSize int) (*StreamEncryptor, error) {
	if len(shareKey) != 32 || chunkSize < 64<<10 || chunkSize > 8<<20 || meta.Size < 0 {
		return nil, errors.New("invalid streaming encryption parameters")
	}
	prefix, err := MakeManifestPrefix(meta)
	if err != nil {
		return nil, err
	}
	if len(prefix) >= chunkSize {
		return nil, errors.New("metadata is too large for streaming chunk")
	}

	var session [defaultSessionBytes]byte
	if _, err := rand.Read(session[:]); err != nil {
		return nil, err
	}

	key := shareKey
	outer := MakeOuterHeader(chunkSize)
	if meta.Type == "application/x-anonblossom-manifest+json" {
		key = ManifestKey(shareKey)
		if key == nil {
			return nil, errors.New("invalid manifest key")
		}
		defer badcrypt.Zeroize(key)
		outer = MakeManifestOuterHeader(chunkSize)
	}

	aead, err := badcrypt.NewShardAEADWithSession(key, binary.BigEndian.Uint64(session[:]))
	if err != nil {
		return nil, err
	}
	s := &StreamEncryptor{
		aead:     aead,
		outer:    outer,
		aad:      AAD(outer),
		buffer:   make([]byte, chunkSize),
		expected: meta.Size,
		hasher:   sha256.New(),
	}
	copy(s.buffer, prefix)
	s.used = len(prefix)
	_, _ = s.hasher.Write(outer)
	return s, nil
}

func (s *StreamEncryptor) Header() []byte {
	return append([]byte(nil), s.outer...)
}

// Write encrypts complete packets and emits each one immediately. The
// callback must consume the packet before returning; packet slices must not
// be retained without copying them.
func (s *StreamEncryptor) Write(plain []byte, emit func([]byte) error) error {
	if s.closed {
		return errors.New("stream already closed")
	}
	if int64(len(plain)) > s.expected-s.received {
		return errors.New("file exceeds declared plaintext size")
	}
	s.received += int64(len(plain))
	for len(plain) > 0 {
		n := copy(s.buffer[s.used:], plain)
		s.used += n
		plain = plain[n:]
		if s.used == len(s.buffer) {
			if err := s.emitPacket(emit); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *StreamEncryptor) emitPacket(emit func([]byte) error) error {
	sealed, err := s.aead.Seal(nil, s.buffer[:s.used], s.aad)
	if err != nil {
		return err
	}
	_, _ = s.hasher.Write(sealed)
	s.used = 0
	return emit(sealed)
}

// Finish emits the final authenticated packet; incomplete file reads are
// rejected, and the owner must always Close the encoder on every exit path.
func (s *StreamEncryptor) Finish(emit func([]byte) error) (string, error) {
	if s.closed {
		return "", errors.New("stream already closed")
	}
	if s.received != s.expected {
		return "", errors.New("file shorter than declared plaintext size")
	}
	if s.used > 0 {
		if err := s.emitPacket(emit); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(s.hasher.Sum(nil)), nil
}

func (s *StreamEncryptor) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	badcrypt.Zeroize(s.buffer)
	s.aead.Close()
}
