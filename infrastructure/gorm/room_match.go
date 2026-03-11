package gorm

import (
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