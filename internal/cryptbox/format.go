package cryptbox

import (
	"crypto/rand"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	outerMagic              = "ABLOBSH1"
	outerHeaderSize         = 16
	formatVersion           = 1
	manifestKeyFormatVersion = 2
	manifestMagic           = "ABMAN001"
	maxFilenameBytes        = 255
	maxManifestBytes        = 64 << 10
	encryptedPacketOverhead = 36 // 8B session + 12B nonce + 16B tag.
)

var aadPrefix = []byte("anonblossom/v1:")

// FileMeta is the encrypted file metadata carried in the first packet.
type FileMeta struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
}

// NewShareKey creates a random 256-bit share key.
func NewShareKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// ShareKeyText encodes a share key for use in a URL path.
func ShareKeyText(key []byte) string {
	return base64.RawURLEncoding.EncodeToString(key)
}

// ParseDownloadPath extracts and validates a share hash and 256-bit key.
func ParseDownloadPath(path string) (hash string, key []byte, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 3 || parts[0] != "d" || !ValidHash(parts[1]) {
		return "", nil, false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(decoded) != 32 {
		return "", nil, false
	}
	return parts[1], decoded, true
}

func ValidHash(hash string) bool {
	if len(hash) != 64 || strings.ToLower(hash) != hash {
		return false
	}
	_, err := hex.DecodeString(hash)
	return err == nil
}

func MakeOuterHeader(chunkSize int) []byte {
	header := make([]byte, outerHeaderSize)
	copy(header[:8], outerMagic)
	binary.BigEndian.PutUint32(header[8:12], uint32(chunkSize))
	binary.BigEndian.PutUint32(header[12:16], formatVersion)
	return header
}

// ManifestKey isolates manifest encryption from file encryption under one share secret.
func ManifestKey(shareKey []byte) []byte {
	if len(shareKey) != 32 { return nil }
	mac := hmac.New(sha256.New, shareKey)
	_, _ = mac.Write([]byte("anonblossom/manifest-key/v2"))
	return mac.Sum(nil)
}

func MakeManifestOuterHeader(chunkSize int) []byte {
	header := MakeOuterHeader(chunkSize)
	binary.BigEndian.PutUint32(header[12:16], manifestKeyFormatVersion)
	return header
}

func UsesManifestKey(header []byte) bool {
	return len(header) == outerHeaderSize && string(header[:8]) == outerMagic &&
		binary.BigEndian.Uint32(header[12:16]) == manifestKeyFormatVersion
}

func ParseOuterHeader(header []byte) (chunkSize int, ok bool) {
	if len(header) != outerHeaderSize || string(header[:8]) != outerMagic {
		return 0, false
	}
	version := binary.BigEndian.Uint32(header[12:16])
	if version != formatVersion && version != manifestKeyFormatVersion {
		return 0, false
	}
	chunkSize = int(binary.BigEndian.Uint32(header[8:12]))
	return chunkSize, chunkSize >= 64<<10 && chunkSize <= 8<<20
}

func AAD(outerHeader []byte) []byte {
	out := make([]byte, 0, len(aadPrefix)+len(outerHeader))
	out = append(out, aadPrefix...)
	out = append(out, outerHeader...)
	return out
}

func MakeManifestPrefix(meta FileMeta) ([]byte, error) {
	if !utf8.ValidString(meta.Name) || len(meta.Name) > maxFilenameBytes || meta.Name == "" {
		return nil, errors.New("invalid filename")
	}
	if meta.Size < 0 {
		return nil, errors.New("invalid size")
	}
	if meta.Type == "" {
		meta.Type = "application/octet-stream"
	}

	body, err := json.Marshal(meta)
	if err != nil || len(body) > maxManifestBytes {
		return nil, errors.New("invalid metadata")
	}

	prefix := make([]byte, 12+len(body))
	copy(prefix[:8], manifestMagic)
	binary.BigEndian.PutUint32(prefix[8:12], uint32(len(body)))
	copy(prefix[12:], body)
	return prefix, nil
}

func ParseManifestPrefix(plain []byte) (FileMeta, []byte, error) {
	if len(plain) < 12 || string(plain[:8]) != manifestMagic {
		return FileMeta{}, nil, errors.New("bad manifest")
	}
	n := int(binary.BigEndian.Uint32(plain[8:12]))
	if n <= 0 || n > maxManifestBytes || 12+n > len(plain) {
		return FileMeta{}, nil, errors.New("bad manifest length")
	}

	var meta FileMeta
	if err := json.Unmarshal(plain[12:12+n], &meta); err != nil {
		return FileMeta{}, nil, err
	}
	return meta, plain[12+n:], nil
}

func CleanFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimRight(name, ".")
	if name == "." || name == ".." || name == "" {
		return ""
	}
	for _, separator := range []string{"/", "\\"} {
		if index := strings.LastIndex(name, separator); index >= 0 {
			name = name[index+1:]
		}
	}

	var builder strings.Builder
	for _, r := range name {
		if r == '/' || r == '\\' || r < 0x20 || r == 0x7f {
			continue
		}
		builder.WriteRune(r)
	}
	name = builder.String()
	for len(name) > maxFilenameBytes {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name
}

func ContentDisposition(name string) string {
	name = CleanFilename(name)
	if name == "" {
		name = "download"
	}
	ascii := strings.Map(func(r rune) rune {
		if r >= 0x20 && r <= 0x7e && r != '"' && r != '\\' {
			return r
		}
		return '_'
	}, name)
	if ascii == "" {
		ascii = "download"
	}
	return fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, ascii, url.PathEscape(name))
}

func MaxEncryptedPacketSize(chunkSize int) int64 {
	return int64(chunkSize + encryptedPacketOverhead)
}
