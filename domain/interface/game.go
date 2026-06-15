package repository

import (
	"auxilia/domain/model"
)

type BattleRepository interface {
	// CreateGame は指定したルームIDとプレイヤーIDを使って
	// 試合を作成します。
	CreateGame(roomID uint32, p1ID, p2ID string) (*model.GameData, error)

	// GetGameDataByRoomID はルームID から関連する試合データを返します。
	GetGameDataByRoomID(roomID uint32) (*model.GameData, error)
	RegisterCharacters(roomID uint32, is1P bool, charIDs []uint32) ([]model.UniqueCharacter, error)
	ApplyMove(roomID uint32, playerID string, characterUniqueID, toX, toY uint32) error
	ApplyAttack(roomID uint32, playerID string, attackerCharacterUniqueID uint32, attackType int32, attackInfos []model.AttackInfoData) error
	EndTurn(roomID uint32) error
	ApplyGridUpdate(roomID uint32, playerID string, grids []model.Grid) error
	ApplyEffect(roomID uint32, playerID string, characterUniqueID uint32, effectType int32, newHP uint32) error
	NewTurn(roomID uint32, playerID string) error
	FetchActionLog(roomID uint32, sequence uint32) (*model.GameActionLog, error)
}
