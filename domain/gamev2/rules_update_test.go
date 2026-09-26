package game

import "testing"

func TestUpdatedSuimaAndKasuimaRules(t *testing.T) {
	d, _ := Definition("suima")
	if d.Attacks[0].Power != 30 || d.AlternateAttacks[0].Power != 10 || d.AlternateAttacks[2].Power != -50 || !containsPosition(d.Attacks[2].Pattern, Position{1, 0}) {
		t.Fatal("suima rules mismatch")
	}
	d, _ = Definition("kasuima")
	if d.Attacks[1].Power != 30 || len(d.Attacks[1].Pattern) != 5 || d.Attacks[2].Power != 10 || !containsPosition(d.Attacks[2].Pattern, Position{3, 0}) {
		t.Fatal("kasuima rules mismatch")
	}
	s := NewState("alcohol", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"kasuima"}, {"tsukiha"}})
	s.TurnPlayerID = "a"
	s.Players[0].Cost = 50
	s.Characters[0].Position = Position{2, 2}
	s.Characters[1].Position = Position{4, 2}
	if err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, AttackIndex: 0, Target: Position{2, 2}, Direction: Position{1, 0}}); err != nil {
		t.Fatal(err)
	}
	s.processTurnEnd("a")
	if !s.hasEffect(0, "威力上昇") {
		t.Fatal("buff expired after one own turn")
	}
	s.Turn += 2
	s.processTurnEnd("a")
	if s.hasEffect(0, "威力上昇") {
		t.Fatal("buff did not expire after two own turns")
	}
	if s.Characters[0].HangoverTurn != s.Turn+2 || s.Characters[0].HangoverUntil != s.Turn+4 {
		t.Fatal("hangover scheduling")
	}
	s.Turn += 2
	s.addEffect(0, "二日酔い")
	s.processTurnEnd("a")
	if !s.hasEffect(0, "二日酔い") {
		t.Fatal("hangover expired early")
	}
	s.Turn += 2
	s.processTurnEnd("a")
	if s.hasEffect(0, "二日酔い") {
		t.Fatal("hangover did not expire")
	}
	s.Characters[0].Effects = []string{"毒", "鈍足", "毒"}
	s.Players[0].Cost = 50
	s.Characters[1].HP = 100
	if err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, AttackIndex: 1, Target: Position{4, 2}, Direction: Position{1, 0}}); err != nil {
		t.Fatal(err)
	}
	if s.Characters[1].HP != 50 {
		t.Fatalf("unique debuff bonus: hp=%d", s.Characters[1].HP)
	}
}
