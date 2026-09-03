package auth

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("invalid token")

func Issue(secret string, userID uint64, ttl time.Duration) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		Subject:   strconv.FormatUint(userID, 10),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	})
	return token.SignedString([]byte(secret))
}

func Verify(secret, tokenString string) (uint64, error) {
	parsed, err := jwt.Parse(tokenString, func(token *jwt.Token) (any, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !parsed.Valid {
		return 0, ErrInvalidToken
	}
	subject, err := parsed.Claims.GetSubject()
	if err != nil {
		return 0, ErrInvalidToken
	}
	id, err := strconv.ParseUint(subject, 10, 64)
	if err != nil || id == 0 {
		return 0, ErrInvalidToken
	}
	return id, nil
}
