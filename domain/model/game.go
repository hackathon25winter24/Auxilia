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
	IsTurnEnded    bool

	IsFinished     bool
	WinnerPlayerID *string
	FinishedAt     *time.Time

	Player1RateDelta int
	Player2RateDelta int
	Player1Rate      int
	Player2Rate      int

	Characters []UniqueCharacter `gorm:"foreignKey:RoomID;references:RoomID"`
	Grids      []Grid            `gorm:"foreignKey:RoomID;references:RoomID"`

	CurrentAction GameActionLog `gorm:"foreignKey:RoomID;references:RoomID"`
}

type Grid struct {
	ID            uint `gorm:"primaryKey"`
	RoomID        uint `gorm:"index"`
	PositionX     uint
	PositionY     uint
	GridType      int32
	RemainingTurn int32
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

type AttackInfoData struct {
	AttackedCharacterID uint
	NewHP               uint
}

type GameActionLog struct {
	ID        uint   `gorm:"primaryKey"`
	RoomID    uint   `gorm:"uniqueIndex:idx_room_seq"` // 部屋IDと通し番号の複合インデックス
	Sequence  uint   `gorm:"uniqueIndex:idx_room_seq"` // 💡 通し番号（1, 2, 3... と増えていく）
	PlayerID  string // 誰が実行したか
	DamageLog string // ダメージログ

	ActionType             string // "MOVE,ATTACK,EFFECT"
	ActorCharacterUniqueID uint   // 行動したキャラのID
	
	// 移動用
	ToX uint
	ToY uint

	// 攻撃用
	AttackType         int
	TargetCharacterIDs string 

	//特殊効果用
	EffectType         int    `gorm:"column:effect_type"`
	NewHP              uint   `gorm:"column:new_hp"`
}


var DefaultCharacterHPs = map[uint]int{
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
	10: 200,
}

var DefaultCharacterMoveCosts = map[uint]int{
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
	10: 10,
}

var CharacterData = map[uint]struct {
	Name string
	HP   int
	MoveCost int
	AttackCosts []int
}{
	0:  {"Sophie", 150, 10,[]int{10, 20, 50}},
	1:  {"Jude", 350, 10, []int{10, 20, 30}},
	2:  {"Nadia", 200, 7, []int{10, 20, 30}},
}