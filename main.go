package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"auxilia/auth"
	"auxilia/config"
	"auxilia/db"
	"auxilia/router"
)

func main() {
	// 1. 設定を読み込む
	cfg := config.LoadConfig()
	fmt.Println("設定を読み込みました")

	// 2. データベースに接続
	_, err := db.InitDB(cfg.GetDSN())
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.CloseDB()
	fmt.Println("データベースに接続しました")

	// 3. テーブルを作成
	if err := db.CreateTables(); err != nil {
		log.Fatalf("Failed to create tables: %v", err)
	}
	fmt.Println("テーブルを初期化しました")

	// 4. TokenManagerを作成（JWT認証用）
	tokenManager := auth.NewTokenManager(
		cfg.JWTSecretKey,
		cfg.AccessTokenDuration,
		cfg.RefreshTokenDuration,
	)

	// 5. ルートをセットアップ
	mux := router.SetupRoutes(cfg, tokenManager)
	handler := router.WrapWithMiddleware(mux)

	// 6. HTTPサーバーを起動
	server := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: handler,
	}

	fmt.Printf("Server starting at http://localhost:%s\n", cfg.Port)
	fmt.Println("利用可能なエンドポイント:")
	fmt.Println("  POST /auth/register      - ユーザー登録")
	fmt.Println("  POST /auth/login         - ログイン")
	fmt.Println("  POST /auth/refresh       - トークン更新")
	fmt.Println("  POST /auth/logout        - ログアウト")
	fmt.Println("  GET  /health             - ヘルスチェック")
	fmt.Println("  GET  /protected/example  - 保護されたエンドポイント例")

	// Graceful shutdown用のシグナルハンドラー
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		fmt.Println("\nサーバーをシャットダウンしています...")
		if err := server.Close(); err != nil {
			log.Fatalf("Failed to close server: %v", err)
		}
	}()

	// サーバー起動
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	fmt.Println("サーバーを停止しました")
}
