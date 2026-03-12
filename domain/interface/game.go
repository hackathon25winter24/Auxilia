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
}