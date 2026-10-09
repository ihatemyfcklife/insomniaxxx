package blossom

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"time"

	"fiatjaf.com/nostr"
	"github.com/ihatemyfcklife/badcrypt"
)

func authorization(serverURL, blobHash string) (string, error) {
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
			{"server", publicHost(serverURL)},
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
