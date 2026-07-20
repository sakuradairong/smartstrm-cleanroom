package signature

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
)

func Create(secret, storageID, remotePath string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(storageID))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(remotePath))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func Valid(secret, storageID, remotePath, provided string) bool {
	expected := Create(secret, storageID, remotePath)
	return hmac.Equal([]byte(expected), []byte(provided))
}
