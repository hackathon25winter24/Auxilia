package repository

import (
	"context"
	"auxilia/domain/model"
)

// GameRepository は試合データの保存・取得・削除を管理するインターフェースです。
type GameRepository interface {
	// SaveGame: 試合の進行状況を保存します（新規作成・更新共通）
	SaveGame(ctx context.Context, game *model.GameData) error

	// DeleteGame: RoomIDを指定して、関連する全データを削除します
	DeleteGame(ctx context.Context, roomID uint) error

	// ListGames: 現在DBに存在するすべての試合情報を取得します（デバッグや一覧表示用）
	ListGames(ctx context.Context) ([]model.GameData, error)

	// GetGame: 特定のRoomIDのデータを取得します（必要に応じて）
	GetGame(ctx context.Context, roomID uint) (*model.GameData, error)
}