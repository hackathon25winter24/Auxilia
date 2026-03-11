package repository

type RoomRepository interface {
	JoinRoom(roomID int32, userID string) error
}