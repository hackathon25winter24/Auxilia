package game

type AttackDefinition struct {
	OncePerTurn  bool       `json:"oncePerTurn,omitempty"`
	Name         string     `json:"name"`
	Cost         int        `json:"cost"`
	Power        int        `json:"power"`
	Range        int        `json:"range"`
	Target       string     `json:"target"`
	Pattern      []Position `json:"pattern"`
	Effect       string     `json:"effect,omitempty"`
	EffectChance int        `json:"effectChance,omitempty"`
	Tile         string     `json:"tile,omitempty"`
	ClearDebuffs bool       `json:"clearDebuffs,omitempty"`
	ClearBuffs   bool       `json:"clearBuffs,omitempty"`
	AllyEffect   string     `json:"allyEffect,omitempty"`
	Description  string     `json:"description,omitempty"`
}
type CharacterDefinition struct {
	AlternateAttacks   *[3]AttackDefinition `json:"alternateAttacks,omitempty"`
	ID                 string               `json:"id"`
	Name               string               `json:"name"`
	Image              string               `json:"image"`
	Portrait           string               `json:"portrait"`
	MaxHP              int                  `json:"maxHP"`
	MoveCost           int                  `json:"moveCost"`
	MoveRange          int                  `json:"moveRange"`
	PassiveName        string               `json:"passiveName"`
	PassiveDescription string               `json:"passiveDescription"`
	Attacks            [3]AttackDefinition  `json:"attacks"`
}

func described(a AttackDefinition, text string) AttackDefinition { a.Description = text; return a }

func p(points ...Position) []Position { return points }
func allyBuff(a AttackDefinition, effect string) AttackDefinition {
	a.AllyEffect = effect
	return a
}
func atk(name string, cost, power int, target string, pattern []Position) AttackDefinition {
	r := 0
	for _, cell := range pattern {
		if d := abs(cell.X) + abs(cell.Y); d > r {
			r = d
		}
	}
	return AttackDefinition{Name: name, Cost: cost, Power: power, Range: r, Target: target, Pattern: pattern}
}

var adjacent = p(Position{1, 0})
var Definitions = []CharacterDefinition{
	{ID: "suima", Name: "睡魔", Image: "Suima_mini.png", Portrait: "Suima.png", MaxHP: 160, MoveCost: 10, MoveRange: 1,
		Attacks: [3]AttackDefinition{
			func() AttackDefinition {
				a := atk("進捗を錬成", 20, 30, "enemy", p(Position{1, -1}, Position{1, 0}, Position{2, 0}, Position{1, 1}))
				a.Description = "範囲への攻撃に加え、敵味方すべての新著久無子に範囲を問わず40ダメージ。"
				return a
			}(),
			func() AttackDefinition {
				a := atk(":star_struck:", 10, 0, "ally", p(Position{0, 0}))
				a.Description = "使用したターン中、自身に俊足と威力上昇を付与する。"
				return a
			}(),
			func() AttackDefinition {
				a := atk("それはよくないとされている", 20, 10, "enemy", p(Position{2, -1}, Position{3, -1}, Position{1, 0}, Position{2, 0}, Position{3, 0}, Position{2, 1}, Position{3, 1}))
				a.ClearBuffs = true
				a.Description = "範囲内の敵のバフと、敵が設置したマスを取り除く。"
				return a
			}(),
		}, AlternateAttacks: &[3]AttackDefinition{
			func() AttackDefinition {
				a := atk(":wara:", 20, 10, "enemy", p(Position{1, -1}, Position{2, -1}, Position{3, -1}, Position{1, 0}, Position{2, 0}, Position{3, 0}, Position{1, 1}, Position{2, 1}, Position{3, 1}))
				a.Effect = "鈍化"
				a.EffectChance = 100
				return a
			}(),
			func() AttackDefinition {
				a := atk("ン！俺が悪い", 20, 0, "ally", p(Position{-1, -1}, Position{0, -1}, Position{1, -1}, Position{-1, 0}, Position{0, 0}, Position{1, 0}, Position{-1, 1}, Position{0, 1}, Position{1, 1}))
				a.ClearDebuffs = true
				return a
			}(),
			atk("一旦寝るか", 15, -50, "ally", p(Position{0, 0})),
		}},
	{ID: "kasuima", Name: "カスイマ", Image: "Kasuima_mini.png", Portrait: "Kasuima.png", MaxHP: 150, MoveCost: 15, MoveRange: 1, Attacks: [3]AttackDefinition{
		func() AttackDefinition {
			a := atk("酒", 10, 0, "ally", p(Position{0, 0}))
			a.Description = "自身に自分の手番2回分の威力上昇を付与。終了後、次の自分の手番から2回分の二日酔い（攻撃力20%低下、移動・攻撃コスト各5増加）を付与する。"
			return a
		}(),
		atk("煙草", 10, 30, "enemy", p(Position{2, -1}, Position{3, -1}, Position{2, 0}, Position{2, 1}, Position{3, 1})),
		func() AttackDefinition {
			a := atk("Reverse", 20, 10, "enemy", p(Position{1, -1}, Position{2, -1}, Position{1, 0}, Position{2, 0}, Position{3, 0}, Position{1, 1}, Position{2, 1}))
			a.Description = "命中したキャラを攻撃方向に2マス押し戻す。移動先が無効なら1マス、そこも無効なら移動しない。"
			return a
		}(),
	}},
	{ID: "verbulus", Name: "ウェルブルス", Image: "Verbulus_mini.png", Portrait: "Verbulus.png", MaxHP: 150, MoveCost: 5, MoveRange: 1, Attacks: [3]AttackDefinition{
		func() AttackDefinition {
			a := atk("栄枯盛衰", 20, 30, "enemy", p(Position{-1, -1}, Position{0, -1}, Position{1, -1}, Position{-1, 0}, Position{1, 0}, Position{-1, 1}, Position{0, 1}, Position{1, 1}))
			a.ClearBuffs = true
			return a
		}(),
		func() AttackDefinition {
			a := atk("永久不変", 20, 0, "cell", adjacent)
			a.Tile = "不変"
			return a
		}(),
		atk("千変万化", 25, -50, "ally", p(Position{0, 0}, Position{-1, 0}, Position{1, 0}, Position{0, -1}, Position{0, 1})),
	}},
	{ID: "sophie", Name: "ソフィー", Image: "Sophie_mini.png", Portrait: "Sophie.png", MaxHP: 100, MoveCost: 10, MoveRange: 2, Attacks: [3]AttackDefinition{atk("突き growth～成長～", 10, 20, "enemy", p(Position{1, 0}, Position{2, 0})), atk("範囲狙撃 bloom～開花～", 20, 80, "enemy", p(Position{2, 0}, Position{3, -1}, Position{3, 0}, Position{3, 1})), atk("集中狙撃 fruit～結実～", 50, 250, "enemy", p(Position{3, 0}))}},
	{ID: "jude", Name: "ジュード", Image: "Jude_mini.png", Portrait: "Jude.png", MaxHP: 250, MoveCost: 10, MoveRange: 2, Attacks: [3]AttackDefinition{func() AttackDefinition {
		a := atk("急襲", 20, 20, "enemy", adjacent)
		a.Effect = "出血"
		a.EffectChance = 30
		return a
	}(), atk("切り裂き", 20, 50, "enemy", adjacent), func() AttackDefinition {
		a := atk("応急手当", 30, -30, "ally", p(Position{0, 0}))
		a.ClearDebuffs = true
		return a
	}()}},
	{ID: "nadia", Name: "ナディア", Image: "Nadia_mini.png", Portrait: "Nadia.png", MaxHP: 200, MoveCost: 7, MoveRange: 3, Attacks: [3]AttackDefinition{func() AttackDefinition {
		a := atk("対処番号05：対包囲戦術", 10, 20, "enemy", p(Position{1, 0}, Position{0, 1}, Position{0, -1}, Position{-1, 0}))
		a.Effect = "毒"
		a.EffectChance = 20
		return a
	}(), func() AttackDefinition {
		a := atk("対処番号03：前方範囲殲滅", 20, 40, "enemy", p(Position{1, -1}, Position{1, 0}, Position{1, 1}))
		a.Effect = "毒"
		a.EffectChance = 40
		return a
	}(), func() AttackDefinition {
		a := atk("対処番号02：前方殲滅・改", 30, 60, "enemy", adjacent)
		a.Effect = "毒"
		a.EffectChance = 60
		return a
	}()}},
	{ID: "tsukiha", Name: "月葉", Image: "Tsukiha_mini.png", Portrait: "Tsukiha.png", MaxHP: 100, MoveCost: 3, MoveRange: 4, Attacks: [3]AttackDefinition{func() AttackDefinition {
		a := atk("忍法：手裏剣投げの術・近", 4, 10, "enemy", p(Position{1, -1}, Position{2, 0}, Position{1, 1}))
		a.Effect = "出血"
		a.EffectChance = 30
		return a
	}(), func() AttackDefinition {
		a := atk("忍法：手裏剣投げの術・遠", 6, 10, "enemy", p(Position{2, -1}, Position{3, 0}, Position{2, 1}))
		a.Effect = "出血"
		a.EffectChance = 20
		return a
	}(), func() AttackDefinition {
		a := atk("忍法：まきびし投げの術", 10, 0, "cell", adjacent)
		a.Tile = "まきびし"
		return a
	}()}},
	{ID: "aoi", Name: "扇衣", Image: "Aoi_mini.png", Portrait: "Aoi.png", MaxHP: 250, MoveCost: 8, MoveRange: 2, Attacks: [3]AttackDefinition{allyBuff(atk("汐汲～しおくみ～", 20, 50, "enemy", p(Position{1, 0}, Position{0, 1}, Position{0, -1})), "俊敏化"), allyBuff(atk("女伊達～おんなだて～", 30, 0, "ally", p(Position{-1, -1}, Position{0, -1}, Position{1, -1}, Position{-1, 0}, Position{1, 0}, Position{-1, 1}, Position{0, 1}, Position{1, 1})), "威力上昇"), atk("鷺娘～さぎむすめ～", 20, -30, "ally", p(Position{-1, -1}, Position{0, -1}, Position{1, -1}, Position{-1, 0}, Position{0, 0}, Position{1, 0}, Position{-1, 1}, Position{0, 1}, Position{1, 1}))}},
	{ID: "sena", Name: "星凪", Image: "Sena_mini.png", Portrait: "Sena.png", MaxHP: 150, MoveCost: 10, MoveRange: 2, Attacks: [3]AttackDefinition{func() AttackDefinition {
		a := atk("一条流槍術：衝き", 15, 40, "enemy", p(Position{1, 0}, Position{2, 0}))
		a.Effect = "出血"
		a.EffectChance = 50
		return a
	}(), atk("一条流槍術：掃い", 20, 60, "enemy", p(Position{2, -1}, Position{2, 0}, Position{2, 1})), func() AttackDefinition {
		a := atk("一条流槍術：薙ぎ", 30, 90, "enemy", p(Position{2, 0}, Position{3, 0}))
		a.Effect = "出血"
		a.EffectChance = 10
		return a
	}()}},
	{ID: "berenice", Name: "ベレニス", Image: "berenice_mini.png", Portrait: "Berenice.png", MaxHP: 200, MoveCost: 7, MoveRange: 2, Attacks: [3]AttackDefinition{func() AttackDefinition {
		a := atk("地雷設置", 10, 0, "cell", adjacent)
		a.Tile = "地雷"
		return a
	}(), atk("爆破！", 30, 60, "enemy", p(Position{1, 0}, Position{2, -1}, Position{2, 0}, Position{2, 1}, Position{3, 0})), atk("小型爆弾", 20, 50, "enemy", p(Position{1, -1}, Position{1, 0}, Position{1, 1}, Position{2, 0}))}},
	{ID: "chiyo", Name: "千代", Image: "Chiyo_mini.png", Portrait: "Chiyo.png", MaxHP: 150, MoveCost: 5, MoveRange: 3, Attacks: [3]AttackDefinition{atk("一文字斬り", 10, 20, "enemy", p(Position{1, -1}, Position{1, 0}, Position{1, 1})), func() AttackDefinition {
		a := atk("袈裟斬り", 20, 60, "enemy", adjacent)
		a.Effect = "出血"
		a.EffectChance = 50
		return a
	}(), atk("真向斬り", 50, 220, "enemy", adjacent)}},
	{ID: "shicho", Name: "新著", Image: "Shicho_mini.png", Portrait: "Shicho.png", MaxHP: 80, MoveCost: 15, MoveRange: 2, Attacks: [3]AttackDefinition{atk("進捗どうですか？", 20, 240, "any", p(Position{-2, 0}, Position{-1, -1}, Position{-1, 0}, Position{-1, 1}, Position{0, -2}, Position{0, -1}, Position{0, 0}, Position{0, 1}, Position{0, 2}, Position{1, -1}, Position{1, 0}, Position{1, 1}, Position{2, 0})), atk(":oyoo:", 10, -40, "any", p(Position{-1, 0}, Position{0, -1}, Position{0, 0}, Position{0, 1}, Position{1, 0})), func() AttackDefinition {
		a := atk(":iihanashi:", 10, 0, "any", p(Position{-1, 0}, Position{0, -1}, Position{0, 0}, Position{0, 1}, Position{1, 0}))
		a.ClearDebuffs = true
		return a
	}()}},
	{ID: "zina", Name: "ジーナ", Image: "Zina_mini.png", Portrait: "Zina.png", MaxHP: 200, MoveCost: 6, MoveRange: 3, Attacks: [3]AttackDefinition{func() AttackDefinition {
		a := atk("遠距離制圧", 20, 30, "enemy", p(Position{3, 0}))
		a.Effect = "麻痺"
		a.EffectChance = 40
		return a
	}(), func() AttackDefinition {
		a := atk("中距離制圧", 20, 20, "enemy", p(Position{2, 0}))
		a.Effect = "麻痺"
		a.EffectChance = 80
		return a
	}(), atk("軍隊式近接格闘術", 30, 60, "enemy", adjacent)}},
	{ID: "dana", Name: "ダーナ", Image: "Dana_mini.png", Portrait: "Dana.png", MaxHP: 200, MoveCost: 9, MoveRange: 2, Attacks: [3]AttackDefinition{func() AttackDefinition {
		a := atk("残留型毒ガス", 10, 0, "cell", adjacent)
		a.Tile = "毒ガス"
		return a
	}(), func() AttackDefinition {
		a := atk("拡散型毒ガス", 20, 20, "enemy", p(Position{1, 0}, Position{2, -1}, Position{2, 0}, Position{2, 1}, Position{3, 0}))
		a.Effect = "毒"
		a.EffectChance = 100
		return a
	}(), atk("活性化ガス", 20, -30, "ally", p(Position{0, 0}))}},
	{ID: "louise", Name: "ルイース", Image: "Louise_mini.png", Portrait: "Louise.png", MaxHP: 100, MoveCost: 5, MoveRange: 1,
		Attacks: [3]AttackDefinition{
			described(atk("退魔の舞", 30, 0, "ally", p(Position{0, 0})), "味方全体のデバフを解除し、免疫を付与する。"),
			described(atk("守護の舞", 30, 0, "ally", p(Position{0, 0})), "自身以外の味方全体を50回復し、自身を含む味方全体に結界を付与する。"),
			described(atk("降納", 25, 0, "ally", p(Position{0, 0})), "自身を臨戦状態にする。"),
		}, AlternateAttacks: &[3]AttackDefinition{
			atk("突撃の踏", 20, 70, "enemy", p(Position{1, 0}, Position{2, 0}, Position{3, 0})),
			atk("薙ぎの踏", 20, 70, "enemy", p(Position{1, -1}, Position{1, 0}, Position{1, 1})),
			described(atk("掲揚", 25, 0, "ally", p(Position{0, 0})), "自身を支援状態にする。"),
		}},
	{ID: "liberette", Name: "リベレット", Image: "Liberette_mini.png", Portrait: "Liberette.png", MaxHP: 150, MoveCost: 8, MoveRange: 1,
		Attacks: [3]AttackDefinition{
			func() AttackDefinition {
				a := atk("ハートの４", 25, -30, "ally", p(Position{0, 0}))
				a.ClearDebuffs = true
				return a
			}(),
			described(atk("クラブの８", 10, 0, "ally", p(Position{0, 0})), "自身にランダムなバフを1つ付与する。"),
			described(atk("ダイヤの10", 20, 30, "enemy", p(Position{2, -1}, Position{1, 0}, Position{2, 0}, Position{3, 0}, Position{2, 1})), "自身にバフがあればダメージ+60。攻撃後、自身の全バフを解除する。"),
		}},
}

var passiveDefinitions = map[string][2]string{
	"louise":    {"援護の舞踏", "周囲1マス以内の自身以外の味方のパッシブ効果値を20上昇させる。"},
	"liberette": {"スペードのエース", "自分のターン開始時、生存中の敵味方全員（自身を含む）から1人を選び、ランダムなバフまたはデバフを1つ付与する。"},
	"suima":     {"やる気の波", "自分のターンごとに活動状態とくねくね状態を交互に繰り返す。"},
	"kasuima":   {"カス", "自身の「酒」による威力上昇以外のバフを受けない。自身のデバフ1種類につき「煙草」と「Reverse」のダメージが10上昇する。"},
	"verbulus":  {"輪廻転生", "戦闘中に一度だけ、戦闘不能になったとき全てのバフ・デバフを解除し、HP50で復活する。"},
	"sophie":    {"範囲支援 sowing～播種～", "戦闘開始時に味方全体に俊足を与え、戦闘離脱時に敵全体に鈍足を与える。"},
	"jude":      {"受け身", "自身が受けるダメージを20軽減する。"},
	"nadia":     {"対処番号04：過量使用", "命中した敵に、ダメージを半分にしてコストを消費せず同じ攻撃を必ず1回追加する。追撃から再追撃は発生しない。"},
	"tsukiha":   {"忍法：隠れ身の術", "デバフマスの影響を受けない。"},
	"aoi":       {"藤娘～ふじむすめ～", "自身のターン終了時、周囲1マス以内にいる自身以外の味方のHPを30回復する。"},
	"sena":      {"一条流槍術：翻弄", "敵のパッシブによるダメージ軽減を無視して攻撃する。"},
	"berenice":  {"爆弾処理", "地雷マスのダメージを受けない。攻撃範囲の地雷を取り除き、個数×10だけ攻撃ダメージを増加する。"},
	"chiyo":     {"刀剣拝見", "HPが最大のとき、攻撃ダメージを50上昇させる。"},
	"shicho":    {":ganbare-:", "周囲1マス以内の味方（自身を含む）の攻撃ダメージを10上昇させ、自身のターン終了時に範囲内の味方のHPを10回復する。"},
	"zina":      {"弱体拡張戦術", "攻撃対象がデバフを持っている場合、その対象への攻撃ダメージが2倍になる。"},
	"dana":      {"毒物耐性", "デバフの影響を受けない。デバフマスによるダメージや移動コスト増加は受ける。"},
}

// PassiveValues contains every numeric value used by a character passive.
// Keeping these beside the base stats and attacks makes character balancing a
// data-only change instead of requiring edits to the battle engine.
type PassiveValues struct {
	ExtraAttackDamagePercent int
	DebuffedDamageMultiplier int
	AttackBoost              int
	DamageReduction          int
	ExtraAttackChance        int
	FullHPAttackBoost        int
	TurnHeal                 int
	PassiveValueBoost        int
	IgnorePassiveReduce      bool
	ExcludeSelf              bool
}

var passiveValues = map[string]PassiveValues{
	"louise": {PassiveValueBoost: 20},
	"jude":   {DamageReduction: 20},
	"nadia":  {ExtraAttackChance: 100, ExtraAttackDamagePercent: 50},
	"aoi":    {TurnHeal: 30, ExcludeSelf: true},
	"sena":   {IgnorePassiveReduce: true},
	"chiyo":  {FullHPAttackBoost: 50},
	"shicho": {AttackBoost: 10, TurnHeal: 10},
	"zina":   {DebuffedDamageMultiplier: 2},
}

func passiveFor(id string) PassiveValues { return passiveValues[id] }

func init() {
	for i := range Definitions {
		for j := range Definitions[i].Attacks {
			a := &Definitions[i].Attacks[j]
			if (Definitions[i].ID == "kasuima" && j == 0) || (Definitions[i].ID == "suima" && j == 1) {
				a.OncePerTurn = true
			}
		}
		if Definitions[i].ID == "suima" && Definitions[i].AlternateAttacks != nil {
			Definitions[i].AlternateAttacks[2].OncePerTurn = true
		}
		passive := passiveDefinitions[Definitions[i].ID]
		Definitions[i].PassiveName = passive[0]
		Definitions[i].PassiveDescription = passive[1]
	}
}

func Definition(id string) (CharacterDefinition, bool) {
	id = CanonicalCharacterID(id)
	for _, d := range Definitions {
		if d.ID == id {
			return d, true
		}
	}
	return CharacterDefinition{}, false
}
