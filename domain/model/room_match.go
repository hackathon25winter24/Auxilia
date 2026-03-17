package model

type RoomMatch struct {
	ID       int    `gorm:"primaryKey;autoIncrement" json:"id"`
	RoomName string `json:"room_name"`
	OwnerID  string `json:"owner_id"`
	IsGaming bool   `gorm:"column:is_gaming;not null;default:false" json:"is_gaming"`
}
