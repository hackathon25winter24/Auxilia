package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// AuthMiddleware はHTTPリクエストのアクセストークンを検証します
// 保護されたエンドポイントの前に使用してください
func AuthMiddleware(tm *TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Authorizationヘッダーから"Bearer <token>"を抽出
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"code":"MISSING_TOKEN","message":"認証トークンが見つかりません"}`, http.StatusUnauthorized)
				return
			}

			// "Bearer <token>"形式を確認
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"code":"INVALID_TOKEN_FORMAT","message":"トークン形式が正しくありません"}`, http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			// アクセストークンを検証
			claims, err := tm.VerifyAccessToken(tokenString)
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"code":"INVALID_TOKEN","message":"トークンが無効です: %s"}`, err.Error()), http.StatusUnauthorized)
				return
			}

			// クレーム情報をコンテキストに追加
			ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
			ctx = context.WithValue(ctx, "user_email", claims.Email)
			ctx = context.WithValue(ctx, "user_name", claims.Username)

			// 次のハンドラーにリクエストを渡す
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserIDFromContext はコンテキストからユーザーIDを取得します
func GetUserIDFromContext(r *http.Request) (int, error) {
	userID, ok := r.Context().Value("user_id").(int)
	if !ok {
		return 0, fmt.Errorf("user_id not found in context")
	}
	return userID, nil
}

// GetUserEmailFromContext はコンテキストからユーザーメールアドレスを取得します
func GetUserEmailFromContext(r *http.Request) (string, error) {
	email, ok := r.Context().Value("user_email").(string)
	if !ok {
		return "", fmt.Errorf("user_email not found in context")
	}
	return email, nil
}

// CORSMiddleware はCORS（クロスオリジンリソース共有）を処理します
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 本番環境ではUnityクライアントのURLを具体的に指定してください
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// プリフライトリクエスト（OPTIONS）への対応
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// LoggingMiddleware はHTTPリクエストをログに記録します
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("[%s] %s %s\n", r.Method, r.RequestURI, r.RemoteAddr)
		next.ServeHTTP(w, r)
	})
}
