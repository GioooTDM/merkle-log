package hashutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func ParsePayloadHashValue(v string) (string, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return "", fmt.Errorf("payload_hash.value is required")
	}

	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "hex:") {
		return "", fmt.Errorf(`payload_hash.value must use "hex:<digest>" format`)
	}
	hexPart := lower[len("hex:"):]
	if len(hexPart) != 64 {
		return "", fmt.Errorf("payload_hash.value must contain 64 hex chars for sha-256")
	}

	raw, err := hex.DecodeString(hexPart)
	if err != nil || len(raw) != 32 {
		return "", fmt.Errorf("payload_hash.value is not valid sha-256 hex")
	}
	return hexPart, nil
}

func SHA256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
