package model

import	"github.com/google/uuid"


type User struct {
	// MariaDBには専用のUUID型がないため、VARCHAR(36)として保存します
	ID    uuid.UUID `gorm:"type:char(36);primaryKey" json:"id"`
	Hash  string    `json:"hash"`
	Story int       `json:"story"`
	Rate  int       `json:"rate"`
}