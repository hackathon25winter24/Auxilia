package main

import (
	"log"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"golang.org/x/net/http2"
  "golang.org/x/net/http2/h2c"

	cfgpkg "auxilia/config"
	handlergrpc "auxilia/handler/grpc"
	httpserver "auxilia/handler/http"
	gorm "auxilia/infrastructure/gorm"
	"auxilia/pb"
)

func main() {
	cfg, err := cfgpkg.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db, err := gorm.NewGormDB(cfg)
	if err != nil {
		log.Fatalf("failed to connect to DB: %v", err)
	}

	// gRPCサーバーの作成
  s := grpc.NewServer()
  // リポジトリを初期化 (db は *gorm.DB)
  userRepo := gorm.NewUserRepository(db)
  // サーバーハンドラをリポジトリを使って初期化
  userHandler := handlergrpc.NewServer(userRepo)
  // 4. サービスを登録
  pb.RegisterUserServiceServer(s, userHandler)
  // 5. リフレクションを登録
  reflection.Register(s)

	// 2. gRPC-Webハンドラーの作成
	httpHandler := httpserver.NewHandler(s)

	// 3. リクエストの種類に応じて振り分けるハンドラーを定義
	// gRPC通信（grpcurlなど）と gRPC-Web通信（ブラウザ）を1つのポートで受ける
	rootHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// gRPC-Web または 通常のHTTPリクエストの場合
		if r.ProtoMajor == 1 || r.Header.Get("Content-Type") == "application/grpc-web" {
			httpHandler.ServeHTTP(w, r)
			return
		}
		// 生のgRPCリクエスト（HTTP/2）の場合
		s.ServeHTTP(w, r)
	})

	// 4. HTTP/2を有効にしたサーバー設定（h2c = HTTP/2 without TLS）
	// grpcurlは通常HTTP/2で通信するため、これが必要です
	h2s := &http2.Server{}
	httpServer := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: h2c.NewHandler(rootHandler, h2s),
	}

	log.Printf("Server listening at %s (supporting both gRPC and gRPC-Web)", cfg.AppPort)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}