package service

import (
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTService struct {
	secretKey  []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type accessTokenClaims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

type refreshTokenClaims struct {
	jwt.RegisteredClaims
}

func NewJWTService(secret string, accessTTL, refreshTTL int64) *JWTService {
	return &JWTService{
		secretKey:  []byte(secret),
		accessTTL:  time.Duration(accessTTL) * time.Second,
		refreshTTL: time.Duration(refreshTTL) * time.Second,
	}
}

func (s *JWTService) GenerateAccessToken(userID int64, username, role string) (string, int64, error) {
	now := time.Now()
	expiresAt := now.Add(s.accessTTL)

	claims := accessTokenClaims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString(s.secretKey)
	if err != nil {
		return "", 0, err
	}

	return signedToken, int64(s.accessTTL / time.Second), nil
}
func (s *JWTService) GenerateRefreshToken(userID int64) (string, int64, error) {
	now := time.Now()
	expiresAt := now.Add(s.refreshTTL)
	claims := refreshTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString(s.secretKey)
	if err != nil {
		return "", 0, err
	}

	return signedToken, int64(s.refreshTTL / time.Second), nil
}
