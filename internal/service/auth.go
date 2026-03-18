package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/repository"
	mysqlDriver "github.com/go-sql-driver/mysql"
)

var (
	ErrInvalidCredentials    = errors.New("invalid credentials")
	ErrInvalidRefreshToken   = errors.New("invalid refresh token")
	ErrUsernameAlreadyExists = errors.New("username already exists")
)

type AuthService struct {
	userRepo         *repository.UserRepository
	refreshTokenRepo *repository.RefreshTokenRepository
	jwtService       *JWTService
}

type LoginResult struct {
	User         AccessIdentity
	AccessToken  IssuedToken
	RefreshToken IssuedToken
}

type RefreshResult struct {
	AccessToken IssuedToken
	//TODO: 后续做refresh token轮换
}

func NewAuthService(
	userRepo *repository.UserRepository,
	refreshTokenRepo *repository.RefreshTokenRepository,
	jwtService *JWTService,
) *AuthService {
	return &AuthService{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		jwtService:       jwtService,
	}
}

func (s *AuthService) Register(ctx context.Context, username, password string) error {
	passwordHash, err := HashPassword(password)
	if err != nil {
		return err
	}

	user := &entity.User{
		Username:     username,
		PasswordHash: passwordHash,
		Role:         "user",
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		var sqlErr *mysqlDriver.MySQLError
		if errors.As(err, &sqlErr) && sqlErr.Number == 1062 {
			return ErrUsernameAlreadyExists
		}
		return err
	}

	return nil
}

func (s *AuthService) Login(ctx context.Context, username, password string) (LoginResult, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LoginResult{}, ErrInvalidCredentials
		}
		return LoginResult{}, err
	}

	if err := VerifyPassword(user.PasswordHash, password); err != nil {
		return LoginResult{}, ErrInvalidCredentials
	}

	accessToken, err := s.jwtService.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		return LoginResult{}, err
	}

	refreshToken, err := s.jwtService.GenerateRefreshToken(user.ID)
	if err != nil {
		return LoginResult{}, err
	}

	refreshTokenEntity := &entity.RefreshToken{
		JTI:       refreshToken.JTI,
		UserID:    user.ID,
		ExpiresAt: refreshToken.ExpiresAt,
	}

	if err := s.refreshTokenRepo.Create(ctx, refreshTokenEntity); err != nil {
		return LoginResult{}, err
	}

	return LoginResult{
		User: AccessIdentity{
			UserID:   user.ID,
			Username: user.Username,
			Role:     user.Role,
		},
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (RefreshResult, error) {
	identity, err := s.jwtService.ParseRefreshToken(refreshToken)
	if err != nil {
		return RefreshResult{}, ErrInvalidRefreshToken
	}

	storedToken, err := s.refreshTokenRepo.GetByJTIAndUserID(ctx, identity.JTI, identity.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RefreshResult{}, ErrInvalidRefreshToken
		}
		return RefreshResult{}, err
	}

	if storedToken.RevokedAt != nil {
		return RefreshResult{}, ErrInvalidRefreshToken
	}

	if time.Now().After(storedToken.ExpiresAt) {
		return RefreshResult{}, ErrInvalidRefreshToken
	}

	user, err := s.userRepo.GetByID(ctx, identity.UserID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RefreshResult{}, ErrInvalidRefreshToken
		}
		return RefreshResult{}, err
	}

	accessToken, err := s.jwtService.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		return RefreshResult{}, err
	}

	return RefreshResult{
		AccessToken: accessToken,
	}, nil
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	identity, err := s.jwtService.ParseRefreshToken(refreshToken)
	if err != nil {
		return ErrInvalidRefreshToken
	}

	revoked, err := s.refreshTokenRepo.RevokeByJTIAndUserID(ctx, identity.JTI, identity.UserID)
	if err != nil {
		return err
	}

	if !revoked {
		return ErrInvalidRefreshToken
	}

	return nil
}
