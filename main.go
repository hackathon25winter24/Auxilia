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
  rootHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // Content-Type を取得
    ct := r.Header.Get("Content-Type")

    // gRPC-Web (HTTP/1.1 or Content-Type) の場合は httpHandler へ
    // ブラウザからの GET リクエストなどもこちらに含まれます
    if r.ProtoMajor == 1 || ct == "application/grpc-web" || ct == "application/grpc-web+proto" {
      httpHandler.ServeHTTP(w, r)
      return
    }

    // それ以外（生の gRPC / HTTP/2）はすべて gRPC サーバーへ
    s.ServeHTTP(w, r)
  })

  // 4. HTTP/2 (h2c) を有効にしたサーバー設定
  h2s := &http2.Server{}
  httpServer := &http.Server{
    Addr:    ":" + cfg.AppPort,
    Handler: h2c.NewHandler(rootHandler, h2s),
    // タイムアウト対策として、コネクション維持の設定を明示的に入れても良いです
  }

  log.Printf("Server listening at %s (supporting both gRPC and gRPC-Web)", cfg.AppPort)
  if err := httpServer.ListenAndServe(); err != nil {
    log.Fatalf("failed to serve: %v", err)
  }
}