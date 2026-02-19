package gorm

import (
	"auxilia/domain/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create: ユーザーを保存する
func (r *UserRepository) Create(user *model.User) error {
	return r.db.Create(user).Error
}

// FindByID: IDでユーザーを探す
func (r *UserRepository) FindByID(id uuid.UUID) (*model.User, error) {
	var user model.User
	if err := r.db.First(&user, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByHash: ハッシュ値でユーザーを探す（ログイン用）
func (r *UserRepository) FindByHash(hash string) (*model.User, error) {
	var user model.User
	if err := r.db.Where("hash = ?", hash).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// FindAll: 全ユーザーを取得する
func (r *UserRepository) FindAll() ([]model.User, error) {
	var users []model.User
	if err := r.db.Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}