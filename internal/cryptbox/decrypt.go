package cryptbox

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"unicode/utf8"

	"github.com/ihatemyfcklife/badcrypt"
)

func DecryptTo(w io.Writer, body io.Reader, contentLength int64, key []byte, maxPlainSize int64) (FileMeta, error) {
	if len(key) != 32 {
		return FileMeta{}, errors.New("invalid share key")
	}

	// Some Blossom implementations do not send Content-Length. Buffer those
	// responses to a temporary file so packet boundaries can be validated.
	var input io.Reader = body
	var cleanup func()
	if contentLength < 0 {
		tmp, err := os.CreateTemp("", "anonblossom-download-*")
		if err != nil {
			return FileMeta{}, err
		}
		cleanup = func() {
			tmp.Close()
			os.Remove(tmp.Name())
		}
		defer cleanup()
		if _, err := io.Copy(tmp, body); err != nil {
			return FileMeta{}, err
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return FileMeta{}, err
		}
		input = tmp
		info, err := tmp.Stat()
		if err != nil {
			return FileMeta{}, err
		}
		contentLength = info.Size()
	}

	reader := bufio.NewReaderSize(input, 64<<10)
	outer := make([]byte, outerHeaderSize)
	if _, err := io.ReadFull(reader, outer); err != nil {
		return FileMeta{}, errors.New("invalid encrypted blob")
	}
	chunkSize, ok := ParseOuterHeader(outer)
	if !ok {
		return FileMeta{}, errors.New("invalid encrypted blob")
	}
	if contentLength < outerHeaderSize+encryptedPacketOverhead {
		return FileMeta{}, errors.New("invalid encrypted blob")
	}

	remaining := contentLength - outerHeaderSize
	packetSize := MaxEncryptedPacketSize(chunkSize)
	if remaining < packetSize {
		packetSize = remaining
	}

	firstPacket := make([]byte, packetSize)
	if _, err := io.ReadFull(reader, firstPacket); err != nil {
		return FileMeta{}, errors.New("truncated encrypted blob")
	}
	if len(firstPacket) < encryptedPacketOverhead {
		return FileMeta{}, errors.New("invalid encrypted blob")
	}

	sessionID := binary.BigEndian.Uint64(firstPacket[:8])
	aead, err := badcrypt.NewShardAEADWithSession(key, sessionID)
	if err != nil {
		return FileMeta{}, err
	}
	defer aead.Close()

	aad := AAD(outer)
	plain, err := aead.Open(nil, firstPacket, aad)
	if err != nil {
		return FileMeta{}, errors.New("invalid share key or corrupted file")
	}
	meta, firstData, err := ParseManifestPrefix(plain)
	if err != nil {
		badcrypt.Zeroize(plain)
		return FileMeta{}, errors.New("invalid encrypted blob")
	}
	if meta.Size < 0 || meta.Size > maxPlainSize || meta.Name == "" || !utf8.ValidString(meta.Name) {
		badcrypt.Zeroize(plain)
		return FileMeta{}, errors.New("invalid encrypted blob")
	}
	meta.Name = CleanFilename(meta.Name)
	if meta.Name == "" {
		meta.Name = "download"
	}
	if meta.Type == "" {
		meta.Type = "application/octet-stream"
	}

	remainingPlain := meta.Size
	if int64(len(firstData)) > remainingPlain {
		firstData = firstData[:remainingPlain]
	}
	if _, err := w.Write(firstData); err != nil {
		badcrypt.Zeroize(plain)
		return meta, err
	}
	remainingPlain -= int64(len(firstData))
	badcrypt.Zeroize(plain)

	remaining -= int64(len(firstPacket))
	for remaining > 0 {
		currentSize := packetSize
		if remaining < currentSize {
			currentSize = remaining
		}
		packet := make([]byte, currentSize)
		if _, err := io.ReadFull(reader, packet); err != nil {
			return meta, err
		}

		plain, err := aead.Open(nil, packet, aad)
		if err != nil {
			return meta, errors.New("corrupted encrypted blob")
		}
		toWrite := plain
		if int64(len(toWrite)) > remainingPlain {
			toWrite = toWrite[:remainingPlain]
		}
		if len(toWrite) > 0 {
			if _, err := w.Write(toWrite); err != nil {
				badcrypt.Zeroize(plain)
				return meta, err
			}
			remainingPlain -= int64(len(toWrite))
		}
		badcrypt.Zeroize(plain)
		if remainingPlain < 0 {
			return meta, errors.New("encrypted file is longer than declared")
		}
		remaining -= currentSize
	}

	if remainingPlain != 0 {
		return meta, errors.New("encrypted file is shorter than declared")
	}
	return meta, nil
}

// InspectShare lit uniquement l'en-tête et le premier bloc pour extraire les
// métadonnées (nom, type MIME, taille) sans déchiffrer tout le fichier.
func InspectShare(body io.Reader, key []byte) (FileMeta, error) {
	if len(key) != 32 {
		return FileMeta{}, errors.New("invalid share key")
	}
	outer := make([]byte, outerHeaderSize)
	if _, err := io.ReadFull(body, outer); err != nil {
		return FileMeta{}, errors.New("invalid encrypted blob")
	}
	chunkSize, ok := ParseOuterHeader(outer)
	if !ok {
		return FileMeta{}, errors.New("invalid encrypted blob")
	}

	packetSize := MaxEncryptedPacketSize(chunkSize)
	firstPacket := make([]byte, packetSize)
	n, err := io.ReadFull(body, firstPacket)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return FileMeta{}, errors.New("truncated encrypted blob")
	}
	if n < encryptedPacketOverhead {
		return FileMeta{}, errors.New("invalid encrypted blob")
	}
	firstPacket = firstPacket[:n]

	sessionID := binary.BigEndian.Uint64(firstPacket[:8])
	aead, err := badcrypt.NewShardAEADWithSession(key, sessionID)
	if err != nil {
		return FileMeta{}, err
	}
	defer aead.Close()

	plain, err := aead.Open(nil, firstPacket, AAD(outer))
	if err != nil {
		return FileMeta{}, errors.New("invalid share key or corrupted file")
	}
	defer badcrypt.Zeroize(plain)

	meta, _, err := ParseManifestPrefix(plain)
	if err != nil {
		return FileMeta{}, errors.New("invalid encrypted blob")
	}
	meta.Name = CleanFilename(meta.Name)
	if meta.Name == "" {
		meta.Name = "download"
	}
	if meta.Type == "" {
		meta.Type = "application/octet-stream"
	}
	return meta, nil
}
