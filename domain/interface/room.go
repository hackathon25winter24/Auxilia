package repository

import (
	"context"
	"auxilia/domain/model"
)

type RoomRepository interface {
	JoinRoom(roomID int32, userID string) error
	ListRoom(ctx context.Context, roomID int32) ([]model.Room, error)
}