package gorm

import (
	"errors"

	"gorm.io/gorm"

	"auxilia/domain/model"
)

type RoomRepository struct {
	db *gorm.DB
}

func NewRoomRepository(db *gorm.DB) *RoomRepository {
	return &RoomRepository{db: db}
}

func (r *RoomRepository) JoinRoom(roomID int32, userID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {

		// 1. room_matchが存在するか確認（部屋削除と同時に入室させないため）
		var roomMatch model.RoomMatch
		if err := tx.Where("id = ?", roomID).First(&roomMatch).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("room does not exist")
			}
			return err
		}

		// 2. 既に同じユーザーが参加している場合は削除する（再参加対応）
		if err := tx.Where("room_id = ? AND user_id = ?", roomID, userID).Delete(&model.Room{}).Error; err != nil {
			return err
		}

		// 3. 現在の参加人数を取得
		var count int64
		if err := tx.Model(&model.Room{}).Where("room_id = ?", roomID).Count(&count).Error; err != nil {
			return err
		}

		// 4. 人数制限を確認
		if count >= 8 {
			return errors.New("room is full")
		}

		// 5. 参加者を追加
		room := model.Room{
			RoomID:  roomID,
			UserID:  userID,
			State:   model.StateSpectator,
			IsReady: false,
		}

		if err := tx.Create(&room).Error; err != nil {
			return err
		}
		return nil
	})
}