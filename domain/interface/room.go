package repository

import (
	"auxilia/domain/model"
	"context"
)

type RoomRepository interface {
	JoinRoom(roomID int32, userID string) error
	ListRoom(ctx context.Context, roomID int32) ([]model.Room, error)
	UpdateRoomState(ctx context.Context, roomID int32, userID string, state int32, isReady bool) error
	StartMatch(ctx context.Context, roomID int32) (player1ID string, player2ID string, err error)
}
