package id

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

func New(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strings.TrimSuffix(prefix, "_") + "_fallback"
	}
	return strings.TrimSuffix(prefix, "_") + "_" + hex.EncodeToString(b[:])
}
