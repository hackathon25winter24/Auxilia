package game

import "testing"

func TestCharacterDefinitionsMatchCharacterSpec(t *testing.T) {
	type attackNumbers struct {
		cost, power, effectChance int
	}
	type characterNumbers struct {
		hp, moveCost int
		attacks      [3]attackNumbers
	}

	// Auxilia-web/specification/rules.md の個別説明を基準とする。
	// 回復量は負数で表す。効果・範囲はアクションテストでも検証する。
	want := map[string]characterNumbers{
		"suima":     {160, 10, [3]attackNumbers{{20, 30, 0}, {10, 0, 0}, {20, 10, 0}}},
		"kasuima":   {150, 15, [3]attackNumbers{{10, 0, 0}, {10, 30, 0}, {20, 10, 0}}},
		"verbulus":  {150, 5, [3]attackNumbers{{20, 30, 0}, {20, 0, 0}, {25, -50, 0}}},
		"sophie":    {100, 10, [3]attackNumbers{{10, 20, 0}, {20, 80, 0}, {50, 250, 0}}},
		"jude":      {250, 10, [3]attackNumbers{{20, 20, 30}, {20, 50, 0}, {30, -30, 0}}},
		"nadia":     {200, 7, [3]attackNumbers{{10, 20, 20}, {20, 40, 40}, {30, 60, 60}}},
		"tsukiha":   {100, 3, [3]attackNumbers{{4, 10, 30}, {6, 10, 20}, {10, 0, 0}}},
		"aoi":       {250, 8, [3]attackNumbers{{20, 50, 0}, {30, 0, 0}, {20, -30, 0}}},
		"sena":      {150, 10, [3]attackNumbers{{15, 40, 50}, {20, 60, 0}, {30, 90, 10}}},
		"berenice":  {200, 7, [3]attackNumbers{{10, 0, 0}, {30, 60, 0}, {20, 50, 0}}},
		"chiyo":     {150, 5, [3]attackNumbers{{10, 20, 0}, {20, 60, 50}, {50, 220, 0}}},
		"shicho":    {80, 15, [3]attackNumbers{{20, 240, 0}, {10, -40, 0}, {10, 0, 0}}},
		"zina":      {200, 6, [3]attackNumbers{{20, 30, 40}, {20, 20, 80}, {30, 60, 0}}},
		"louise":    {100, 5, [3]attackNumbers{{30, 0, 0}, {30, 0, 0}, {25, 0, 0}}},
		"liberette": {150, 8, [3]attackNumbers{{25, -30, 0}, {10, 0, 0}, {20, 30, 0}}},
		"dana":      {200, 9, [3]attackNumbers{{10, 0, 0}, {20, 20, 100}, {20, -30, 0}}},
	}

	if len(Definitions) != len(want) {
		t.Fatalf("character count = %d, want %d", len(Definitions), len(want))
	}
	for id, expected := range want {
		got, ok := Definition(id)
		if !ok {
			t.Errorf("definition %q is missing", id)
			continue
		}
		if got.MaxHP != expected.hp || got.MoveCost != expected.moveCost {
			t.Errorf("%s base stats = HP %d / move cost %d, want HP %d / move cost %d", id, got.MaxHP, got.MoveCost, expected.hp, expected.moveCost)
		}
		for i, attack := range got.Attacks {
			attackWant := expected.attacks[i]
			if attack.Cost != attackWant.cost || attack.Power != attackWant.power || attack.EffectChance != attackWant.effectChance {
				t.Errorf("%s attack %d = cost %d / power %d / chance %d, want %d / %d / %d", id, i+1, attack.Cost, attack.Power, attack.EffectChance, attackWant.cost, attackWant.power, attackWant.effectChance)
			}
		}
	}
}

func TestPassiveNumericValuesMatchCharacterSpec(t *testing.T) {
	tests := map[string]PassiveValues{
		"sophie": {},
		"jude":   {DamageReduction: 20},
		"nadia":  {ExtraAttackChance: 100, ExtraAttackDamagePercent: 50},
		"aoi":    {TurnHeal: 30, ExcludeSelf: true},
		"sena":   {IgnorePassiveReduce: true},
		"chiyo":  {FullHPAttackBoost: 50},
		"shicho": {AttackBoost: 10, TurnHeal: 10},
		"zina":   {DebuffedDamageMultiplier: 2},
	}
	for id, want := range tests {
		if got := passiveFor(id); got != want {
			t.Errorf("%s passive values = %+v, want %+v", id, got, want)
		}
	}
}
