package service

import (
	"crypto/rand"
	"encoding/hex"
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

type IssuedToken struct {
	Token     string
	JTI       string
	ExpiresAt time.Time
	ExpiresIn int64
}

type AccessIdentity struct {
	UserID   int64
	Username string
	Role     string
}

type RefreshIdentity struct {
	UserID int64
	JTI    string
}

func newTokenID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func NewJWTService(secret string, accessTTL, refreshTTL int64) *JWTService {
	return &JWTService{
		secretKey:  []byte(secret),
		accessTTL:  time.Duration(accessTTL) * time.Second,
		refreshTTL: time.Duration(refreshTTL) * time.Second,
	}
}

func (s *JWTService) GenerateAccessToken(userID int64, username, role string) (IssuedToken, error) {
	now := time.Now()
	expiresAt := now.Add(s.accessTTL)

	tokenID, err := newTokenID()
	if err != nil {
		return IssuedToken{}, err
	}

	claims := accessTokenClaims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			ID:        tokenID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString(s.secretKey)
	if err != nil {
		return IssuedToken{}, err
	}

	return IssuedToken{
		Token:     signedToken,
		JTI:       tokenID,
		ExpiresAt: expiresAt,
		ExpiresIn: int64(s.accessTTL / time.Second),
	}, nil
}

func (s *JWTService) GenerateRefreshToken(userID int64) (IssuedToken, error) {
	now := time.Now()
	expiresAt := now.Add(s.refreshTTL)

	tokenID, err := newTokenID()
	if err != nil {
		return IssuedToken{}, err
	}

	claims := refreshTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			ID:        tokenID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString(s.secretKey)
	if err != nil {
		return IssuedToken{}, err
	}

	return IssuedToken{
		Token:     signedToken,
		JTI:       tokenID,
		ExpiresAt: expiresAt,
		ExpiresIn: int64(s.refreshTTL / time.Second),
	}, nil
}

func (s *JWTService) ParseAccessToken(tokenStr string) (AccessIdentity, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &accessTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		return s.secretKey, nil
	})

	if err != nil {
		return AccessIdentity{}, err
	}

	claims, ok := token.Claims.(*accessTokenClaims)
	if !ok || !token.Valid {
		return AccessIdentity{}, jwt.ErrTokenInvalidClaims
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return AccessIdentity{}, err
	}

	return AccessIdentity{
		UserID:   userID,
		Username: claims.Username,
		Role:     claims.Role,
	}, nil
}

func (s *JWTService) ParseRefreshToken(tokenStr string) (RefreshIdentity, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &refreshTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		return s.secretKey, nil
	})

	if err != nil {
		return RefreshIdentity{}, err
	}

	claims, ok := token.Claims.(*refreshTokenClaims)
	if !ok || !token.Valid {
		return RefreshIdentity{}, jwt.ErrTokenInvalidClaims
	}

	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil {
		return RefreshIdentity{}, err
	}

	return RefreshIdentity{
		UserID: userID,
		JTI:    claims.ID,
	}, nil
}
