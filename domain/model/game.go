package model

type GameData struct {
	ID        uint `gorm:"primaryKey"`
	RoomID    uint `gorm:"uniqueIndex"`

	Player1ID string
	Player2ID string
	BaseHP1   uint
	BaseHP2   uint
	Turn      uint

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