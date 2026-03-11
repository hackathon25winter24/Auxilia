package repository

import (
	"context"
	"github.com/google/uuid"
	"auxilia/domain/model"
)

type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	FindByName(ctx context.Context, name string) (*model.User, error) // 重複確認用
	FindAll(ctx context.Context) ([]model.User, error)               // 一覧用
	Update(ctx context.Context, user *model.User) error
	Delete(ctx context.Context, id uuid.UUID) error
}