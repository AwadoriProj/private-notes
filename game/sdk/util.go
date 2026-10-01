package sdk

import (
	"crypto/sha256"
	"fmt"
)

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum)
}