package transaction

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// FinancialAccountID preserves public account IDs, including legacy 32-character
// registration IDs, while exposing an ObjectID-compatible ledger/feed key.
func FinancialAccountID(id string) string {
	if len(id) == 24 {
		if _, err := hex.DecodeString(id); err == nil {
			return strings.ToLower(id)
		}
	}
	sum := sha256.Sum256([]byte("financial-account:" + id))
	return hex.EncodeToString(sum[:12])
}
