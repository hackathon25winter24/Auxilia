package model

import (
	"time"
)

type GameData struct {
	ID     uint `gorm:"primaryKey"`
	RoomID uint `gorm:"uniqueIndex"`

	Player1ID      string
	Player2ID      string
	BaseHP1        uint
	BaseHP2        uint
	Cost1P         uint
	Cost2P         uint
	Turn           uint
	Is1PTurn       bool `gorm:"column:is_1p_turn"`
	TurnStartAt    time.Time
	IsFinished     bool
	WinnerPlayerID *string
	FinishedAt     *time.Time

	Characters []UniqueCharacter `gorm:"foreignKey:RoomID;references:RoomID"`
	Grids      []Grid            `gorm:"foreignKey:RoomID;references:RoomID"`
}

type Grid struct {
	ID        uint `gorm:"primaryKey"`
	RoomID    uint `gorm:"index"`
	PositionX uint
	PositionY uint
	GridType  int32
}

// TableName overrides the table name used by GORM
func (Grid) TableName() string {
	return "game_grids"
}

type UniqueCharacter struct {
	ID          uint `gorm:"primaryKey"`
	RoomID      uint `gorm:"index"`
	Is1P        bool
	IsSelected  bool `gorm:"not null;default:false"`
	CharacterID uint

	HP        uint
	PositionX uint
	PositionY uint

	Conditions []CharacterCondition `gorm:"foreignKey:UniqueCharacterID"`
}

type CharacterCondition struct {
	ID uint `gorm:"primaryKey"`

	UniqueCharacterID uint `gorm:"index"`
	ConditionID       int
	LastingTurn       int
}

type Position struct {
	X uint
	Y uint
}

var DefaultPoints1P = []Position{
	{X: 0, Y: 0},
	{X: 1, Y: 2},
	{X: 0, Y: 4},
}

var DefaultPoints2P = []Position{
	{X: 7, Y: 0},
	{X: 6, Y: 2},
	{X: 7, Y: 4},
}

var CharacterHPs = map[uint]int{
	0: 150,
	1: 300,
	2: 150,
	3: 100,
	4: 250,
	5: 250,
	6: 200,
	7: 300,
	8: 100,
	9: 200,
}

var CharacterMoveCosts = map[uint]int{
	0: 10,
	1: 10,
	2: 7,
	3: 3,
	4: 10,
	5: 10,
	6: 10,
	7: 5,
	8: 10,
	9: 5,
}
