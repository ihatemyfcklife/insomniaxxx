package uploadauth

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
)

// Keep this wire format independent of relay/network clients so the same
// signer builds natively and in js/wasm. Signatures use the existing BIP-340
// implementation; no custom elliptic-curve or signature code is involved.
type uploadEvent struct {
	ID        string     `json:"id"`
	PubKey    string     `json:"pubkey"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig"`
}

// Sign creates an ephemeral hash-scoped credential without network access.
func Sign(serverURL, blobHash string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
		return "", errors.New("invalid relay origin")
	}
	if len(blobHash) != 64 {
		return "", errors.New("invalid blob hash")
	}
	for _, c := range blobHash {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return "", errors.New("invalid blob hash")
		}
	}
	secret, err := btcec.NewPrivateKey()
	if err != nil {
		return "", err
	}
	defer secret.Zero()

	now := time.Now().Unix()
	event := uploadEvent{
		PubKey:    hex.EncodeToString(schnorr.SerializePubKey(secret.PubKey())),
		Kind:      24242,
		CreatedAt: now,
		Tags: [][]string{
			{"t", "upload"},
			{"x", blobHash},
			{"expiration", strconv.FormatInt(now+300, 10)},
			{"server", u.Hostname()},
		},
		Content: "anonblossom upload",
	}
	// NIP-01 canonical event preimage. Disable HTML escaping so the JSON
	// serialization agrees with relay verification.
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode([]any{0, event.PubKey, event.CreatedAt, event.Kind, event.Tags, event.Content}); err != nil {
		return "", err
	}
	id := sha256.Sum256(bytes.TrimSuffix(canonical.Bytes(), []byte("\n")))
	sig, err := schnorr.Sign(secret, id[:])
	if err != nil {
		return "", err
	}
	event.ID = hex.EncodeToString(id[:])
	event.Sig = hex.EncodeToString(sig.Serialize())

	encoded, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	return "Nostr " + base64.StdEncoding.EncodeToString(encoded), nil
}
