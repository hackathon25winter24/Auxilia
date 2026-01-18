package auth

import (
	"fmt"
	"time"
)

// User はユーザーデータを表します
type User struct {
	ID           int       `json:"id"`
	Email        string    `json:"email"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"` // JSONレスポンスに含めない
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TokenPair はアクセストークンとリフレッシュトークンのペアです
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"` // 秒単位
	TokenType    string `json:"token_type"` // "Bearer"
}

// JWTClaims はJWTに含まれるクレーム（情報）です
type JWTClaims struct {
	UserID    int    `json:"user_id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	TokenType string `json:"token_type"` // "access" or "refresh"
}

// LoginRequest はログインリクエストのボディです
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterRequest はユーザー登録リクエストのボディです
type RegisterRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthError は認証エラーの詳細です
type AuthError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error はerrorインターフェースを実装します
func (ae *AuthError) Error() string {
	return fmt.Sprintf("[%s] %s", ae.Code, ae.Message)
}
