package handlergrpc

import (
	"context"
	"fmt"
	"log"

	"auxilia/domain/model"
	"auxilia/pb"
	"auxilia/infrastructure/gorm" // リポジトリをインポート

	"github.com/google/uuid"
    "google.golang.org/grpc/codes"  // これが必要
    "google.golang.org/grpc/status" // これが足りていなかったもの
)

type Server struct {
	pb.UnimplementedUserServiceServer
	// DB (*gorm.DB) ではなく、リポジトリを保持する
	userRepo *gorm.UserRepository
}

func NewServer(repo *gorm.UserRepository) *Server {
	return &Server{userRepo: repo}
}

func (s *Server) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.UserResponse, error) {
	// 1. 翻訳：gRPCのリクエストをドメインモデルに変換
	user := &model.User{
		Hash:  req.Hash,
		Story: int(req.Story),
		Rate:  int(req.Rate),
	}

	// 2. 依頼：DB操作はリポジトリに丸投げする
	if err := s.userRepo.Create(user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// 3. 返答：ドメインモデルをgRPCのレスポンス形式に包み直す
	return &pb.UserResponse{
		Id:    user.ID.String(),
		Hash:  user.Hash,
		Story: int32(user.Story),
		Rate:  int32(user.Rate),
	}, nil
}

func (s *Server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.UserResponse, error) {
	uid, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, fmt.Errorf("invalid uuid format: %w", err)
	}

	// リポジトリにデータ取得を依頼
	user, err := s.userRepo.FindByID(uid)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return &pb.UserResponse{
		Id:    user.ID.String(),
		Hash:  user.Hash,
		Story: int32(user.Story),
		Rate:  int32(user.Rate),
	}, nil
}

// ListUsers: 全ユーザー一覧取得
func (s *Server) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	log.Printf("Received ListUsers request: %v", req)
	users, err := s.userRepo.FindAll()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch users: %v", err)
	}

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

// Login: Hash一致でユーザー取得
func (s *Server) Login(ctx context.Context, req *pb.LoginRequest) (*pb.UserResponse, error) {
	user, err := s.userRepo.FindByHash(req.Hash)
	if err != nil {
		// ユーザーが見つからない場合は NotFound を返す
		return nil, status.Error(codes.NotFound, "invalid hash or user not found")
	}

	return &pb.UserResponse{
		Id:    user.ID.String(),
		Hash:  user.Hash,
		Story: int32(user.Story),
		Rate:  int32(user.Rate),
	}, nil
}