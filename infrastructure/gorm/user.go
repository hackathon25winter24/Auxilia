package gorm

import (
	"auxilia/domain/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"context"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create: ユーザーを保存する
func (r *UserRepository) Create(ctx context.Context,user *model.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

// FindByID: IDでユーザーを探す
func (r *UserRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByHash: ハッシュ値でユーザーを探す（ログイン用）
func (r *UserRepository) FindByHash(ctx context.Context, hash string) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).Where("hash = ?", hash).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindAll: 全ユーザーを取得する
func (r *UserRepository) FindAll(ctx context.Context) ([]model.User, error) {
	var users []model.User
	if err := r.db.WithContext(ctx).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// FindByName: 名前でユーザーを検索する実装を追加
func (r *UserRepository) FindByName(ctx context.Context, name string) (*model.User, error) {
	var user model.User
	// Nameカラムで一致する最初のレコードを取得
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// Update: ユーザー情報の更新
func (r *UserRepository) Update(ctx context.Context, user *model.User) error {
	// GORMのSaveメソッドはIDがあればUpdate、なければCreateとして機能します
	return r.db.WithContext(ctx).Save(user).Error
}