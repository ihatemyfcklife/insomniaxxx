package blossom

import "anonblossom/internal/uploadauth"

func authorization(serverURL, blobHash string) (string, error) {
	return uploadauth.Sign(serverURL, blobHash)
}
