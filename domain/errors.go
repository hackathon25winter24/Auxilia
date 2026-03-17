package domain

import "errors"

var (
	ErrRoomNotFound         = errors.New("room does not exist")
	ErrRoomFull             = errors.New("room is full")
	ErrMatchStarted         = errors.New("match has already started")
	ErrNotAllUsersReady     = errors.New("not all users are ready")
	ErrPlayerSlotsNotFilled = errors.New("both 1p and 2p users must exist")
	ErrGameNotFound         = errors.New("game does not exist")
	ErrCharacterNotFound    = errors.New("character does not exist")
	ErrInvalidTurn          = errors.New("invalid turn")
	ErrForbiddenAction      = errors.New("forbidden action")
)
