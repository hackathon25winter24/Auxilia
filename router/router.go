package router

import (
	"net/http"

	"auxilia/auth"
	"auxilia/config"
	"auxilia/handler"
)

// SetupRoutes はすべてのエンドポイントをセットアップします
func SetupRoutes(cfg *config.Config, tm *auth.TokenManager) *http.ServeMux {
	mux := http.NewServeMux()

	// ハンドラーの作成
	authHandler := handler.NewAuthHandler(cfg, tm)

	// 認証関連のエンドポイント（保護なし）
	mux.HandleFunc("/auth/register", authHandler.RegisterHandler)
	mux.HandleFunc("/auth/login", authHandler.LoginHandler)
	mux.HandleFunc("/auth/refresh", authHandler.RefreshTokenHandler)
	mux.HandleFunc("/auth/logout", authHandler.LogoutHandler)

	// 保護されたエンドポイントの例
	// AuthMiddlewareでラップする
	protectedMux := http.NewServeMux()
	protectedMux.HandleFunc("/protected/example", authHandler.ProtectedExample)

	// 認証ミドルウェアを適用した保護されたハンドラー
	authenticatedHandler := auth.AuthMiddleware(tm)(protectedMux)
	mux.Handle("/protected/", authenticatedHandler)

	// ヘルスチェックエンドポイント
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// CORS と ロギング ミドルウェアを適用
	return mux
}

// WrapWithMiddleware はミドルウェアをハンドラーに適用します
func WrapWithMiddleware(handler http.Handler) http.Handler {
	// ロギング → CORS → ハンドラー の順で適用
	handler = auth.LoggingMiddleware(handler)
	handler = auth.CORSMiddleware(handler)
	return handler
}
