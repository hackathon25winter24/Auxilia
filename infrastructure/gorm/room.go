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

func (r *RoomRepository) LeaveRoom(roomID int32, userID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {

		// 1. 退出ユーザー削除
		if err := tx.Where("room_id = ? AND user_id = ?", roomID, userID).Delete(&model.Room{}).Error; err != nil {
			return err
		}

		// 2. 残り人数を確認
		var count int64
		if err := tx.Model(&model.Room{}).Where("room_id = ?", roomID).Count(&count).Error; err != nil {
			return err
		}

		// 3. 残り人数が0ならRoomMatchを削除
		if count == 0 {
			if err := tx.Where("id = ?", roomID).Delete(&model.RoomMatch{}).Error; err != nil {
				return err
			}
			return nil // 最後の1人ならオーナー更新処理は不要
		}

		// 4. 自分がオーナーだったら入室順が早いプレイヤーにオーナーを更新
		var roomMatch model.RoomMatch
		if err := tx.Where("id = ?", roomID).First(&roomMatch).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}

		if roomMatch.OwnerID == userID {
			var nextOwner model.Room
			if err := tx.Where("room_id = ?", roomID).Order("joined_at ASC").First(&nextOwner).Error; err != nil {
				return err
			}

			if err := tx.Model(&model.RoomMatch{}).Where("id = ?", roomID).Update("owner_id", nextOwner.UserID).Error; err != nil {
				return err
			}
		}

		// 5. 回線落ちなどによる同時退出に備えて念の為
		if err := tx.Exec(`
		DELETE FROM room_matches
		WHERE id = ?
		AND NOT EXISTS (
			SELECT 1 FROM rooms WHERE room_id = ?
		)
		`, roomID, roomID).Error; err != nil {
			return err
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