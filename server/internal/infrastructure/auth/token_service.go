package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	domainauth "github.com/dont-wait/anomaly/internal/domain/auth"
)

var ErrInvalidToken = errors.New("invalid token")

const (
	accessTokenPurpose = "access"
	kycTokenPurpose    = "kyc"
	kycTokenExpiry     = 15 * time.Minute
)

type TokenService struct {
	secret []byte
	expiry time.Duration
}

func NewTokenService(secret string, expiry time.Duration) *TokenService {
	return &TokenService{
		secret: []byte(secret),
		expiry: expiry,
	}
}

func (s *TokenService) Issue(userID, username, role string, isVerify bool) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.expiry)
	claims := jwt.MapClaims{
		"sub":      userID,
		"username": username,
		"role":     role,
		"isVerify": isVerify,
		"purpose":  accessTokenPurpose,
		"exp":      expiresAt.Unix(),
		"iat":      time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, expiresAt, nil
}

func (s *TokenService) Parse(tokenString string) (*domainauth.Claims, error) {
	return s.parse(tokenString, accessTokenPurpose)
}

func (s *TokenService) IssueKYC(userID string) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(kycTokenExpiry)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":     userID,
		"purpose": kycTokenPurpose,
		"exp":     expiresAt.Unix(),
		"iat":     now.Unix(),
	})
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign KYC token: %w", err)
	}
	return signed, expiresAt, nil
}

func (s *TokenService) ParseKYC(tokenString string) (*domainauth.Claims, error) {
	return s.parse(tokenString, kycTokenPurpose)
}

func (s *TokenService) parse(tokenString, expectedPurpose string) (*domainauth.Claims, error) {
	parsed, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil || !parsed.Valid {
		return nil, ErrInvalidToken
	}

	claimsMap, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}

	sub, ok := claimsMap["sub"].(string)
	if !ok || sub == "" {
		return nil, ErrInvalidToken
	}
	purpose, ok := claimsMap["purpose"].(string)
	if !ok || purpose != expectedPurpose {
		return nil, ErrInvalidToken
	}

	exp, err := claimsMap.GetExpirationTime()
	if err != nil || exp == nil {
		return nil, ErrInvalidToken
	}

	claims := &domainauth.Claims{
		UserID:    sub,
		Purpose:   purpose,
		ExpiresAt: exp.Time,
	}
	if expectedPurpose == kycTokenPurpose {
		return claims, nil
	}

	username, ok := claimsMap["username"].(string)
	if !ok || username == "" {
		return nil, ErrInvalidToken
	}
	role, ok := claimsMap["role"].(string)
	if !ok || role == "" {
		role = "user"
	}
	isVerify, ok := claimsMap["isVerify"].(bool)
	if !ok {
		return nil, ErrInvalidToken
	}

	claims.Username = username
	claims.Role = role
	claims.IsVerify = isVerify
	return claims, nil
}
