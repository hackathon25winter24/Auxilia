package gorm

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"auxilia/domain"
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
				return domain.ErrRoomNotFound
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
			return domain.ErrRoomFull
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

		// 6. もし試合が始まっているなら、試合が始まっているとエラーを返す（フロント側で観戦処理に移行するため）
		if err := tx.Where("room_id = ? AND is_private = ?", roomID, false).First(&model.RoomMatch{}).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrMatchStarted
			}
		}
		return nil
	})
}

func (r *RoomRepository) ListRoom(ctx context.Context, roomID int32) ([]model.Room, error) {

	var rooms []model.Room

	if err := r.db.WithContext(ctx).Where("room_id = ?", roomID).Find(&rooms).Error; err != nil {
		return nil, err
	}

	return rooms, nil
}

func (r *RoomRepository) UpdateRoomState(ctx context.Context, roomID int32, userID string, state int32, isReady bool) error {
	result := r.db.WithContext(ctx).
		Model(&model.Room{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Updates(map[string]any{
			"state":    state,
			"is_ready": isReady,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrRoomNotFound
	}
	return nil
}
