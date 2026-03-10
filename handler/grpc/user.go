package handlergrpc

import (
	"context"

	"auxilia/domain/model"
	"auxilia/domain/interface"
	"auxilia/pb"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"golang.org/x/crypto/bcrypt"
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
  if len(req.Password) < 6 {
    return nil, status.Error(codes.InvalidArgument, "too short password")
	}
	// 1. 生パスワードをハッシュ化する
	// 第2引数の Cost はデフォルト（10）で十分安全です
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
			return nil, status.Error(codes.Internal, "failed to hash password")
	}


	// 2. モデルの作成
	// これまで「Hash」と呼んでいたフィールドに、ハッシュ化したパスワードを入れます
	newUser := &model.User{
			ID:   uuid.New(),
			Name: req.Name,
			Hash: string(hashedPassword), // ここが重要！
			Story: 1,
			NumWins: 0,
			NumBattles: 0,
	}

	// 3. リポジトリ経由でDB保存
	if err := h.repo.Create(ctx, newUser); err != nil {
			return nil, status.Error(codes.AlreadyExists, "user name already exists")
	}

	return h.toPBResponse(newUser), nil
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

func (h *UserHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.UserResponse, error) {
    userName := req.Name
    if userName == "" {
        return nil, status.Error(codes.InvalidArgument, "invalid user name format")
    }

    // 2. 変換した userID を使って検索
    user, err := h.repo.FindByName(ctx, userName)
    if err != nil {
        return nil, status.Error(codes.NotFound, "user not found")
    }

    // 3. パスワード（Hashフィールド）の照合
    err = bcrypt.CompareHashAndPassword([]byte(user.Hash), []byte(req.Password))
    if err != nil {
        return nil, status.Error(codes.Unauthenticated, "invalid password")
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
	user.NumWins = int(req.NumWins)
	user.NumBattles = int(req.NumBattles)

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
		NumWins: int32(u.NumWins),
		NumBattles: int32(u.NumBattles),
	}
}

func (h *UserHandler) DeleteUser(ctx context.Context,req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	uid,err := uuid.Parse(req.Id)
	if err != nil {
        return nil, status.Error(codes.InvalidArgument, "invalid uuid format")
    }

    if err := h.repo.Delete(ctx, uid); err != nil {
        return nil, status.Errorf(codes.Internal, "failed to delete user: %v", err)
    }

    return &pb.DeleteUserResponse{Success: true}, nil
}