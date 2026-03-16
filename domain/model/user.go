package model

import (
    "github.com/google/uuid"
    "gorm.io/gorm"
)

type User struct {
		// MariaDBには専用のUUID型がないため、VARCHAR(36)として保存します
    ID    uuid.UUID `gorm:"type:char(36);primaryKey" json:"id"`
    Name string    `gorm:"unique" ;json:"name"`
    Hash  string    `json:"hash"`
    Story int       `json:"story"`
    NumWins int       `json:"num_wins"`
    NumBattles int     `json:"num_battles"`
    Rate int `json:"rate"`
    HomeCharacterID int `json:"home_character_id"`
    Deck1 int `json:"deck_1"` // 初期値は-1で、未設定を表します。
    Deck2 int `json:"deck_2"`
    Deck3 int `json:"deck_3"`
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
    if u.ID == uuid.Nil {
        u.ID = uuid.New()
    }
    return nil
}