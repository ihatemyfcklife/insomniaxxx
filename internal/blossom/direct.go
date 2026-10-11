package blossom

// DirectUploadTarget is one configured relay and an ephemeral Nostr BUD-02
// upload credential bound to the encrypted blob hash and that relay host.
// The credential authorizes no decryption and must never be logged.
type DirectUploadTarget struct {
	URL           string `json:"url"`
	Authorization string `json:"authorization"`
}

// UploadAuthorization signs a short-lived, hash-scoped BUD-02 event using an
// ephemeral Nostr identity; this never exports the signing private key.
func UploadAuthorization(relayURL, blobHash string) (string, error) {
	return authorization(relayURL, blobHash)
}
