package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Room struct {
	ID       uuid.UUID `gorm:"type:char(36);primaryKey" json:"id"`
	RoomID   int32     `gorm:"index;not null" json:"room_id"`
	UserID   string    `gorm:"not null" json:"user_id"`

	State    int32     `gorm:"not null" json:"state"`
	IsReady  bool      `gorm:"not null" json:"is_ready"`

	JoinedAt time.Time `gorm:"autoCreateTime" json:"joined_at"`
}

const (
	StateSpectator int32 = 0
	StatePlayer1   int32 = 1
	StatePlayer2   int32 = 2
)

func (r *Room) BeforeCreate(tx *gorm.DB) (err error) {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}