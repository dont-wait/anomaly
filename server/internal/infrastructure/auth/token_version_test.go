package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type versionStub struct {
	version int64
	err     error
}

func (s *versionStub) TokenVersion(context.Context, string) (int64, error) { return s.version, s.err }

const testSecret = "0123456789abcdef0123456789abcdef"

func TestParseRejectsTokenIssuedBeforePasswordChange(t *testing.T) {
	versions := &versionStub{}
	service := NewTokenService(testSecret, time.Hour).WithVersionLookup(versions)
	token, _, err := service.Issue("owner", "user", "user", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Parse(token); err != nil {
		t.Fatalf("current token rejected: %v", err)
	}
	versions.version = 1
	if _, err := service.Parse(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("old token error = %v", err)
	}
	fresh, _, err := service.Issue("owner", "user", "user", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Parse(fresh); err != nil {
		t.Fatalf("new token rejected: %v", err)
	}
}

func TestParseTreatsLegacyTokenAsVersionZero(t *testing.T) {
	versions := &versionStub{}
	service := NewTokenService(testSecret, time.Hour).WithVersionLookup(versions)
	legacy, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "owner", "username": "user", "role": "user", "isVerify": true,
		"purpose": accessTokenPurpose, "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Parse(legacy); err != nil {
		t.Fatalf("legacy token rejected at version 0: %v", err)
	}
	versions.version = 1
	if _, err := service.Parse(legacy); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("legacy token error at version 1 = %v", err)
	}
}

func TestParseFailsClosedWhenVersionUnavailable(t *testing.T) {
	versions := &versionStub{}
	service := NewTokenService(testSecret, time.Hour).WithVersionLookup(versions)
	token, _, err := service.Issue("owner", "user", "user", true)
	if err != nil {
		t.Fatal(err)
	}
	versions.err = errors.New("store unavailable")
	if _, err := service.Parse(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("error = %v", err)
	}
}
