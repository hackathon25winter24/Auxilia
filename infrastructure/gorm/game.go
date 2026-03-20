package gorm

import (
	crand "crypto/rand"
	"errors"
	"math"
	"time"

	"auxilia/domain"
	"auxilia/domain/model" // プロジェクト構造に合わせて調整してください

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

	var grids []model.Grid
	for x := uint(0); x < 8; x++ {
		for y := uint(0); y < 5; y++ {
			grids = append(grids, model.Grid{
				PositionX: x,
				PositionY: y,
				GridType:  0,
			})
		}
	}
	gameData.Grids = grids

	// キャラクター登録がないため、シンプルな Create で実装可能
	if err := r.db.Create(gameData).Error; err != nil {
		return nil, err
	}

	return gameData, nil
}

// GetGameDataByRoomID: RoomIDから関連する全データを取得
func (r *BattleRepository) GetGameDataByRoomID(roomID uint32) (*model.GameData, error) {
	if err := r.finalizeGameIfNeeded(roomID); err != nil {
		return nil, err
	}

	return r.loadGameDataByRoomID(roomID)
}

func (r *BattleRepository) loadGameDataByRoomID(roomID uint32) (*model.GameData, error) {
	var gameData model.GameData

	// PreloadでCharactersとそのConditionsまで再帰的に取得
	err := r.db.Preload("Characters.Conditions").Preload("Grids").
		Where("room_id = ?", roomID).
		First(&gameData).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrGameNotFound
		}
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

	if err := r.recalculateTurnStarter(roomID); err != nil {
		return nil, err
	}

	return characters, nil
}

func (r *BattleRepository) ApplyMove(roomID uint32, playerID string, characterUniqueID, toX, toY uint32) (*model.GameData, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("room_id = ?", roomID).
			First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrGameNotFound
			}
			return err
		}

		if gameData.IsFinished {
			return nil
		}

		is1PTurn := gameData.Is1PTurn
		expectedPlayerID := gameData.Player2ID
		if is1PTurn {
			expectedPlayerID = gameData.Player1ID
		}

		if playerID != expectedPlayerID {
			return domain.ErrInvalidTurn
		}

		var character model.UniqueCharacter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND room_id = ?", characterUniqueID, roomID).
			First(&character).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrCharacterNotFound
			}
			return err
		}

		if character.HP == 0 {
			return domain.ErrForbiddenAction
		}
		if character.Is1P != is1PTurn {
			return domain.ErrForbiddenAction
		}

		return tx.Model(&model.UniqueCharacter{}).
			Where("id = ?", character.ID).
			Updates(map[string]any{
				"position_x": toX,
				"position_y": toY,
			}).Error
	})
	if err != nil {
		return nil, err
	}

	return r.GetGameDataByRoomID(roomID)
}

func (r *BattleRepository) ApplyAttack(roomID uint32, playerID string, attackerCharacterUniqueID uint32, attackType int32, isStarted bool, baseHP1 uint32, baseHP2 uint32, attackedCharacterUniqueID uint32, newHP uint32) (*model.GameData, *model.AttackInfo, error) {
	var createdAttackInfo *model.AttackInfo

	err := r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("room_id = ?", roomID).
			First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrGameNotFound
			}
			return err
		}

		if gameData.IsFinished {
			return nil
		}

		if attackType < model.AttackType0 || attackType > model.AttackType3 {
			return domain.ErrForbiddenAction
		}

		is1PTurn := gameData.Is1PTurn
		expectedPlayerID := gameData.Player2ID
		attackerSide := model.AttackBy2P
		if is1PTurn {
			expectedPlayerID = gameData.Player1ID
			attackerSide = model.AttackBy1P
		}

		if playerID != expectedPlayerID {
			return domain.ErrInvalidTurn
		}

		var character model.UniqueCharacter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND room_id = ?", attackerCharacterUniqueID, roomID).
			First(&character).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrCharacterNotFound
			}
			return err
		}

		if character.HP == 0 {
			return domain.ErrForbiddenAction
		}
		if character.Is1P != is1PTurn {
			return domain.ErrForbiddenAction
		}

		attackerCharacterID := character.ID
		attackInfo := model.AttackInfo{
			RoomID:              uint(roomID),
			AttackerSide:        attackerSide,
			IsStarted:           isStarted,
			AttackerCharacterID: &attackerCharacterID,
			AttackType:          attackType,
		}

		if err := tx.Create(&attackInfo).Error; err != nil {
			return err
		}
		createdAttackInfo = &attackInfo

		if err := tx.Model(&model.UniqueCharacter{}).
			Where("id = ?", character.ID).
			Update("is_selected", true).Error; err != nil {
			return err
		}

		if err := tx.Model(&model.GameData{}).
			Where("id = ?", gameData.ID).
			Updates(map[string]any{
				"base_hp1": baseHP1,
				"base_hp2": baseHP2,
			}).Error; err != nil {
			return err
		}

		if attackedCharacterUniqueID != 0 {
			if err := tx.Model(&model.UniqueCharacter{}).
				Where("id = ? AND room_id = ?", attackedCharacterUniqueID, roomID).
				Update("hp", newHP).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	gameData, err := r.GetGameDataByRoomID(roomID)
	return gameData, createdAttackInfo, err
}

func (r *BattleRepository) EndTurn(roomID uint32) (*model.GameData, error) {
	now := time.Now()
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("room_id = ?", roomID).
			First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrGameNotFound
			}
			return err
		}

		if gameData.IsFinished {
			return nil
		}

		if isGameFinished(gameData.BaseHP1, gameData.BaseHP2) {
			return r.finishGameAndUpdateRatings(tx, &gameData, now)
		}

		var characters []model.UniqueCharacter
		if err := tx.Where("room_id = ?", roomID).Find(&characters).Error; err != nil {
			return err
		}

		nextIs1P := determineNextActor(characters, gameData.Is1PTurn)
		return tx.Model(&model.GameData{}).
			Where("id = ?", gameData.ID).
			Updates(map[string]any{
				"turn":          gameData.Turn + 1,
				"is_1p_turn":    nextIs1P,
				"turn_start_at": now,
			}).Error
	})
	if err != nil {
		return nil, err
	}

	return r.GetGameDataByRoomID(roomID)
}

func (r *BattleRepository) finalizeGameIfNeeded(roomID uint32) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("room_id = ?", roomID).
			First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrGameNotFound
			}
			return err
		}

		if gameData.IsFinished {
			return nil
		}

		if !isGameFinished(gameData.BaseHP1, gameData.BaseHP2) {
			return nil
		}

		return r.finishGameAndUpdateRatings(tx, &gameData, time.Now())
	})
}

func (r *BattleRepository) finishGameAndUpdateRatings(tx *gorm.DB, gameData *model.GameData, finishedAt time.Time) error {
	score1, score2, winnerID := battleResult(gameData)

	var player1 model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", gameData.Player1ID).
		First(&player1).Error; err != nil {
		return err
	}

	var player2 model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", gameData.Player2ID).
		First(&player2).Error; err != nil {
		return err
	}

	newRate1, newRate2 := calculateEloRates(player1.Rate, player2.Rate, score1, score2)
	player1.Rate = newRate1
	player2.Rate = newRate2
	player1.NumBattles++
	player2.NumBattles++
	if score1 > score2 {
		player1.NumWins++
	} else if score2 > score1 {
		player2.NumWins++
	}

	if err := tx.Save(&player1).Error; err != nil {
		return err
	}
	if err := tx.Save(&player2).Error; err != nil {
		return err
	}

	updates := map[string]any{
		"is_finished": true,
		"finished_at": finishedAt,
	}
	if winnerID != nil {
		updates["winner_player_id"] = *winnerID
	} else {
		updates["winner_player_id"] = nil
	}

	if err := tx.Model(&model.GameData{}).
		Where("id = ?", gameData.ID).
		Updates(updates).Error; err != nil {
		return err
	}

	return tx.Model(&model.RoomMatch{}).
		Where("id = ?", gameData.RoomID).
		Update("is_gaming", false).Error
}

func (r *BattleRepository) recalculateTurnStarter(roomID uint32) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("room_id = ?", roomID).
			First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrGameNotFound
			}
			return err
		}

		var characters []model.UniqueCharacter
		if err := tx.Where("room_id = ?", roomID).Find(&characters).Error; err != nil {
			return err
		}

		nextIs1P := determineNextActor(characters, gameData.Is1PTurn)
		return tx.Model(&model.GameData{}).
			Where("id = ?", gameData.ID).
			Update("is_1p_turn", nextIs1P).Error
	})
}

func determineNextActor(characters []model.UniqueCharacter, currentIs1PTurn bool) bool {
	p1Min, has1P := minAliveMoveCost(characters, true)
	p2Min, has2P := minAliveMoveCost(characters, false)

	if has1P && !has2P {
		return true
	}
	if !has1P && has2P {
		return false
	}
	if !has1P && !has2P {
		return currentIs1PTurn
	}

	if p1Min < p2Min {
		return true
	}
	if p2Min < p1Min {
		return false
	}

	return randomFirstPlayer(currentIs1PTurn)
}

func randomFirstPlayer(fallback bool) bool {
	var b [1]byte
	if _, err := crand.Read(b[:]); err != nil {
		return fallback
	}
	return b[0]%2 == 0
}

func isGameFinished(baseHP1, baseHP2 uint) bool {
	return baseHP1 == 0 || baseHP2 == 0
}

func battleResult(gameData *model.GameData) (score1 float64, score2 float64, winnerID *string) {
	if gameData.BaseHP1 == 0 && gameData.BaseHP2 == 0 {
		return 0.5, 0.5, nil
	}
	if gameData.BaseHP2 == 0 {
		winner := gameData.Player1ID
		return 1.0, 0.0, &winner
	}
	winner := gameData.Player2ID
	return 0.0, 1.0, &winner
}

func calculateEloRates(rate1, rate2 int, score1, score2 float64) (int, int) {
	const k = 32.0

	r1 := float64(normalizeRate(rate1))
	r2 := float64(normalizeRate(rate2))

	expected1 := 1.0 / (1.0 + math.Pow(10, (r2-r1)/400.0))
	expected2 := 1.0 / (1.0 + math.Pow(10, (r1-r2)/400.0))

	newRate1 := int(math.Round(r1 + k*(score1-expected1)))
	newRate2 := int(math.Round(r2 + k*(score2-expected2)))
	return newRate1, newRate2
}

func normalizeRate(rate int) int {
	if rate <= 0 {
		return 1500
	}
	return rate
}

func minAliveMoveCost(characters []model.UniqueCharacter, is1P bool) (int, bool) {
	minCost := 0
	found := false

	for _, character := range characters {
		if character.Is1P != is1P {
			continue
		}
		if character.HP == 0 {
			continue
		}

		cost, ok := model.CharacterMoveCosts[character.CharacterID]
		if !ok {
			cost = 10
		}

		if !found || cost < minCost {
			minCost = cost
			found = true
		}
	}

	return minCost, found
}

func (r *BattleRepository) ApplyGridUpdate(roomID uint32, playerID string, grids []model.Grid) (*model.GameData, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("room_id = ?", roomID).
			First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return domain.ErrGameNotFound
			}
			return err
		}

		if gameData.IsFinished {
			return nil
		}

		// ターンチェック (オプションだが他のApply系と同様に実装)
		is1PTurn := gameData.Is1PTurn
		expectedPlayerID := gameData.Player2ID
		if is1PTurn {
			expectedPlayerID = gameData.Player1ID
		}
		if playerID != expectedPlayerID {
			return domain.ErrInvalidTurn
		}

		// グリッド情報の更新
		for _, g := range grids {
			if err := tx.Model(&model.Grid{}).
				Where("room_id = ? AND position_x = ? AND position_y = ?", roomID, g.PositionX, g.PositionY).
				Update("grid_type", g.GridType).Error; err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return r.GetGameDataByRoomID(roomID)
}
