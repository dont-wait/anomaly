package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	domainauth "github.com/dont-wait/anomaly/internal/domain/auth"
)

var ErrInvalidToken = errors.New("invalid token")

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

func (s *TokenService) Issue(userID, username string, isVerify bool) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.expiry)
	claims := jwt.MapClaims{
		"sub":      userID,
		"username": username,
		"isVerify": isVerify,
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
	parsed, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
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
	username, ok := claimsMap["username"].(string)
	if !ok || username == "" {
		return nil, ErrInvalidToken
	}
	isVerify, ok := claimsMap["isVerify"].(bool)
	if !ok {
		return nil, ErrInvalidToken
	}

	expFloat, ok := claimsMap["exp"].(float64)
	if !ok {
		return nil, ErrInvalidToken
	}
	expiresAt := time.Unix(int64(expFloat), 0)

	return &domainauth.Claims{
		UserID:    sub,
		Username:  username,
		IsVerify:  isVerify,
		ExpiresAt: expiresAt,
	}, nil
}
