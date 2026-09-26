package game

import (
	"encoding/json"
	"testing"
	"time"
)

func suimaState() *State {
	s := NewState("suima", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"suima", "shicho", "jude"}, {"shicho", "jude"}})
	s.TurnPlayerID = "a"
	positions := []Position{{2, 2}, {0, 4}, {3, 2}, {7, 4}, {4, 2}}
	for i := range s.Characters {
		s.Characters[i].Position = positions[i]
	}
	return s
}
func suimaAttack(s *State, attack int, target Position) error {
	return s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, AttackIndex: attack, Target: target, Direction: Position{1, 0}})
}
func TestSuimaAlternatesOnOwnTurnsAndTemporaryBuffsExpire(t *testing.T) {
	s := suimaState()
	if err := suimaAttack(s, 1, Position{2, 2}); err != nil {
		t.Fatal(err)
	}
	if !s.hasEffect(0, "俊足") || !s.hasEffect(0, "威力上昇") {
		t.Fatal("self buffs missing")
	}
	if err := s.EndTurn("a", s.Revision); err != nil {
		t.Fatal(err)
	}
	if s.hasEffect(0, "俊足") || s.hasEffect(0, "威力上昇") || !s.Characters[0].Wriggling {
		t.Fatal("end-turn cleanup/transition failed")
	}
	s.ExpireTurn(s.PhaseDeadline.Add(time.Millisecond))
	if err := s.EndTurn("b", s.Revision); err != nil {
		t.Fatal(err)
	}
	s.ExpireTurn(s.PhaseDeadline.Add(time.Millisecond))
	raw, _ := json.Marshal(s)
	if err := json.Unmarshal(raw, s); err != nil {
		t.Fatal(err)
	}
	_, d, err := s.actor("a", s.Characters[0].ID)
	if err != nil || d.Attacks[0].Name != ":wara:" {
		t.Fatal("alternate moves missing after reload")
	}
	if err := suimaAttack(s, 0, Position{3, 2}); err != nil {
		t.Fatal(err)
	}
	if s.hasEffect(2, "鈍化") || !s.hasEffect(4, "鈍化") {
		t.Fatal("wara must affect enemies only")
	}
	s.Characters[0].HP = 50
	if err := suimaAttack(s, 2, Position{2, 2}); err != nil {
		t.Fatal(err)
	}
	if s.Characters[0].HP != 100 {
		t.Fatal("recovery must heal 50")
	}
	if err := s.EndTurn("a", s.Revision); err != nil {
		t.Fatal(err)
	}
	if s.Characters[0].Wriggling {
		t.Fatal("did not return to active")
	}
}
func TestSuimaGlobalDamageAndEnemyTileRemoval(t *testing.T) {
	s := suimaState()
	if err := suimaAttack(s, 0, Position{3, 1}); err != nil {
		t.Fatal(err)
	}
	if s.Characters[1].HP != 40 || s.Characters[3].HP != 40 {
		t.Fatal("all Shicho must receive global 40")
	}
	s = suimaState()
	s.setTile(Position{4, 1}, "不変", "b")
	s.setTile(Position{5, 1}, "毒ガス", "b")
	s.setTile(Position{5, 2}, "地雷", "a")
	s.addEffect(4, "威力上昇")
	if err := suimaAttack(s, 2, Position{4, 1}); err != nil {
		t.Fatal(err)
	}
	if s.hasEffect(4, "威力上昇") || len(s.TileEffects) != 1 || s.TileEffects[0].OwnerID != "a" {
		t.Fatal("must clear enemy buffs/tiles only")
	}
}

func TestSuimaCleanseAndPreservePermanentBuff(t *testing.T) {
	s := suimaState()
	s.addEffect(0, "俊足")
	if err := suimaAttack(s, 1, Position{2, 2}); err != nil {
		t.Fatal(err)
	}
	s.processTurnEnd("a")
	if !s.hasEffect(0, "俊足") || s.hasEffect(0, "威力上昇") {
		t.Fatal("permanent buff incorrectly expired")
	}
	s.addEffect(0, "毒")
	s.addEffect(2, "毒")
	s.Characters[4].Position = Position{2, 3}
	s.addEffect(4, "毒")
	if err := suimaAttack(s, 1, Position{2, 2}); err != nil {
		t.Fatal(err)
	}
	if s.hasEffect(0, "毒") || s.hasEffect(2, "毒") || !s.hasEffect(4, "毒") {
		t.Fatal("cleanse must affect only allies including self")
	}
}
