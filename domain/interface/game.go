package repository

import (
	"auxilia/domain/model"
	"context"
)

type BattleRepository interface {
	// CreateGame は指定したルームIDとプレイヤーIDを使って
	// 試合を作成します。
	CreateGame(roomID uint32, p1ID, p2ID string) (*model.GameData, error)

	// GetGameDataByRoomID はルームID から関連する試合データを返します。
	GetGameDataByRoomID(ctx context.Context,roomID uint32) (*model.GameData, error)
	RegisterCharacters(ctx context.Context, roomID uint32, is1P bool, charIDs []uint32) ([]model.UniqueCharacter, error)
	ApplyMove(ctx context.Context, roomID uint32, playerID string, characterUniqueID, toX, toY uint32) error
	ApplyAttack(ctx context.Context, roomID uint32, playerID string, attackerCharacterUniqueID uint32, attackType int32, attackInfos []model.AttackInfoData) error
	EndTurn(ctx context.Context,roomID uint32) error
	ApplyGridUpdate(ctx context.Context, roomID uint32, grids []model.Grid) error
	ApplyEffect(ctx context.Context,roomID uint32, playerID string, characterUniqueID uint32, effectType int32, newHP uint32) error
	NewTurn(ctx context.Context, roomID uint32, playerID string) error
	FetchActionLog(ctx context.Context, roomID uint32, sequence uint32) (*model.GameActionLog, error)
}
