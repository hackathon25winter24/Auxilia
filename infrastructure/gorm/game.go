package gorm

import (
	//crand "crypto/rand"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"auxilia/domain"
	"auxilia/domain/model"

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
		BaseHP1:     model.DefaultBaseHP,
		BaseHP2:     model.DefaultBaseHP,
		Cost1P:      model.DefaultCost,
		Cost2P:      model.DefaultCost,
		Turn:        1,
		Is1PTurn:    true,
		TurnStartAt: now,
		IsTurnEnded: false, // 💡 初期状態はfalse
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

	err := r.db.Preload("Characters.Conditions").Preload("Grids").
		Where("room_id = ?", roomID).
		First(&gameData).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrGameNotFound
		}
		return nil, err
	}

	var actionLog model.GameActionLog
	if err := r.db.Where("room_id = ?", roomID).
		Order("sequence DESC").
		First(&actionLog).Error; err == nil {
		gameData.CurrentAction = actionLog
	}

	return &gameData, nil
}

func (r *BattleRepository) RegisterCharacters(roomID uint32, is1P bool, charIDs []uint32) ([]model.UniqueCharacter, error) {
	var characters []model.UniqueCharacter

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
			HP:          uint(model.DefaultCharacterHPs[uint(charID)]),
			PositionX:   charPos[i].X,
			PositionY:   charPos[i].Y,
		}
		characters = append(characters, char)
	}

	if err := r.db.Create(&characters).Error; err != nil {
		return nil, err
	}

	if err := r.recalculateTurnStarter(roomID); err != nil {
		return nil, err
	}

	return characters, nil
}

func (r *BattleRepository) ApplyMove(roomID uint32, playerID string, characterUniqueID, toX, toY uint32) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("room_id = ?", roomID).First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return domain.ErrGameNotFound }
			return err
		}

		if gameData.IsFinished || gameData.IsTurnEnded { return nil }

		expectedPlayerID := gameData.Player2ID
		if gameData.Is1PTurn { expectedPlayerID = gameData.Player1ID }
		if playerID != expectedPlayerID { return domain.ErrInvalidTurn }

		var character model.UniqueCharacter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND room_id = ?", characterUniqueID, roomID).First(&character).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return domain.ErrCharacterNotFound }
			return err
		}

		if character.HP == 0 || character.Is1P != gameData.Is1PTurn { return domain.ErrForbiddenAction }

		var cost uint
		cost = uint(model.CharacterData[GetCharacterIDFromUCID(characterUniqueID)].MoveCost)

		if err := r.consumePlayerCost(tx, gameData.ID, gameData.Is1PTurn, cost); err != nil {
      return err
    }

		if err := tx.Model(&model.UniqueCharacter{}).Where("id = ?", character.ID).Updates(map[string]any{
			"position_x": toX,
			"position_y": toY,
		}).Error; err != nil {
			return err
		}

		var lastSequence uint
		if err := tx.Where("room_id = ?", roomID).Order("sequence DESC").Limit(1).Pluck("sequence", &lastSequence).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		actionLog := model.GameActionLog{
			RoomID:                 uint(roomID),
			Sequence:               lastSequence + 1,
			PlayerID:               playerID,
			ActionType:             "MOVE",
			ActorCharacterUniqueID: uint(characterUniqueID),
			ToX:                    uint(toX),
			ToY:                    uint(toY),
		}
		return tx.Create(&actionLog).Error
	})
}

func (r *BattleRepository) ApplyAttack(roomID uint32, playerID string, attackerCharacterUniqueID uint32, attackType int32, attackInfos []model.AttackInfoData) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("room_id = ?", roomID).First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return domain.ErrGameNotFound }
			return err
		}

		if gameData.IsFinished || gameData.IsTurnEnded { return nil }
		if attackType < model.AttackType0 || attackType > model.AttackType3 { return domain.ErrForbiddenAction }

		expectedPlayerID := gameData.Player2ID
		if gameData.Is1PTurn { expectedPlayerID = gameData.Player1ID }
		if playerID != expectedPlayerID { return domain.ErrInvalidTurn }

		var character model.UniqueCharacter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND room_id = ?", attackerCharacterUniqueID, roomID).First(&character).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return domain.ErrCharacterNotFound }
			return err
		}

		if character.HP == 0 || character.Is1P != gameData.Is1PTurn { return domain.ErrForbiddenAction }

		var cost uint
		cost = uint(model.CharacterData[GetCharacterIDFromUCID(attackerCharacterUniqueID)].AttackCosts[attackType])

		if err := r.consumePlayerCost(tx, gameData.ID, gameData.Is1PTurn, cost); err != nil {
      return err
    }

		var targetIDs []string
		var damageLog string
		for _, info := range attackInfos {
			// 💡 コメント仕様：IDが98または99なら拠点のHP(BaseHP)を直接削る力技マッピング
			if info.AttackedCharacterID == 98 {
				if err := tx.Model(&model.GameData{}).Where("id = ?", gameData.ID).Update("base_hp1", info.NewHP).Error; err != nil {
					return err
				}
				damageLog += fmt.Sprintf("-%d(%d)", info.AttackedCharacterID, info.NewHP)
			} else if info.AttackedCharacterID == 99 {
				if err := tx.Model(&model.GameData{}).Where("id = ?", gameData.ID).Update("base_hp2", info.NewHP).Error; err != nil {
					return err
				}
				damageLog += fmt.Sprintf("-%d(%d)", info.AttackedCharacterID, info.NewHP)
			} else {
				// 通常のキャラクターへのダメージ適用
				if err := tx.Model(&model.UniqueCharacter{}).Where("id = ? AND room_id = ?", info.AttackedCharacterID, roomID).Update("hp", info.NewHP).Error; err != nil {
					return err
				}
				damageLog += fmt.Sprintf("-%d(%d)", info.AttackedCharacterID, info.NewHP)
			}
			targetIDs = append(targetIDs, fmt.Sprintf("%d", info.AttackedCharacterID))
		}

		var lastSequence uint
		if err := tx.Where("room_id = ?", roomID).Order("sequence DESC").Limit(1).Pluck("sequence", &lastSequence).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		actionLog := model.GameActionLog{
			RoomID:                 uint(roomID),
			Sequence:               lastSequence + 1,
			PlayerID:               playerID,
			ActionType:             "ATTACK",
			ActorCharacterUniqueID: uint(attackerCharacterUniqueID),
			AttackType:             int(attackType),
			TargetCharacterIDs:     strings.Join(targetIDs, ","),
			DamageLog:              damageLog,
		}
		return tx.Create(&actionLog).Error
	})
}

func (r *BattleRepository) consumePlayerCost(tx *gorm.DB, gameDataID uint, is1PTurn bool, cost uint) error {
	if is1PTurn {
		// 1Pのコストチェック
		var cost1P uint
		if err := tx.Model(&model.GameData{}).Where("id = ?", gameDataID).Pluck("cost1_p", &cost1P).Error; err != nil {
			return err
		}
		if cost1P < cost {
			return domain.ErrInsufficientCost
		}

		return tx.Model(&model.GameData{}).
			Where("id = ?", gameDataID).
			Update("cost1_p", gorm.Expr("cost1_p - ?", cost)).Error
	} else {
		var cost2P uint
		if err := tx.Model(&model.GameData{}).Where("id = ?", gameDataID).Pluck("cost2_p", &cost2P).Error; err != nil {
			return err
		}
		if cost2P < cost {
			return domain.ErrInsufficientCost
		}

		return tx.Model(&model.GameData{}).
			Where("id = ?", gameDataID).
			Update("cost2_p", gorm.Expr("cost2_p - ?", cost)).Error
	}
}

// EndTurn: 💡 ターン終了フラグを立てるだけに変更（ステートマシンの要件反映）
func (r *BattleRepository) EndTurn(roomID uint32) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("room_id = ?", roomID).First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return domain.ErrGameNotFound }
			return err
		}

		if gameData.IsFinished || gameData.IsTurnEnded {
			return nil
		}

		// DBのIsTurnEndedをtrueにする
		if err := tx.Model(&model.GameData{}).Where("id = ?", gameData.ID).Update("is_turn_ended", true).Error; err != nil {
			return err
		}

		// ログに通し番号を進めて記録
		var lastSequence uint
		if err := tx.Where("room_id = ?", roomID).Order("sequence DESC").Limit(1).Pluck("sequence", &lastSequence).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		expectedPlayerID := gameData.Player2ID
		if gameData.Is1PTurn { expectedPlayerID = gameData.Player1ID }

		actionLog := model.GameActionLog{
			RoomID:     uint(roomID),
			Sequence:   lastSequence + 1,
			PlayerID:   expectedPlayerID,
			ActionType: "END_TURN",
		}
		return tx.Create(&actionLog).Error
	})
}

// NewTurn: 💡 フロントの効果処理が全て終わった後、実際にターンを+1して次に進める
func (r *BattleRepository) NewTurn(roomID uint32, playerID string) error {
	now := time.Now()
	return r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("room_id = ?", roomID).First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return domain.ErrGameNotFound }
			return err
		}

		if gameData.IsFinished { return nil }

		// 試合中の拠点HPチェックを行い、終了条件を満たしていればここでリザルト処理へ
		if isGameFinished(gameData.BaseHP1, gameData.BaseHP2) {
			return r.finishGameAndUpdateRatings(tx, &gameData, now)
		}

		var characters []model.UniqueCharacter
		if err := tx.Where("room_id = ?", roomID).Find(&characters).Error; err != nil {
			return err
		}

		// 次の手番プレイヤーを判定
		nextIs1P := determineNextActor(&gameData, characters)

		// ターン数を +1、フラグをリセットし、コストを最大値(50)に回復させて更新
		if err := tx.Model(&model.GameData{}).Where("id = ?", gameData.ID).Updates(map[string]any{
			"turn":          gameData.Turn + 1,
			"is_1p_turn":    nextIs1P,
			"turn_start_at": now,
			"is_turn_ended": false, // 💡 フラグを元に戻す
			"cost1_p":       50,
			"cost2_p":       50,
		}).Error; err != nil {
			return err
		}

		var lastSequence uint
		if err := tx.Where("room_id = ?", roomID).Order("sequence DESC").Limit(1).Pluck("sequence", &lastSequence).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		actionLog := model.GameActionLog{
			RoomID:     uint(roomID),
			Sequence:   lastSequence + 1,
			PlayerID:   playerID,
			ActionType: "NEW_TURN",
		}
		return tx.Create(&actionLog).Error
	})
}

func (r *BattleRepository) ApplyGridUpdate(roomID uint32, playerID string, grids []model.Grid) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("room_id = ?", roomID).First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return domain.ErrGameNotFound }
			return err
		}

		if gameData.IsFinished || gameData.IsTurnEnded { return nil }

		expectedPlayerID := gameData.Player2ID
		if gameData.Is1PTurn { expectedPlayerID = gameData.Player1ID }
		if playerID != expectedPlayerID { return domain.ErrInvalidTurn }

		for _, g := range grids {
			if g.RemainingTurn < 0{
				if err := tx.Model(&model.Grid{}).
					Where("room_id = ? AND position_x = ? AND position_y = ?", roomID, g.PositionX, g.PositionY).
					Delete(&model.Grid{}).Error; err != nil {
					return err
				}
				continue
			}
			if err := tx.Model(&model.Grid{}).
				Where("room_id = ? AND position_x = ? AND position_y = ?", roomID, g.PositionX, g.PositionY).
				Updates(map[string]any{
					"grid_type":       g.GridType,
					"remaining_turn":  g.RemainingTurn,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *BattleRepository) DecrementGridRemainingTurn(roomID uint32) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Grid{}).
			Where("room_id = ? AND remaining_turn > 0", roomID).
			Update("remaining_turn", gorm.Expr("remaining_turn - ?", 1)).Error; err != nil {
			return err
		}

		if err := tx.Where("room_id = ? AND remaining_turn <= 0", roomID).Delete(&model.Grid{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func (r *BattleRepository) ApplyEffect(roomID uint32, playerID string, characterUniqueID uint32, effectType int32, newHP uint32) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("room_id = ?", roomID).First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return domain.ErrGameNotFound }
			return err
		}

		if gameData.IsFinished { return nil }

		var character model.UniqueCharacter
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND room_id = ?", characterUniqueID, roomID).First(&character).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return domain.ErrCharacterNotFound }
			return err
		}

		if err := tx.Model(&model.UniqueCharacter{}).Where("id = ?", character.ID).Update("hp", newHP).Error; err != nil {
			return err
		}

		var lastSequence uint
		if err := tx.Where("room_id = ?", roomID).Order("sequence DESC").Limit(1).Pluck("sequence", &lastSequence).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		actionLog := model.GameActionLog{
			RoomID:                 uint(roomID),
			Sequence:               lastSequence + 1,
			PlayerID:               playerID,
			ActionType:             "EFFECT",
			ActorCharacterUniqueID: uint(characterUniqueID),
			EffectType:             int(effectType),
		}
		return tx.Create(&actionLog).Error
	})
}

func (r *BattleRepository) finalizeGameIfNeeded(roomID uint32) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("room_id = ?", roomID).First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return domain.ErrGameNotFound }
			return err
		}

		if gameData.IsFinished { return nil }
		if !isGameFinished(gameData.BaseHP1, gameData.BaseHP2) { return nil }

		return r.finishGameAndUpdateRatings(tx, &gameData, time.Now())
	})
}

func (r *BattleRepository) finishGameAndUpdateRatings(tx *gorm.DB, gameData *model.GameData, finishedAt time.Time) error {
	score1, score2, winnerID := battleResult(gameData)

	var player1 model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", gameData.Player1ID).First(&player1).Error; err != nil {
		return err
	}

	var player2 model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", gameData.Player2ID).First(&player2).Error; err != nil {
		return err
	}

	oldRate1 := player1.Rate
	oldRate2 := player2.Rate
	newRate1, newRate2 := calculateEloRates(oldRate1, oldRate2, score1, score2)
	player1.Rate = newRate1
	player2.Rate = newRate2
	player1.NumBattles++
	player2.NumBattles++
	if score1 > score2 {
		player1.NumWins++
	} else if score2 > score1 {
		player2.NumWins++
	}

	gameData.Player1RateDelta = newRate1 - oldRate1
	gameData.Player2RateDelta = newRate2 - oldRate2
	gameData.Player1Rate = newRate1
	gameData.Player2Rate = newRate2

	if err := tx.Save(&player1).Error; err != nil { return err }
	if err := tx.Save(&player2).Error; err != nil { return err }

	updates := map[string]any{
		"is_finished":        true,
		"finished_at":         finishedAt,
		"player1_rate_delta": gameData.Player1RateDelta,
		"player2_rate_delta": gameData.Player2RateDelta,
		"player1_rate":       gameData.Player1Rate,
		"player2_rate":       gameData.Player2Rate,
	}
	if winnerID != nil {
		updates["winner_player_id"] = *winnerID
	} else {
		updates["winner_player_id"] = nil
	}

	if err := tx.Model(&model.GameData{}).Where("id = ?", gameData.ID).Updates(updates).Error; err != nil {
		return err
	}

	return tx.Model(&model.RoomMatch{}).Where("id = ?", gameData.RoomID).Update("is_gaming", false).Error
}

func (r *BattleRepository) recalculateTurnStarter(roomID uint32) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var gameData model.GameData
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("room_id = ?", roomID).First(&gameData).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return domain.ErrGameNotFound }
			return err
		}

		var characters []model.UniqueCharacter
		if err := tx.Where("room_id = ?", roomID).Find(&characters).Error; err != nil {
			return err
		}

		nextIs1P := determineNextActor(&gameData, characters)
		return tx.Model(&model.GameData{}).Where("id = ?", gameData.ID).Update("is_1p_turn", nextIs1P).Error
	})
}

func determineNextActor(gameData *model.GameData, characters []model.UniqueCharacter) bool {
	isOddTurn := gameData.Turn%2 != 0
	if isOddTurn {
		return !gameData.Is1PTurn
	}

	p1Min, has1P := minAliveMoveCost(characters, true)
	p2Min, has2P := minAliveMoveCost(characters, false)

	if has1P && !has2P { return true }
	if !has1P && has2P { return false }
	if !has1P && !has2P { return !gameData.Is1PTurn }

	if p1Min < p2Min { return true }
	if p2Min < p1Min { return false }

	return !gameData.Is1PTurn
}

func isGameFinished(baseHP1, baseHP2 uint) bool {
	return baseHP1 == 0 || baseHP2 == 0
}

func battleResult(gameData *model.GameData) (score1 float64, score2 float64, winnerID *string) {
	if gameData.BaseHP1 == 0 && gameData.BaseHP2 == 0 { return 0.5, 0.5, nil }
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
	if rate <= 0 { return 1500 }
	return rate
}

func minAliveMoveCost(characters []model.UniqueCharacter, is1P bool) (int, bool) {
	minCost := 0
	found := false
	for _, character := range characters {
		if character.Is1P != is1P || character.HP == 0 { continue }
		cost, ok := model.DefaultCharacterMoveCosts[character.CharacterID]
		if !ok { cost = 10 }
		if !found || cost < minCost {
			minCost = cost
			found = true
		}
	}
	return minCost, found
}

func (r *BattleRepository) FetchActionLog(roomID uint32, sequence uint32) (*model.GameActionLog, error) {
	var logData model.GameActionLog
	if err := r.db.Where("room_id = ? AND sequence = ?", roomID, sequence).First(&logData).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) { return nil, domain.ErrGameNotFound }
		return nil, err
	}
	return &logData, nil
}

func GetCharacterIDFromUCID(uniqueCharacterID uint32) uint {
	return uint(uniqueCharacterID / 1000)
}