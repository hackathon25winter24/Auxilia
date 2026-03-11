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
	"auxilia/domain/model" // 追加
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

	// ★ 修正点1: 必要な全モデルをマイグレーション対象に追加
	// （RoomMatch を忘れるとテーブルがなくてクエリが失敗する）
	db.AutoMigrate(&model.User{}, &model.RoomMatch{}, &model.GameData{}, &model.UniqueCharacter{}, &model.CharacterCondition{})

	// gRPCサーバーの作成
	s := grpc.NewServer()

	// Userサービスの設定
	userRepo := gorm.NewUserRepository(db)
	userHandler := handlergrpc.NewUserHandler(userRepo)
	pb.RegisterUserServiceServer(s, userHandler)

	// RoomMatchサービスの設定を追加
	roomMatchRepo := gorm.NewRoomMatchRepository(db)
	roomMatchHandler := handlergrpc.NewRoomMatchServer(roomMatchRepo)
	pb.RegisterRoomMatchServiceServer(s, roomMatchHandler)

	// Roomサービスの設定を追加
	roomRepo := gorm.NewRoomRepository(db)
	roomHandler := handlergrpc.NewRoomHandler(roomRepo)
	pb.RegisterRoomServiceServer(s, roomHandler)

	// ★ 修正点2: Gameサービスの設定を追加
	gameRepo := gorm.NewGameRepository(db)
	gameHandler := handlergrpc.NewGameHandler(gameRepo)
	pb.RegisterGameServiceServer(s, gameHandler)

	reflection.Register(s)

	// --- 以下、既存のハイブリッドサーバー設定（変更なし） ---
	httpHandler := httpserver.NewHandler(s)

	rootHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if r.ProtoMajor == 2 && strings.HasPrefix(contentType, "application/grpc") {
			s.ServeHTTP(w, r)
			return
		}
		httpHandler.ServeHTTP(w, r)
	})

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
