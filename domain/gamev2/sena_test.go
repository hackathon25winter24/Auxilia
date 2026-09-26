package game

import "testing"

func TestSenaAttackRangesAndDebuffsMatchCharacterSpec(t *testing.T) {
	sena, ok := Definition("sena")
	if !ok {
		t.Fatal("星凪の定義がありません")
	}
	wantPatterns := [3][]Position{
		{{1, 0}, {2, 0}},
		{{2, -1}, {2, 0}, {2, 1}},
		{{2, 0}, {3, 0}},
	}
	wantEffects := [3]string{"出血", "", "出血"}
	wantChances := [3]int{50, 0, 10}
	for i, attack := range sena.Attacks {
		if len(attack.Pattern) != len(wantPatterns[i]) {
			t.Fatalf("星凪の攻撃%dの範囲=%v, want=%v", i+1, attack.Pattern, wantPatterns[i])
		}
		for cell := range attack.Pattern {
			if attack.Pattern[cell] != wantPatterns[i][cell] {
				t.Errorf("星凪の攻撃%dの範囲=%v, want=%v", i+1, attack.Pattern, wantPatterns[i])
				break
			}
		}
		if attack.Effect != wantEffects[i] || attack.EffectChance != wantChances[i] {
			t.Errorf("星凪の攻撃%dのデバフ=%s %d%%, want=%s %d%%", i+1, attack.Effect, attack.EffectChance, wantEffects[i], wantChances[i])
		}
	}
}
