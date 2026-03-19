package gorm

import (
	"context"

	"auxilia/domain/model"

	"gorm.io/gorm"
)

type RoomMatchRepository struct {
	db *gorm.DB
}

func NewRoomMatchRepository(db *gorm.DB) *RoomMatchRepository {
	return &RoomMatchRepository{db: db}
}

// CreateRoomMatch: マッチング部屋をDBに保存する
func (r *RoomMatchRepository) CreateRoomMatch(room *model.RoomMatch) error {
	return r.db.Create(room).Error
}

// FindAll: 全てのマッチング部屋を取得する
func (r *RoomMatchRepository) FindAll(ctx context.Context) ([]model.RoomMatch, error) {
	// ユーザーが0人の部屋をDBから削除する
	if err := r.db.WithContext(ctx).Exec(`
		DELETE FROM room_matches
		WHERE id NOT IN (
			SELECT DISTINCT room_id FROM rooms
		)
	`).Error; err != nil {
		return nil, err
	}

	var rooms []model.RoomMatch
	if err := r.db.WithContext(ctx).Find(&rooms).Error; err != nil {
		return nil, err
	}
	return rooms, nil
}
