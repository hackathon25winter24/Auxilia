package handlergrpc

import (
	"context"
	"fmt"
	"testing"

	"auxilia/domain/model"
	"auxilia/pb"
	// リポジトリのパッケージを alias 'repo' としてインポート
	repo "auxilia/infrastructure/gorm"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestRepo はテスト用のインメモリDBを作成し、リポジトリを初期化して返します
func setupTestRepo(t *testing.T) *repo.UserRepository {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}

	// テーブル作成
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("Failed to migrate test DB: %v", err)
	}

	// リポジトリを作成
	return repo.NewUserRepository(db)
}

func TestCreateUser(t *testing.T) {
	userRepo := setupTestRepo(t)
	srv := NewServer(userRepo) // リポジトリを渡す

	resp, err := srv.CreateUser(context.Background(), &pb.CreateUserRequest{
		Hash:  "test_hash",
		Story: 1,
		Rate:  5,
	})

	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	if resp.Hash != "test_hash" {
		t.Errorf("Expected Hash 'test_hash', got '%s'", resp.Hash)
	}
}

func TestGetUser(t *testing.T) {
	userRepo := setupTestRepo(t)
	srv := NewServer(userRepo)

	// 事前にユーザーを作成
	createResp, _ := srv.CreateUser(context.Background(), &pb.CreateUserRequest{
		Hash: "get_test", Story: 1, Rate: 1,
	})

	// GetUserの実行
	getResp, err := srv.GetUser(context.Background(), &pb.GetUserRequest{
		Id: createResp.Id,
	})

	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if getResp.Id != createResp.Id {
		t.Errorf("Expected ID %s, got %s", createResp.Id, getResp.Id)
	}
}

func TestListUsers(t *testing.T) {
	userRepo := setupTestRepo(t)
	srv := NewServer(userRepo)

	// 複数作成
	for i := 1; i <= 3; i++ {
		_, err := srv.CreateUser(context.Background(), &pb.CreateUserRequest{
			Hash:  fmt.Sprintf("user_%d", i),
			Story: int32(i),
			Rate:  int32(i),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}