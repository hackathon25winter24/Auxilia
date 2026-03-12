package model

import(
	"time"
)

type GameData struct {
	ID        uint `gorm:"primaryKey"`
	RoomID    uint `gorm:"uniqueIndex"`

	Player1ID string
	Player2ID string
	BaseHP1   uint
	BaseHP2   uint
	Turn      uint
	Is1PTurn   bool
	TurnStartAt time.Time

	Characters []UniqueCharacter `gorm:"foreignKey:RoomID;references:RoomID"`
}

type UniqueCharacter struct {
	ID          uint `gorm:"primaryKey"`
	RoomID      uint `gorm:"index"`
	Is1P        bool
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


