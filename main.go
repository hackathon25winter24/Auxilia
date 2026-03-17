package main

import (
	"log"
	"net/http"
	"strings"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

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
	db.AutoMigrate(&model.User{}, &model.RoomMatch{}, &model.Room{}, &model.GameData{}, &model.UniqueCharacter{}, &model.CharacterCondition{}, &model.AttackInfo{})

	// room_matches の is_private を is_gaming に移行し、既存値はすべて false に揃える
	migratedFromIsPrivate := false
	hasIsPrivate := db.Migrator().HasColumn(&model.RoomMatch{}, "is_private")
	hasIsGaming := db.Migrator().HasColumn(&model.RoomMatch{}, "is_gaming")

	if hasIsPrivate && !hasIsGaming {
		if err := db.Migrator().RenameColumn(&model.RoomMatch{}, "is_private", "is_gaming"); err != nil {
			log.Fatalf("failed to rename column is_private -> is_gaming: %v", err)
		}
		migratedFromIsPrivate = true
	}

	if hasIsPrivate && hasIsGaming {
		if err := db.Migrator().DropColumn(&model.RoomMatch{}, "is_private"); err != nil {
			log.Printf("warning: failed to drop legacy column is_private: %v", err)
		}
		migratedFromIsPrivate = true
	}

	if migratedFromIsPrivate {
		if err := db.Model(&model.RoomMatch{}).Update("is_gaming", false).Error; err != nil {
			log.Fatalf("failed to reset is_gaming values: %v", err)
		}
	}

	// game_data の is1_p_turn を is_1p_turn に移行
	hasOldIs1PTurn := db.Migrator().HasColumn(&model.GameData{}, "is1_p_turn")
	hasNewIs1PTurn := db.Migrator().HasColumn(&model.GameData{}, "is_1p_turn")

	if hasOldIs1PTurn && !hasNewIs1PTurn {
		if err := db.Migrator().RenameColumn(&model.GameData{}, "is1_p_turn", "is_1p_turn"); err != nil {
			log.Fatalf("failed to rename column is1_p_turn -> is_1p_turn: %v", err)
		}
	}

	if hasOldIs1PTurn && hasNewIs1PTurn {
		if err := db.Migrator().DropColumn(&model.GameData{}, "is1_p_turn"); err != nil {
			log.Printf("warning: failed to drop legacy column is1_p_turn: %v", err)
		}
	}

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

	// Gameサービスの設定を追加
	gameRepo := gorm.NewBattleRepository(db)
	gameHandler := handlergrpc.NewBattleHandler(gameRepo)
	pb.RegisterBattleServiceServer(s, gameHandler)

	// Roomサービスの設定を追加
	roomRepo := gorm.NewRoomRepository(db)
	roomHandler := handlergrpc.NewRoomHandler(roomRepo, gameRepo)
	pb.RegisterRoomServiceServer(s, roomHandler)

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
