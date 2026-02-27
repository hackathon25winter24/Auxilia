package gorm

import (
	repository "auxilia/domain/interface"
	"auxilia/domain/model"
	"context"

	"gorm.io/gorm"
)

// GameRepository は GORM を使った実装です。インターフェースは
// domain/interface/game.go で定義されています。
//
// コンパイル時にインターフェースを満たしていることをチェックするため、
// 空白識別子アサーションを配置しておきます。

type GameRepository struct {
	db *gorm.DB
}

var _ repository.GameRepository = (*GameRepository)(nil)

func NewGameRepository(db *gorm.DB) *GameRepository {
	return &GameRepository{db: db}
}

func (r *GameRepository) SaveGame(ctx context.Context, game *model.GameData) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// すでに存在する場合は一度消してから作り直す（または Upsert する）のがゲームデータでは確実
		var existing model.GameData
		if err := tx.Where("room_id = ?", game.RoomID).First(&existing).Error; err == nil {
			// 既存データがある場合は削除（関連データも後述のDeleteGameと同じロジックで消すのが安全）
			r.deleteGameData(tx, game.RoomID)
		}

		// 保存（リレーションも一括保存される）
		return tx.Create(game).Error
	})
}

func (r *GameRepository) DeleteGame(ctx context.Context, roomID uint) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return r.deleteGameData(tx, roomID)
	})
}

// 内部用：関連テーブルを順番に消す
func (r *GameRepository) deleteGameData(tx *gorm.DB, roomID uint) error {
	var uniqueIDs []uint
	tx.Model(&model.UniqueCharacter{}).Where("room_id = ?", roomID).Pluck("id", &uniqueIDs)

	if len(uniqueIDs) > 0 {
		tx.Where("unique_character_id IN ?", uniqueIDs).Delete(&model.CharacterCondition{})
		tx.Where("id IN ?", uniqueIDs).Delete(&model.UniqueCharacter{})
	}
	return tx.Where("room_id = ?", roomID).Delete(&model.GameData{}).Error
}

func (r *GameRepository) ListGames(ctx context.Context) ([]model.GameData, error) {
	var games []model.GameData
	// Preload にリレーション名を指定することで、子や孫のテーブルも一気に取得
	err := r.db.WithContext(ctx).
		Preload("Characters.Conditions"). 
		Find(&games).Error
	return games, err
}
// GetByRoomID retrieves a single game record by room ID, including related characters and conditions.
func (r *GameRepository) GetByRoomID(ctx context.Context, roomID uint) (*model.GameData, error) {
    var game model.GameData
    err := r.db.WithContext(ctx).
        Preload("Characters.Conditions").
        Where("room_id = ?", roomID).
        First(&game).Error
    if err != nil {
        return nil, err
    }
    return &game, nil
}

// UpdateGame updates an existing record; this implementation simply
// reuses SaveGame which deletes then recreates the row to ensure
// related tables stay in sync.
func (r *GameRepository) UpdateGame(ctx context.Context, game *model.GameData) error {
    // delegate to SaveGame, which already handles upsert semantics
    return r.SaveGame(ctx, game)
}
