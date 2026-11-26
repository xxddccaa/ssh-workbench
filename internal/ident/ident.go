package ident

import (
	"crypto/rand"
	"encoding/base64"
)

func New() string {
	var buffer [24]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer[:])
}
