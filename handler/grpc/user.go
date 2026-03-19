package handlergrpc

import (
	"context"
	"errors"  // 追加
	"strings" // 追加

	repository "auxilia/domain/interface"
	"auxilia/domain/model"
	"auxilia/pb"

	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm" // GORMのエラー判定用
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
	// バリデーション
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "ユーザー名を入力してください")
	}
	if utf8.RuneCountInString(req.Name) > 16 {
			return nil, status.Error(codes.OutOfRange, "ユーザー名は16文字以内で入力してください")
	}
	if len(req.Password) < 6 {
		return nil, status.Error(codes.FailedPrecondition, "パスワードが短すぎます")
	}

	// 1. パスワードハッシュ化
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Error(codes.Internal, "パスワードの処理に失敗しました")
	}

	// 2. モデル作成
	newUser := &model.User{
		ID:         uuid.New(),
		Name:       req.Name,
		Hash:       string(hashedPassword),
		Story:      1,
		NumWins:    0,
		NumBattles: 0,
		Rate:       0,
		HomeCharacterID: 0,
		Deck1:      -1,
		Deck2:      -1,
		Deck3:      -1,
	}

	// 3. DB保存とエラー判定
	if err := h.repo.Create(ctx, newUser); err != nil {
		var mysqlErr *mysql.MySQLError
		// Check if the error is a MySQL error and specifically code 1062
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return nil, status.Error(codes.AlreadyExists, "そのユーザー名は既に使用されています")
		}

		// Fallback for other drivers or general errors
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "1062") {
			return nil, status.Error(codes.AlreadyExists, "そのユーザー名は既に使用されています")
		}
		return nil, status.Errorf(codes.Internal, "ユーザー作成失敗: %v", err)
	}

	return h.toPBResponse(newUser), nil
}

// Login: ユーザー名とパスワードで認証
func (h *UserHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.UserResponse, error) {
	if req.Name == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "名前とパスワードを入力してください")
	}

	user, err := h.repo.FindByName(ctx, req.Name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, status.Error(codes.NotFound, "ユーザーが見つかりません")
		}
		return nil, status.Error(codes.Internal, "ログイン処理中にエラーが発生しました")
	}

	// パスワード照合
	err = bcrypt.CompareHashAndPassword([]byte(user.Hash), []byte(req.Password))
	if err != nil {
		// 意図的に NotFound ではなく Unauthenticated を返す
		return nil, status.Error(codes.Unauthenticated, "ユーザー名またはパスワードが正しくありません")
	}

	return h.toPBResponse(user), nil
}

// UpdateUser: 情報の更新
func (h *UserHandler) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UserResponse, error) {
	uid, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "無効なID形式です")
	}

	user, err := h.repo.FindByID(ctx, uid)
	if err != nil {
		return nil, status.Error(codes.NotFound, "ユーザーが存在しません")
	}

	// 更新があった項目のみ上書き
	if req.Name != "" {
		user.Name = req.Name
	}

	if req.Password != "" {
		if len(req.Password) < 6 {
			return nil, status.Error(codes.InvalidArgument, "パスワードは6文字以上必要です")
		}
		hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, status.Error(codes.Internal, "パスワードの更新に失敗しました")
		}
		user.Hash = string(hashed)
	}

	if req.Story > 0 { user.Story = int(req.Story) }
	if req.NumWins >= 0 { user.NumWins = int(req.NumWins) }
	if req.NumBattles >= 0 { user.NumBattles = int(req.NumBattles) }
	if req.Rate >= 0 { user.Rate = int(req.Rate) }
	if req.HomeCharacterId >= 0 { user.HomeCharacterID = int(req.HomeCharacterId) }
	if req.Deck1 >= -1 { user.Deck1 = int(req.Deck1) }
	if req.Deck2 >= -1 { user.Deck2 = int(req.Deck2) }
	if req.Deck3 >= -1 { user.Deck3 = int(req.Deck3) }

	if err := h.repo.Update(ctx, user); err != nil {
		// 名前を更新した際の一意制約チェック
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, status.Error(codes.AlreadyExists, "変更先のユーザー名は既に使用されています")
		}
		return nil, status.Errorf(codes.Internal, "更新失敗: %v", err)
	}

	return h.toPBResponse(user), nil
}

// DeleteUser: 削除
func (h *UserHandler) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	uid, err := uuid.Parse(req.Id)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "無効なID形式です")
	}

	if err := h.repo.Delete(ctx, uid); err != nil {
		return nil, status.Error(codes.Internal, "ユーザーの削除に失敗しました")
	}

	return &pb.DeleteUserResponse{Success: true}, nil
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

// 内部補助メソッド: model.User -> pb.UserResponse の変換
func (h *UserHandler) toPBResponse(u *model.User) *pb.UserResponse {
	return &pb.UserResponse{
		Id:    u.ID.String(),
		Name:  u.Name,
		Story: int32(u.Story),
		NumWins: int32(u.NumWins),
		NumBattles: int32(u.NumBattles),
		Rate: int32(u.Rate),
		HomeCharacterId: int32(u.HomeCharacterID),
		Deck1: int32(u.Deck1),
		Deck2: int32(u.Deck2),
		Deck3: int32(u.Deck3),
	}
}