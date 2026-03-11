package model

type RoomMatch struct {
	ID		  int	 `gorm:"primaryKey;autoIncrement" json:"id"`
	RoomName	  string `json:"room_name"`
	OwnerID	  string `json:"owner_id"`
	IsPrivate bool   `json:"is_private"`
}