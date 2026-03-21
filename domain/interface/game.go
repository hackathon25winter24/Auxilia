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
	ApplyMove(roomID uint32, playerID string, characterUniqueID, toX, toY uint32, cost uint32) (*model.GameData, error)
	ApplyAttack(roomID uint32, playerID string, attackerCharacterUniqueID uint32, attackType int32, isStarted bool, baseHP1 uint32, baseHP2 uint32, attackedCharacterUniqueID uint32, newHP uint32, cost uint32) (*model.GameData, *model.AttackInfo, error)
	EndTurn(roomID uint32) (*model.GameData, error)
	ApplyGridUpdate(roomID uint32, playerID string, grids []model.Grid, cost uint32) (*model.GameData, error)
}
