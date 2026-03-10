package repository

import (
	"context"
	"auxilia/domain/model"
)

// GameRepository は試合データの保存・取得・削除を管理するインターフェースです。
type GameRepository interface {
    SaveGame(ctx context.Context, data *model.GameData) error
    GetByRoomID(ctx context.Context, roomID uint) (*model.GameData, error) 
    UpdateGame(ctx context.Context, data *model.GameData) error            
    DeleteGame(ctx context.Context, roomID uint) error
    ListGames(ctx context.Context) ([]model.GameData, error)
}