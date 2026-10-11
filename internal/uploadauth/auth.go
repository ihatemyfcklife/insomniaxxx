package uploadauth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"time"

	"fiatjaf.com/nostr"
	"github.com/ihatemyfcklife/badcrypt"
)

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
	secret := nostr.Generate()
	defer badcrypt.Zeroize(secret[:])

	now := time.Now().Unix()
	event := nostr.Event{
		Kind:      24242,
		CreatedAt: nostr.Timestamp(now),
		Tags: nostr.Tags{
			{"t", "upload"},
			{"x", blobHash},
			{"expiration", strconv.FormatInt(now+300, 10)},
			{"server", u.Hostname()},
		},
		Content: "anonblossom upload",
	}
	if err := event.Sign(secret); err != nil {
		return "", err
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	return "Nostr " + base64.StdEncoding.EncodeToString(encoded), nil
}
