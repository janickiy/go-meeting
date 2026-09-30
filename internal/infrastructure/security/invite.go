package security

import (
	"crypto/rand"
	"encoding/base64"
)

func GenerateInviteCode() (string, error) {
	var random [24]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(random[:]), nil
}
