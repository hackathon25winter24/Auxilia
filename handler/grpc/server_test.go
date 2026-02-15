package handlergrpc

import (
	"context"
	"testing"

	"auxilia/domain/model"
	"auxilia/pb"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// テスト用のインメモリ SQLite DB を構築
func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}

	// テーブル作成
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("Failed to migrate test DB: %v", err)
	}

	return db
}

func TestCreateUser(t *testing.T) {
	db := setupTestDB(t)
	srv := NewServer(db)

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
	if resp.Story != 1 {
		t.Errorf("Expected Story 1, got %d", resp.Story)
	}
	if resp.Rate != 5 {
		t.Errorf("Expected Rate 5, got %d", resp.Rate)
	}
	if resp.Id == "" {
		t.Error("Expected non-empty UUID ID")
	}
}

func TestGetUser(t *testing.T) {
	db := setupTestDB(t)
	srv := NewServer(db)

	// Create user
	createResp, err := srv.CreateUser(context.Background(), &pb.CreateUserRequest{
		Hash:  "get_test_hash",
		Story: 2,
		Rate:  4,
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Get user
	getResp, err := srv.GetUser(context.Background(), &pb.GetUserRequest{
		Id: createResp.Id,
	})
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	if getResp.Hash != "get_test_hash" {
		t.Errorf("Expected Hash 'get_test_hash', got '%s'", getResp.Hash)
	}
	if getResp.Id != createResp.Id {
		t.Errorf("Expected ID %s, got %s", createResp.Id, getResp.Id)
	}
}

func TestListUsers(t *testing.T) {
	db := setupTestDB(t)
	srv := NewServer(db)

	// Create multiple users
	for i := 1; i <= 3; i++ {
		_, err := srv.CreateUser(context.Background(), &pb.CreateUserRequest{
			Hash:  "list_test_" + string(rune(i)),
			Story: int32(i),
			Rate:  int32(i * 2),
		})
		if err != nil {
			t.Fatalf("CreateUser %d failed: %v", i, err)
		}
	}

	// List users
	listResp, err := srv.ListUsers(context.Background(), &pb.ListUsersRequest{})
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}

	if len(listResp.Users) != 3 {
		t.Errorf("Expected 3 users, got %d", len(listResp.Users))
	}
}

func TestLogin(t *testing.T) {
	db := setupTestDB(t)
	srv := NewServer(db)

	// Create user
	testHash := "login_test_hash"
	_, err := srv.CreateUser(context.Background(), &pb.CreateUserRequest{
		Hash:  testHash,
		Story: 3,
		Rate:  5,
	})
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Login
	loginResp, err := srv.Login(context.Background(), &pb.LoginRequest{
		Hash: testHash,
	})
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}

	if loginResp.Hash != testHash {
		t.Errorf("Expected Hash '%s', got '%s'", testHash, loginResp.Hash)
	}
}

func TestLoginNotFound(t *testing.T) {
	db := setupTestDB(t)
	srv := NewServer(db)

	_, err := srv.Login(context.Background(), &pb.LoginRequest{
		Hash: "nonexistent_hash",
	})

	if err == nil {
		t.Error("Expected Login to fail for nonexistent hash")
	}
}
