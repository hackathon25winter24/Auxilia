package repository

import (
	"context"
	"auxilia/domain/model"
)

type RoomMatchRepository interface {
	CreateRoomMatch(room *model.RoomMatch) error
	FindAll(ctx context.Context) ([]model.RoomMatch, error)
	UpdateRoomMatch(room *model.RoomMatch) error
}