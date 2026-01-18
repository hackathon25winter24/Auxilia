package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenManager はJWTトークンの生成・検証を管理します
type TokenManager struct {
	secretKey          string
	accessTokenExpiry  time.Duration
	refreshTokenExpiry time.Duration
}

// NewTokenManager は新しいTokenManagerを作成します
func NewTokenManager(secretKey string, accessExpiry, refreshExpiry time.Duration) *TokenManager {
	return &TokenManager{
		secretKey:          secretKey,
		accessTokenExpiry:  accessExpiry,
		refreshTokenExpiry: refreshExpiry,
	}
}

// GenerateTokenPair はアクセストークンとリフレッシュトークンを生成します
func (tm *TokenManager) GenerateTokenPair(user *User) (*TokenPair, error) {
	// アクセストークン生成（短期）
	accessToken, err := tm.generateToken(user, "access", tm.accessTokenExpiry)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	// リフレッシュトークン生成（長期）
	refreshToken, err := tm.generateToken(user, "refresh", tm.refreshTokenExpiry)
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(tm.accessTokenExpiry.Seconds()),
		TokenType:    "Bearer",
	}, nil
}

// generateToken は指定されたタイプのトークンを生成します
func (tm *TokenManager) generateToken(user *User, tokenType string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":    user.ID,
		"email":      user.Email,
		"username":   user.Username,
		"token_type": tokenType,
		"iat":        now.Unix(),
		"exp":        now.Add(expiry).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(tm.secretKey))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
}

// VerifyAccessToken はアクセストークンを検証します
func (tm *TokenManager) VerifyAccessToken(tokenString string) (*JWTClaims, error) {
	return tm.verifyToken(tokenString, "access")
}

// VerifyRefreshToken はリフレッシュトークンを検証します
func (tm *TokenManager) VerifyRefreshToken(tokenString string) (*JWTClaims, error) {
	return tm.verifyToken(tokenString, "refresh")
}

// verifyToken はトークンを検証し、クレーム情報を返します
func (tm *TokenManager) verifyToken(tokenString string, expectedType string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		// 署名方式の確認（HMAC以外の署名は拒否）
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(tm.secretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}

	// トークンタイプの確認
	tokenType, ok := claims["token_type"].(string)
	if !ok || tokenType != expectedType {
		return nil, fmt.Errorf("invalid token type")
	}

	// ユーザー情報の抽出
	userID, ok := claims["user_id"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid user_id in token")
	}

	return &JWTClaims{
		UserID:    int(userID),
		Email:     claims["email"].(string),
		Username:  claims["username"].(string),
		TokenType: tokenType,
	}, nil
}
