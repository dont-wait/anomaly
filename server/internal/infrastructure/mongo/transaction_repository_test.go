package mongo

import "testing"

func TestNormalizeFeedSearch(t *testing.T) {
	if got, want := normalizeFeedSearch("Đặng Đình"), "dang dinh"; got != want {
		t.Fatalf("normalizeFeedSearch() = %q, want %q", got, want)
	}
}
