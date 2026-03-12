package domain

import "errors"

var (
	ErrRoomNotFound = errors.New("room does not exist")
	ErrRoomFull = errors.New("room is full")
	ErrMatchStarted = errors.New("match has already started")
)