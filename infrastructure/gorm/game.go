package gorm

import (
	"time"

	"auxilia/domain/model" // プロジェクト構造に合わせて調整してください
	"gorm.io/gorm"
)

type BattleRepository struct {
	db *gorm.DB
}

func NewBattleRepository(db *gorm.DB) *BattleRepository {
	return &BattleRepository{db: db}
}

// CreateGame: 試合情報の初期登録
func (r *BattleRepository) CreateGame(roomID uint32, p1ID, p2ID string) (*model.GameData, error) {
	now := time.Now()
	gameData := &model.GameData{
		RoomID:      uint(roomID),
		Player1ID:   p1ID,
		Player2ID:   p2ID,
		BaseHP1:     200,
		BaseHP2:     200,
		Turn:        1,
		Is1PTurn:    true,
		TurnStartAt: now,
		// Characters は空の状態で開始
	}

	// キャラクター登録がないため、シンプルな Create で実装可能
	if err := r.db.Create(gameData).Error; err != nil {
		return nil, err
	}

	return gameData, nil
}

// GetGameDataByRoomID: RoomIDから関連する全データを取得
func (r *BattleRepository) GetGameDataByRoomID(roomID uint32) (*model.GameData, error) {
	var gameData model.GameData
	
	// PreloadでCharactersとそのConditionsまで再帰的に取得
	err := r.db.Preload("Characters.Conditions").
		Where("room_id = ?", roomID).
		First(&gameData).Error

	if err != nil {
		return nil, err
	}

	return &gameData, err
}

func (r *BattleRepository) RegisterCharacters(roomID uint32, is1P bool, charIDs []uint32) ([]model.UniqueCharacter, error) {
	var characters []model.UniqueCharacter
	
	// Y座標を1Pなら下側(0)、2Pなら上側(7)にするなどの初期配置ロジック
	var charPos []model.Position
	if is1P {
			charPos = model.DefaultPoints1P
	} else {
			charPos = model.DefaultPoints2P
	}

	for i, charID := range charIDs {
		char := model.UniqueCharacter{
			RoomID:      uint(roomID),
			Is1P:        is1P,
			CharacterID: uint(charID),
			HP:          uint(model.CharacterHPs[uint(charID)]),
			PositionX:   charPos[i].X,
			PositionY:   charPos[i].Y,
		}
		characters = append(characters, char)
	}

	// バルクインサート（一括保存）
	if err := r.db.Create(&characters).Error; err != nil {
		return nil, err
	}

	return characters, nil
}