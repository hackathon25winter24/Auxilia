package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"gorm.io/driver/mysql" // MariaDB用
	"gorm.io/gorm"

	// TODO: go.mod のモジュール名に合わせて書き換えてください
	"auxilia/pb"
)

// --- モデル定義 ---

type User struct {
	// MariaDBには専用のUUID型がないため、VARCHAR(36)として保存します
	ID    uuid.UUID `gorm:"type:char(36);primaryKey" json:"id"`
	Hash  string    `json:"hash"`
	Story int       `json:"story"`
	Rate  int       `json:"rate"`
}

// データを保存する直前にUUIDを自動生成するフック
func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return
}

// --- gRPC サーバー実装 ---

type server struct {
	pb.UnimplementedUserServiceServer
	db *gorm.DB
}

func (s *server) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.UserResponse, error) {
	user := User{
		Hash:  req.Hash,
		Story: int(req.Story),
		Rate:  int(req.Rate),
	}

	if err := s.db.Create(&user).Error; err != nil {
		return nil, err
	}

	return &pb.UserResponse{
		Id:    user.ID.String(),
		Hash:  user.Hash,
		Story: int32(user.Story),
		Rate:  int32(user.Rate),
	}, nil
}

func (s *server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.UserResponse, error) {
	uid, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, err
	}

	var user User
	if err := s.db.First(&user, "id = ?", uid).Error; err != nil {
		return nil, err
	}

	return &pb.UserResponse{
		Id:    user.ID.String(),
		Hash:  user.Hash,
		Story: int32(user.Story),
		Rate:  int32(user.Rate),
	}, nil
}

func (s *server) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
    var users []User
    // DBから全件取得
    if err := s.db.Find(&users).Error; err != nil {
        return nil, fmt.Errorf("failed to fetch users: %v", err)
    }

    // gRPC用のリストに詰め替える
    var pbUsers []*pb.UserResponse
    for _, u := range users {
        pbUsers = append(pbUsers, &pb.UserResponse{
            Id:    u.ID.String(),
            Hash:  u.Hash,
            Story: int32(u.Story),
            Rate:  int32(u.Rate),
        })
    }

    return &pb.ListUsersResponse{Users: pbUsers}, nil
}


// --- 他のメソッド（CreateUserなど）の下に追加 ---

func (s *server) Login(ctx context.Context, req *pb.LoginRequest) (*pb.UserResponse, error) {
	var user User
	
	// DBからHashが一致する最初の1件を取得
	// 見つからない場合はエラーが返ります
	if err := s.db.Where("hash = ?", req.Hash).First(&user).Error; err != nil {
		log.Printf("Login failed: user not found for hash %s", req.Hash)
		return nil, fmt.Errorf("invalid hash or user not found")
	}

	log.Printf("Login successful: %s (ID: %s)", user.Hash, user.ID)

	return &pb.UserResponse{
		Id:    user.ID.String(),
		Hash:  user.Hash,
		Story: int32(user.Story),
		Rate:  int32(user.Rate),
	}, nil
}


// --- メイン処理 ---

func main() {
	// .envがあれば読み込む（ローカル用）、なければシステム環境変数を参照（NeoShowcase用）
	_ = godotenv.Load()

// 1. 環境変数の取得（NeoShowcaseのKeyに厳密に合わせる）
dbHost := os.Getenv("NS_MARIADB_HOSTNAME")
dbUser := os.Getenv("NS_MARIADB_USER")
dbPass := os.Getenv("NS_MARIADB_PASSWORD")
dbName := os.Getenv("NS_MARIADB_DATABASE")
dbPort := os.Getenv("NS_MARIADB_PORT")

// 【重要】デバッグログ：これを確認すれば原因がわかります
log.Printf("DEBUG: Host=[%s] Port=[%s] User=[%s] DB=[%s]", dbHost, dbPort, dbUser, dbName)

// 値が空の場合は、その場でプログラムを止める（:0 で進ませないため）
if dbHost == "" || dbPort == "" {
    log.Fatal("CRITICAL ERROR: 環境変数が読み込めていません。NeoShowcaseの設定を確認してください。")
}

// 2. MariaDB (MySQL互換) 用の DSN 構築
dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
    dbUser, dbPass, dbHost, dbPort, dbName)

	// 3. DB接続
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to MariaDB: %v", err)
	}

	// テーブル自動作成
	if err := db.AutoMigrate(&User{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Println("Database connection and migration successful.")

	// 4. gRPC サーバーの起動準備
	// アプリ自体のポート（これは通常 PORT または 8080）
	appPort := os.Getenv("PORT")
	if appPort == "" {
   		appPort = "8080"
	}
// 1. gRPCサーバーの作成
s := grpc.NewServer()
pb.RegisterUserServiceServer(s, &server{db: db})
reflection.Register(s)

// 2. gRPC-Web でラップする (CORS 許可設定を追加)
wrappedServer := grpcweb.WrapServer(s, 
    grpcweb.WithOriginFunc(func(origin string) bool {
        // すべてのオリジンからの接続を許可（開発中はこれが一番確実です）
        return true 
    }),
)

// 3. HTTPハンドラー
handler := func(resp http.ResponseWriter, req *http.Request) {
    // gRPC-Web のリクエストなら処理
    if wrappedServer.IsGrpcWebRequest(req) {
        wrappedServer.ServeHTTP(resp, req)
        return
    }
    // 通常の gRPC も通す
    s.ServeHTTP(resp, req)
}

// 4. HTTP サーバー起動
httpServer := &http.Server{
    Addr:    ":" + appPort,
    Handler: http.HandlerFunc(handler),
}

log.Printf("Server with gRPC-Web listening at %s", appPort) // ログが変わるはずです
if err := httpServer.ListenAndServe(); err != nil {
    log.Fatalf("failed to serve: %v", err)
}
}