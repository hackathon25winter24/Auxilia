package model

import "time"

const (
	AttackBy1P int32 = 1
	AttackBy2P int32 = 2
)

const (
	AttackType0 int32 = 0
	AttackType1 int32 = 1
	AttackType2 int32 = 2
	AttackType3 int32 = 3
)

type AttackInfo struct {
	ID uint `gorm:"primaryKey"`

	RoomID uint `gorm:"index;not null"`

	AttackerSide int32 `gorm:"not null"`
	IsStarted    bool  `gorm:"not null;default:false"`

	AttackerCharacterID *uint `gorm:"index"`

	AttackType int32 `gorm:"not null;default:0"`

	AttackedAt time.Time `gorm:"autoCreateTime"`
}
