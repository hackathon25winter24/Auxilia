package handlergrpc

import (
	"context"
	"log"

	"auxilia/domain/model"
	"auxilia/domain/interface"
	"auxilia/pb"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type UserHandler struct {
	pb.UnimplementedUserServiceServer
	repo repository.UserRepository
}

func NewUserHandler(repo repository.UserRepository) *UserHandler {
	return &UserHandler{
		repo: repo,
	}
}

// CreateUser: 新規ユーザー作成
func (h *UserHandler) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.UserResponse, error) {
	user := &model.User{
		Name:  req.Name,
		Hash:  req.Hash,
		Story: int(req.Story),
		Rate:  int(req.Rate),
	}

	if err := h.repo.Create(ctx, user); err != nil {
		log.Printf("Failed to create user: %v", err)
		return nil, status.Errorf(codes.Internal, "failed to create user")
	}

	return h.toPBResponse(user), nil
}

// GetUser: IDでユーザー取得
func (h *UserHandler) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.UserResponse, error) {
	uid, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid uuid format")
	}

	user, err := h.repo.FindByID(ctx, uid)
	if err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	return h.toPBResponse(user), nil
}

// ListUsers: 全ユーザー一覧取得
func (h *UserHandler) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	users, err := h.repo.FindAll(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to fetch users")
	}

	var pbUsers []*pb.UserResponse
	for _, u := range users {
		// スライスの中身を1つずつ変換
		pbUsers = append(pbUsers, h.toPBResponse(&u))
	}

	return &pb.ListUsersResponse{Users: pbUsers}, nil
}

// Login: Hash一致でユーザー取得
func (h *UserHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.UserResponse, error) {
	user, err := h.repo.FindByHash(ctx, req.Hash)
	if err != nil {
		return nil, status.Error(codes.NotFound, "invalid hash or user not found")
	}

	return h.toPBResponse(user), nil
}

// UpdateUser: ユーザー情報の更新 (必要に応じてサービスに追加してください)
func (h *UserHandler) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UserResponse, error) {
	uid, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid uuid format")
	}

	// 1. まず現在のデータを取得（存在確認）
	user, err := h.repo.FindByID(ctx, uid)
	if err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	// 2. リクエストの内容で更新
	user.Name = req.Name
	user.Hash = req.Hash
	user.Story = int(req.Story)
	user.Rate = int(req.Rate)

	// 3. DBに保存
	if err := h.repo.Update(ctx, user); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update user: %v", err)
	}

	return h.toPBResponse(user), nil
}

// FindByName に関して: 
// もし proto 側で "rpc GetUserByName(NameRequest) returns (UserResponse)" 
// のようなメソッドを定義している場合の実装例です。
func (h *UserHandler) GetUserByName(ctx context.Context, req *pb.NameRequest) (*pb.UserResponse, error) {
	user, err := h.repo.FindByName(ctx, req.Name)
	if err != nil {
		return nil, status.Error(codes.NotFound, "user not found by name")
	}

	return h.toPBResponse(user), nil
}

// 内部補助メソッド: model.User -> pb.UserResponse の変換
func (h *UserHandler) toPBResponse(u *model.User) *pb.UserResponse {
	return &pb.UserResponse{
		Id:    u.ID.String(),
		Name:  u.Name,
		Hash:  u.Hash,
		Story: int32(u.Story),
		Rate:  int32(u.Rate),
	}
}