package auth

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"auxilia/config"
	"auxilia/db"

	"golang.org/x/crypto/bcrypt"
)

// AuthService はユーザー認証に関するロジックを管理します
type AuthService struct {
	config       *config.Config
	tokenManager *TokenManager
	db           *sql.DB
}

// NewAuthService は新しいAuthServiceを作成します
func NewAuthService(cfg *config.Config, tm *TokenManager) *AuthService {
	return &AuthService{
		config:       cfg,
		tokenManager: tm,
		db:           db.GetDB(),
	}
}

// RegisterUser は新しいユーザーを作成します
func (as *AuthService) RegisterUser(req *RegisterRequest) (*User, error) {
	// バリデーション
	if err := as.validateRegistration(req); err != nil {
		return nil, err
	}

	// パスワードをbcryptでハッシュ化
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// ユーザーをデータベースに挿入
	user := &User{
		Email:        req.Email,
		Username:     req.Username,
		PasswordHash: string(hashedPassword),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err = as.db.QueryRow(
		"INSERT INTO users (email, username, password_hash, created_at, updated_at) VALUES ($1, $2, $3, $4, $5) RETURNING id",
		user.Email, user.Username, user.PasswordHash, user.CreatedAt, user.UpdatedAt,
	).Scan(&user.ID)

	if err != nil {
		// SQLエラーの詳細を確認
		if strings.Contains(err.Error(), "duplicate key value") {
			if strings.Contains(err.Error(), "email") {
				return nil, &AuthError{Code: "EMAIL_ALREADY_EXISTS", Message: "このメールアドレスは既に登録されています"}
			}
			if strings.Contains(err.Error(), "username") {
				return nil, &AuthError{Code: "USERNAME_ALREADY_EXISTS", Message: "このユーザー名は既に使用されています"}
			}
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// LoginUser はユーザーをログインさせます
func (as *AuthService) LoginUser(req *LoginRequest, clientIP string) (*TokenPair, error) {
	// ブルートフォース攻撃対策：最近のログイン失敗をチェック
	if err := as.checkLoginAttempts(req.Email, clientIP); err != nil {
		return nil, err
	}

	// ユーザーを取得
	user, err := as.getUserByEmail(req.Email)
	if err != nil {
		// ログイン失敗を記録
		as.recordLoginAttempt(req.Email, clientIP, false)
		return nil, &AuthError{Code: "INVALID_CREDENTIALS", Message: "メールアドレスまたはパスワードが間違っています"}
	}

	// パスワードを検証
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		// ログイン失敗を記録
		as.recordLoginAttempt(req.Email, clientIP, false)
		return nil, &AuthError{Code: "INVALID_CREDENTIALS", Message: "メールアドレスまたはパスワードが間違っています"}
	}

	// 成功時もログイン試行を記録
	as.recordLoginAttempt(req.Email, clientIP, true)

	// トークンペアを生成
	tokenPair, err := as.tokenManager.GenerateTokenPair(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate tokens: %w", err)
	}

	// リフレッシュトークンをハッシュ化してDBに保存（トークンの無効化管理用）
	tokenHash := as.hashToken(tokenPair.RefreshToken)
	expiresAt := time.Now().Add(as.config.RefreshTokenDuration)
	_, err = as.db.Exec(
		"INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)",
		user.ID, tokenHash, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store refresh token: %w", err)
	}

	return tokenPair, nil
}

// RefreshAccessToken はリフレッシュトークンを使ってアクセストークンを更新します
func (as *AuthService) RefreshAccessToken(refreshTokenString string) (*TokenPair, error) {
	// リフレッシュトークンを検証
	claims, err := as.tokenManager.VerifyRefreshToken(refreshTokenString)
	if err != nil {
		return nil, &AuthError{Code: "INVALID_REFRESH_TOKEN", Message: "リフレッシュトークンが無効です"}
	}

	// トークンがデータベースで無効化されていないか確認
	tokenHash := as.hashToken(refreshTokenString)
	var isRevoked bool
	err = as.db.QueryRow(
		"SELECT is_revoked FROM refresh_tokens WHERE token_hash = $1 AND expires_at > NOW()",
		tokenHash,
	).Scan(&isRevoked)

	if err == sql.ErrNoRows || isRevoked {
		return nil, &AuthError{Code: "REFRESH_TOKEN_REVOKED", Message: "リフレッシュトークンが無効化されています"}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to check refresh token: %w", err)
	}

	// ユーザーを取得
	user, err := as.getUserByID(claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// 新しいトークンペアを生成
	tokenPair, err := as.tokenManager.GenerateTokenPair(user)
	if err != nil {
		return nil, fmt.Errorf("failed to generate new tokens: %w", err)
	}

	// 古いリフレッシュトークンを無効化
	_, err = as.db.Exec(
		"UPDATE refresh_tokens SET is_revoked = true, revoked_at = NOW() WHERE token_hash = $1",
		tokenHash,
	)
	if err != nil {
		// 警告ログですが、トークンペアは返す
		fmt.Printf("警告: 古いリフレッシュトークンの無効化に失敗しました: %v\n", err)
	}

	// 新しいリフレッシュトークンを保存
	newTokenHash := as.hashToken(tokenPair.RefreshToken)
	expiresAt := time.Now().Add(as.config.RefreshTokenDuration)
	_, err = as.db.Exec(
		"INSERT INTO refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, $2, $3)",
		user.ID, newTokenHash, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to store new refresh token: %w", err)
	}

	return tokenPair, nil
}

// RevokeRefreshToken はリフレッシュトークンを無効化します（ログアウト時など）
func (as *AuthService) RevokeRefreshToken(refreshTokenString string) error {
	tokenHash := as.hashToken(refreshTokenString)
	_, err := as.db.Exec(
		"UPDATE refresh_tokens SET is_revoked = true, revoked_at = NOW() WHERE token_hash = $1",
		tokenHash,
	)
	return err
}

// validateRegistration はユーザー登録リクエストを検証します
func (as *AuthService) validateRegistration(req *RegisterRequest) error {
	// メールアドレスの検証
	if !isValidEmail(req.Email) {
		return &AuthError{Code: "INVALID_EMAIL", Message: "有効なメールアドレスではありません"}
	}

	// ユーザー名の検証（英数字とアンダースコアのみ、3-30文字）
	if len(req.Username) < 3 || len(req.Username) > 30 {
		return &AuthError{Code: "INVALID_USERNAME_LENGTH", Message: "ユーザー名は3～30文字である必要があります"}
	}

	if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(req.Username) {
		return &AuthError{Code: "INVALID_USERNAME_FORMAT", Message: "ユーザー名は英数字とアンダースコアのみ使用できます"}
	}

	// パスワードの検証
	if len(req.Password) < as.config.PasswordMinLength {
		return &AuthError{
			Code:    "PASSWORD_TOO_SHORT",
			Message: fmt.Sprintf("パスワードは%d文字以上である必要があります", as.config.PasswordMinLength),
		}
	}

	// パスワードの複雑性チェック（大文字、小文字、数字を含む）
	if !regexp.MustCompile(`[a-z]`).MatchString(req.Password) ||
		!regexp.MustCompile(`[A-Z]`).MatchString(req.Password) ||
		!regexp.MustCompile(`[0-9]`).MatchString(req.Password) {
		return &AuthError{
			Code:    "PASSWORD_WEAK",
			Message: "パスワードは大文字、小文字、数字を含む必要があります",
		}
	}

	return nil
}

// checkLoginAttempts はブルートフォース攻撃をチェックします
func (as *AuthService) checkLoginAttempts(email, clientIP string) error {
	// 過去N分間の失敗したログイン試行を数える
	var failedAttempts int
	err := as.db.QueryRow(
		"SELECT COUNT(*) FROM login_attempts WHERE email = $1 AND ip_address = $2 AND success = false AND attempted_at > NOW() - INTERVAL '1 minute' * $3",
		email, clientIP, int(as.config.LockoutDuration.Minutes()),
	).Scan(&failedAttempts)

	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to check login attempts: %w", err)
	}

	if failedAttempts >= as.config.MaxLoginAttempts {
		return &AuthError{
			Code:    "TOO_MANY_ATTEMPTS",
			Message: fmt.Sprintf("ログイン試行回数が多すぎます。%d分後に再度お試しください", int(as.config.LockoutDuration.Minutes())),
		}
	}

	return nil
}

// recordLoginAttempt はログイン試行を記録します
func (as *AuthService) recordLoginAttempt(email, clientIP string, success bool) {
	_, err := as.db.Exec(
		"INSERT INTO login_attempts (email, ip_address, success, attempted_at) VALUES ($1, $2, $3, $4)",
		email, clientIP, success, time.Now(),
	)
	if err != nil {
		fmt.Printf("警告: ログイン試行の記録に失敗しました: %v\n", err)
	}
}

// getUserByEmail はメールアドレスからユーザーを取得します
func (as *AuthService) getUserByEmail(email string) (*User, error) {
	user := &User{}
	err := as.db.QueryRow(
		"SELECT id, email, username, password_hash, created_at, updated_at FROM users WHERE email = $1 AND is_deleted = false",
		email,
	).Scan(&user.ID, &user.Email, &user.Username, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// getUserByID はユーザーIDからユーザーを取得します
func (as *AuthService) getUserByID(userID int) (*User, error) {
	user := &User{}
	err := as.db.QueryRow(
		"SELECT id, email, username, password_hash, created_at, updated_at FROM users WHERE id = $1 AND is_deleted = false",
		userID,
	).Scan(&user.ID, &user.Email, &user.Username, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// hashToken はトークンをハッシュ化します（DBには生トークンを保存しない）
func (as *AuthService) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// isValidEmail はメールアドレスの形式をチェックします
func isValidEmail(email string) bool {
	// シンプルなメール検証
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(email) && len(email) <= 255
}

// ExtractIPFromRequest はリクエストからクライアントのIPアドレスを抽出します
func ExtractIPFromRequest(remoteAddr string) string {
	// X-Forwarded-Forヘッダーが設定されている場合はそれを使用（プロキシ経由の場合）
	// 通常はこれはミドルウェアで処理し、コンテキストに格納するべき

	// remoteAddrからホストポート部分を抽出
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// ポート番号がない場合
		return remoteAddr
	}
	return host
}
