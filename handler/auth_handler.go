package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"auxilia/auth"
	"auxilia/config"
)

// AuthHandler はユーザー認証に関するHTTPハンドラーです
type AuthHandler struct {
	authService *auth.AuthService
}

// NewAuthHandler は新しいAuthHandlerを作成します
func NewAuthHandler(cfg *config.Config, tm *auth.TokenManager) *AuthHandler {
	return &AuthHandler{
		authService: auth.NewAuthService(cfg, tm),
	}
}

// RegisterHandler はユーザー登録エンドポイント: POST /auth/register
func (h *AuthHandler) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"code":"METHOD_NOT_ALLOWED","message":"POSTメソッドのみ使用可能です"}`, http.StatusMethodNotAllowed)
		return
	}

	// リクエストボディを読み込む
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"code":"BAD_REQUEST","message":"リクエストボディの読み込みに失敗しました"}`, http.StatusBadRequest)
		return
	}

	// JSONをパース
	var req auth.RegisterRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"code":"INVALID_JSON","message":"JSONの形式が正しくありません"}`, http.StatusBadRequest)
		return
	}

	// ユーザー登録
	user, err := h.authService.RegisterUser(&req)
	if authErr, ok := err.(*auth.AuthError); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(authErr)
		return
	}
	if err != nil {
		http.Error(w, `{"code":"INTERNAL_ERROR","message":"ユーザー登録に失敗しました"}`, http.StatusInternalServerError)
		return
	}

	// 成功レスポンス
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":       user.ID,
		"email":    user.Email,
		"username": user.Username,
	})
}

// LoginHandler はログインエンドポイント: POST /auth/login
func (h *AuthHandler) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"code":"METHOD_NOT_ALLOWED","message":"POSTメソッドのみ使用可能です"}`, http.StatusMethodNotAllowed)
		return
	}

	// クライアントのIPアドレスを取得
	clientIP := auth.ExtractIPFromRequest(r.RemoteAddr)

	// リクエストボディを読み込む
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"code":"BAD_REQUEST","message":"リクエストボディの読み込みに失敗しました"}`, http.StatusBadRequest)
		return
	}

	// JSONをパース
	var req auth.LoginRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"code":"INVALID_JSON","message":"JSONの形式が正しくありません"}`, http.StatusBadRequest)
		return
	}

	// ログイン処理
	tokenPair, err := h.authService.LoginUser(&req, clientIP)
	if authErr, ok := err.(*auth.AuthError); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(authErr)
		return
	}
	if err != nil {
		http.Error(w, `{"code":"INTERNAL_ERROR","message":"ログインに失敗しました"}`, http.StatusInternalServerError)
		return
	}

	// 成功レスポンス
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tokenPair)
}

// RefreshTokenHandler はトークン更新エンドポイント: POST /auth/refresh
func (h *AuthHandler) RefreshTokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"code":"METHOD_NOT_ALLOWED","message":"POSTメソッドのみ使用可能です"}`, http.StatusMethodNotAllowed)
		return
	}

	// リクエストボディを読み込む
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"code":"BAD_REQUEST","message":"リクエストボディの読み込みに失敗しました"}`, http.StatusBadRequest)
		return
	}

	// JSONをパース
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"code":"INVALID_JSON","message":"JSONの形式が正しくありません"}`, http.StatusBadRequest)
		return
	}

	// トークンを更新
	tokenPair, err := h.authService.RefreshAccessToken(req.RefreshToken)
	if authErr, ok := err.(*auth.AuthError); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(authErr)
		return
	}
	if err != nil {
		http.Error(w, `{"code":"INTERNAL_ERROR","message":"トークン更新に失敗しました"}`, http.StatusInternalServerError)
		return
	}

	// 成功レスポンス
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tokenPair)
}

// LogoutHandler はログアウトエンドポイント: POST /auth/logout
func (h *AuthHandler) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"code":"METHOD_NOT_ALLOWED","message":"POSTメソッドのみ使用可能です"}`, http.StatusMethodNotAllowed)
		return
	}

	// リクエストボディを読み込む
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"code":"BAD_REQUEST","message":"リクエストボディの読み込みに失敗しました"}`, http.StatusBadRequest)
		return
	}

	// JSONをパース
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"code":"INVALID_JSON","message":"JSONの形式が正しくありません"}`, http.StatusBadRequest)
		return
	}

	// リフレッシュトークンを無効化
	if err := h.authService.RevokeRefreshToken(req.RefreshToken); err != nil {
		http.Error(w, `{"code":"INTERNAL_ERROR","message":"ログアウトに失敗しました"}`, http.StatusInternalServerError)
		return
	}

	// 成功レスポンス
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "ログアウトしました",
	})
}

// ProtectedExample はトークン認証を必須とする保護されたエンドポイントの例です
// ルーターでAuthMiddlewareを適用してください
func (h *AuthHandler) ProtectedExample(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.GetUserIDFromContext(r)
	if err != nil {
		http.Error(w, `{"code":"UNAUTHORIZED","message":"認証情報が見つかりません"}`, http.StatusUnauthorized)
		return
	}

	userEmail, _ := auth.GetUserEmailFromContext(r)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "この情報は認証ユーザーのみ閲覧可能です",
		"user_id": userID,
		"email":   userEmail,
	})
}
