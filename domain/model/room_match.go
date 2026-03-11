package model

type RoomMatch struct {
	ID		  int	 `gorm:"primaryKey;autoIncrement" json:"id"`
	Name	  string `json:"name"`
	Owner	  string `json:"owner"`
	IsPrivate bool   `json:"is_private"`
}