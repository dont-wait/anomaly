package account

import "testing"

func TestCanonicalKYCSessionIDMapsLegacySeedIDDeterministically(t *testing.T) {
	legacy := "seed-kyc-507f1f77bcf86cd799439011"

	first := CanonicalKYCSessionID(legacy)
	second := CanonicalKYCSessionID(legacy)
	if first != second {
		t.Fatalf("legacy ID mapping is not deterministic: %q != %q", first, second)
	}
	if !isObjectID(first) {
		t.Fatalf("canonical ID = %q, want a 24-character hex ObjectID", first)
	}
}

func TestCanonicalKYCSessionIDPreservesValidID(t *testing.T) {
	const id = "507F1F77BCF86CD799439011"
	if got := CanonicalKYCSessionID(id); got != "507f1f77bcf86cd799439011" {
		t.Fatalf("canonical ID = %q, want lowercase valid ID", got)
	}
}

func TestNormalizeKYCSessionIdentifiersKeepsCustomerReferenceAligned(t *testing.T) {
	const legacy = "seed-kyc-507f1f77bcf86cd799439011"
	account := &UserAccount{
		Customer:    &Customer{VerifiedKYCSessionId: legacy},
		KYCSessions: []*KYCSession{{Id: legacy}},
	}

	NormalizeKYCSessionIdentifiers(account)

	if account.KYCSessions[0].Id == legacy || account.Customer.VerifiedKYCSessionId != account.KYCSessions[0].Id {
		t.Fatalf("legacy identifiers were not normalized: session=%q customer=%q", account.KYCSessions[0].Id, account.Customer.VerifiedKYCSessionId)
	}
}

func TestValidateKYCSessionIdentifiersRejectsDanglingReference(t *testing.T) {
	account := &UserAccount{
		Customer: &Customer{VerifiedKYCSessionId: "507f1f77bcf86cd799439011"},
	}

	if err := ValidateKYCSessionIdentifiers(account); err == nil {
		t.Fatal("expected dangling verified KYC reference to be rejected")
	}
}
