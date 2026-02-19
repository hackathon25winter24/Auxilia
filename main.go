package main

import (
	"log"
	"net/http"
	"strings"

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

	// 1. gRPCサーバーの作成と設定
	s := grpc.NewServer()
	userRepo := gorm.NewUserRepository(db)
	userHandler := handlergrpc.NewServer(userRepo)
	pb.RegisterUserServiceServer(s, userHandler)
	reflection.Register(s)

	// 2. gRPC-Webハンドラーの作成
	// 通常、httpserver.NewHandler(s) 内で grpcweb.WrapServer(s) が呼ばれている前提です
	httpHandler := httpserver.NewHandler(s)

	// 3. ハイブリッド・ハンドラー
	// 自前で複雑な条件分岐をせず、Content-Typeのみで「生のgRPC」か「それ以外」かを判定します
	rootHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")

		// 純粋な gRPC (HTTP/2) の場合
		if r.ProtoMajor == 2 && strings.HasPrefix(contentType, "application/grpc") {
			s.ServeHTTP(w, r)
			return
		}

		// それ以外 (gRPC-Web, ブラウザのGET/POST, HTTP/1.1) はすべて httpHandler へ
		// httpHandler (grpcweb) は内部で gRPC-Web かどうかを判別して処理してくれます
		httpHandler.ServeHTTP(w, r)
	})

	// 4. H2C (HTTP/2 Cleartext) サーバーの設定
	// Traefikとの相性を高めるため、標準的な設定にします
	h2s := &http2.Server{}
	httpServer := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: h2c.NewHandler(rootHandler, h2s),
	}

	log.Printf("Server listening at %s (Dual Mode: gRPC & gRPC-Web)", cfg.AppPort)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}