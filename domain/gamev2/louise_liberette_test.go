package game

import (
	"encoding/json"
	"testing"
)

func dancerState(id string) *State {
	s := NewState("dancers", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{id, "jude", "tsukiha"}, {"jude", "tsukiha", "zina"}})
	s.TurnPlayerID = "a"
	s.Players[0].Cost = 50
	positions := []Position{{2, 2}, {1, 2}, {0, 0}, {3, 2}, {7, 4}, {7, 3}}
	for i := range s.Characters {
		s.Characters[i].Position = positions[i]
		s.Characters[i].Effects = nil
	}
	return s
}
func dancerAttack(s *State, index int, target Position) error {
	return s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, AttackIndex: index, Target: target, Direction: Position{1, 0}})
}
func TestLouiseSupportAndStance(t *testing.T) {
	s := dancerState("louise")
	d, _ := Definition("louise")
	if d.MaxHP != 100 || d.MoveCost != 5 || d.PassiveName != "援護の舞踏" {
		t.Fatal(d)
	}
	if s.passiveBoost(0) != 0 || s.passiveBoost(1) != 20 || s.passiveBoost(2) != 0 || s.passiveBoost(3) != 0 {
		t.Fatal("passive range/owner/self filter")
	}
	for i := range s.Characters {
		s.Characters[i].Effects = []string{"毒", "出血"}
	}
	if err := dancerAttack(s, 0, s.Characters[0].Position); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if s.hasEffect(i, "毒") || !s.hasEffect(i, "免疫") {
			t.Fatal("purification", i)
		}
	}
	if !s.hasEffect(3, "毒") {
		t.Fatal("enemy cleansed")
	}
	s.addEffect(0, "鈍足")
	if s.hasEffect(0, "鈍足") || s.hasEffect(0, "免疫") {
		t.Fatal("immunity not consumed")
	}
	s.Players[0].Cost = 50
	s.Characters[0].HP = 40
	s.Characters[1].HP = 100
	if err := dancerAttack(s, 1, s.Characters[0].Position); err != nil {
		t.Fatal(err)
	}
	if s.Characters[0].HP != 40 || s.Characters[1].HP != 150 {
		t.Fatal("healing targets")
	}
	if s.consumeBarrier(0) {
		t.Fatal("barrier active too early")
	}
	s.Turn++
	if !s.consumeBarrier(0) || s.consumeBarrier(0) {
		t.Fatal("barrier must block once")
	}
	s.Players[0].Cost = 50
	if err := dancerAttack(s, 2, s.Characters[0].Position); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(s)
	if err := json.Unmarshal(raw, s); err != nil {
		t.Fatal(err)
	}
	_, active, err := s.actor("a", s.Characters[0].ID)
	if err != nil || !s.Characters[0].CombatStance || active.Attacks[0].Power != 70 || len(active.Attacks[0].Pattern) != 3 {
		t.Fatal("stance not persisted")
	}
	if err := dancerAttack(s, 2, s.Characters[0].Position); err != nil {
		t.Fatal(err)
	}
	if s.Characters[0].CombatStance {
		t.Fatal("stance not restored")
	}
}
func TestLiberetteCards(t *testing.T) {
	s := dancerState("liberette")
	s.Characters[0].HP = 70
	s.Characters[0].Effects = []string{"毒", "出血"}
	if err := dancerAttack(s, 0, s.Characters[0].Position); err != nil {
		t.Fatal(err)
	}
	if s.Characters[0].HP != 100 || len(s.Characters[0].Effects) != 0 {
		t.Fatal("heart heal/cleanse")
	}
	if err := dancerAttack(s, 1, s.Characters[0].Position); err != nil {
		t.Fatal(err)
	}
	if len(s.Characters[0].Effects) != 1 || !s.hasBuff(0) {
		t.Fatal("club random buff")
	}
	for _, buffed := range []bool{false, true} {
		s = dancerState("liberette")
		s.Characters[3].DefinitionID = "tsukiha" // No damage reduction passive.
		if buffed {
			s.Characters[0].Effects = []string{"免疫", "結界", "俊足"}
		}
		before := s.Characters[3].HP
		if err := dancerAttack(s, 2, s.Characters[3].Position); err != nil {
			t.Fatal(err)
		}
		want := 30
		if buffed {
			want = 90
		}
		if before-s.Characters[3].HP != want || s.hasBuff(0) {
			t.Fatal("diamond power/consumption", before-s.Characters[3].HP)
		}
	}
}
func TestLiberetteTurnStartAndRandomPool(t *testing.T) {
	s := dancerState("liberette")
	s.applyTurnStartPassives()
	count := 0
	for _, c := range s.Characters {
		count += len(c.Effects)
	}
	if count != 1 {
		t.Fatal("passive must grant exactly one effect", count)
	}
	seen := map[string]bool{}
	for r := 0; r < 500; r++ {
		s.Revision = uint64(r)
		seen[s.randomEffect(0, true)] = true
	}
	for _, effect := range append(append([]string{}, randomBuffs...), randomDebuffs...) {
		if !seen[effect] {
			t.Fatal("unreachable random effect", effect)
		}
	}
}
