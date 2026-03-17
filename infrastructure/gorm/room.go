package gorm

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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

		// 【追加】全てのroom操作で共通して同一IDのroom_matchをロックする。そうすることで同一部屋の変更処理が直列化される
		var roomLock model.RoomMatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", roomID).First(&roomLock).Error; err != nil {
			return err
		}

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
		if roomMatch.IsGaming {
			return domain.ErrMatchStarted
		}
		return nil
	})
}

func (r *RoomRepository) LeaveRoom(roomID int32, userID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {

		// 【追加】全てのroom操作で共通して同一IDのroom_matchをロックする。そうすることで同一部屋の変更処理が直列化される
		var roomLock model.RoomMatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", roomID).First(&roomLock).Error; err != nil {
			return err
		}

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

func (r *RoomRepository) EnterRing(roomID int32, userID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {

		// 1. 同IDのRoomMatchをロック
		var roomLock model.RoomMatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", roomID).First(&roomLock).Error; err != nil {
			return err
		}

		// 2. 1P2Pの存在チェック
		var players [] model.Room
		if err := tx.Where("room_id = ? AND state IN (?, ?)", roomID, model.StatePlayer1, model.StatePlayer2).Find(&players).Error; err != nil {
			return err
		}

		var p1Exists bool
		var p2Exists bool

		for _, p := range players {
			if p.State == model.StatePlayer1 {
				p1Exists = true
			}
			if p.State == model.StatePlayer2 {
				p2Exists = true
			}
		}

		// 3. 1P2Pが共に埋まっている時、エラーを返す
		if p1Exists && p2Exists {
			return domain.ErrRingFull
		}

		// 4. 両方いなければ1Pに、1Pがいれば2PにStateを更新
		state := model.StatePlayer1
		if p1Exists {
			state = model.StatePlayer2
		}

		if err := tx.Model(&model.Room{}).
			Where("room_id = ? AND user_id = ? AND state = ?", roomID, userID, model.StateSpectator).
			Update("state", state).Error; err != nil {
			return err
		}

		return nil
	})
}

func (r *RoomRepository) LeaveRing(roomID int32, userID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {

		// 1. 同IDのRoomMatchをロック
		var roomLock model.RoomMatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", roomID).First(&roomLock).Error; err != nil {
			return err
		}

		// 2. 自分を取得
		var self model.Room
		if err := tx.Where("room_id = ? AND user_id = ?", roomID, userID).First(&self).Error; err != nil {
			return err
		}
		

		// 3. 観戦者なら何もしない
		if self.State == model.StateSpectator {
			return nil
		}


		// 5. DBの状態を観戦者に更新、is_readyをfalseにする（selfは更新されない）
		if err := tx.Model(&model.Room{}).
			Where("room_id = ? AND user_id = ? AND state != ?", roomID, userID, model.StateSpectator).
			Updates(map[string]interface{}{
				"state": model.StateSpectator,
				"is_ready": false,
			}).Error; err != nil {
			return err
		}

		// 6. 自分が1Pだったなら2Pを1Pに更新する
		if self.State == model.StatePlayer1 {

			if err := tx.Model(&model.Room{}).
				Where("room_id = ? AND state = ?", roomID, model.StatePlayer2).
				Update("state", model.StatePlayer1).Error; err != nil {
					return err
				}
		}

		return nil
	})
}

func (r *RoomRepository) SetReady(roomID int32, userID string, ready bool) error {
	return r.db.Transaction(func(tx *gorm.DB) error {

		// 1. 同IDのRoomMatchをロック
		var roomLock model.RoomMatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", roomID).First(&roomLock).Error; err != nil {
			return err
		}

		// 2. 自分を取得
		var self model.Room
		if err := tx.Where("room_id = ? AND user_id = ?", roomID, userID).First(&self).Error; err != nil {
			return err
		}

		// 3. 観戦者はready不可
		if self.State == model.StateSpectator {
			return domain.ErrSpectatorCannotReady
		}

		// 4. DBのready更新
		if err := tx.Model(&model.Room{}).
			Where("room_id = ? AND user_id = ? AND is_ready != ?", roomID, userID, ready).
			Update("is_ready", ready).Error; err != nil {
			return err
		}
	/*
		// 5. 1P2Pのis_readyチェック
		var count int64
		if err := tx.Where("room_id = ? AND state IN (?, ?) AND is_ready = ?", 
			roomID, model.StatePlayer1, model.StatePlayer2, true).
			Count(&count).Error; err != nil {
				return err
			}

		// 6. ゲーム二重開始バグを防ぐため試合が始まっているかを確認
		var roomMatch model.RoomMatch
		if err := tx.Where("id = ?", roomID).First(&roomMatch).Error; err != nil {
			return err
		}

		if count == 2 && !roomMatch.IsGaming {
			if err := tx.Model(&model.RoomMatch{}).Where("id = ?", roomID).Update("is_gaming", true).Error; err != nil {
				return err
			}
			// ここにゲーム開始処理

		}
	*/

		return nil
	})
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

func (r *RoomRepository) StartMatch(ctx context.Context, roomID int32) (string, string, error) {
	var p1ID, p2ID string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var roomMatch model.RoomMatch
		if err := tx.Where("id = ?", roomID).First(&roomMatch).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrRoomNotFound
			}
			return err
		}

		if roomMatch.IsGaming {
			return domain.ErrMatchStarted
		}

		var rooms []model.Room
		if err := tx.Where("room_id = ?", roomID).Find(&rooms).Error; err != nil {
			return err
		}
		if len(rooms) == 0 {
			return domain.ErrRoomNotFound
		}

		hasPlayer1 := false
		hasPlayer2 := false
		for _, room := range rooms {
			if !room.IsReady {
				return domain.ErrNotAllUsersReady
			}
			if room.State == model.StatePlayer1 {
				hasPlayer1 = true
				p1ID = room.UserID
			}
			if room.State == model.StatePlayer2 {
				hasPlayer2 = true
				p2ID = room.UserID
			}
		}

		if !hasPlayer1 || !hasPlayer2 {
			return domain.ErrPlayerSlotsNotFilled
		}

		return tx.Model(&model.RoomMatch{}).
			Where("id = ?", roomID).
			Update("is_gaming", true).Error
	})
	return p1ID, p2ID, err
}
