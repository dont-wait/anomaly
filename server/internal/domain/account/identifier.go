package account

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// CanonicalKYCSessionID returns the Mongo-compatible ID for a KYC session.
//
// KYC session IDs are ObjectIDs at the persistence boundary. Older demo seed
// events used the human-readable "seed-kyc-" prefix instead, so retain a
// deterministic mapping for those events while leaving other malformed IDs
// untouched and therefore visible to validation.
func CanonicalKYCSessionID(id string) string {
	if isObjectID(id) {
		return strings.ToLower(id)
	}
	if !strings.HasPrefix(id, "seed-kyc-") {
		return id
	}

	sum := sha256.Sum256([]byte("kyc-session:" + id))
	return hex.EncodeToString(sum[:12])
}

func isObjectID(id string) bool {
	if len(id) != 24 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// NormalizeKYCSessionIdentifiers updates legacy seed IDs in an account and
// keeps the customer reference aligned with the corresponding session.
func NormalizeKYCSessionIdentifiers(account *UserAccount) {
	if account == nil {
		return
	}
	for _, session := range account.KYCSessions {
		if session != nil {
			session.Id = CanonicalKYCSessionID(session.Id)
		}
	}
	if account.Customer != nil {
		account.Customer.VerifiedKYCSessionId = CanonicalKYCSessionID(account.Customer.VerifiedKYCSessionId)
	}
}
