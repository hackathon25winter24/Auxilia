package game

import "testing"

func TestLiberetteOnlyOwnSideActivates(t *testing.T) {
	for _, owner := range []string{"a", "b"} {
		s := NewPendingState("both", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"liberette", "jude", "tsukiha"}, {"liberette", "jude", "tsukiha"}})
		s.Turn = 1
		s.TurnPlayerID = owner
		actor := 0
		if owner == "b" {
			actor = 3
		}
		target := s.randomIndex(actor, "target", 6)
		effect := s.randomEffect(actor, true)
		s.applyTurnStartPassives()
		count := 0
		for _, c := range s.Characters {
			count += len(c.Effects)
		}
		if count != 1 || !s.hasEffect(target, effect) {
			t.Fatalf("wrong passive activation on %s turn", owner)
		}
	}
}
func TestLiberetteKnockedOutSourceDoesNotActivate(t *testing.T) {
	s := NewPendingState("dead", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"liberette", "jude", "tsukiha"}, {"liberette", "jude", "tsukiha"}})
	s.Turn = 1
	s.TurnPlayerID = "a"
	s.Characters[0].HP = 0
	s.applyTurnStartPassives()
	for _, c := range s.Characters {
		if len(c.Effects) != 0 {
			t.Fatal("dead or enemy source activated")
		}
	}
}
func TestBarrierOnlyConsumedForAttackedCharacter(t *testing.T) {
	s := NewState("barrier-target", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"louise", "jude", "tsukiha"}, {"chiyo", "jude", "tsukiha"}})
	s.TurnPlayerID = "b"
	s.Players[1].Cost = 50
	positions := []Position{{4, 2}, {3, 2}, {0, 0}, {2, 2}, {7, 4}, {7, 3}}
	for i := range s.Characters {
		s.Characters[i].Position = positions[i]
		s.Characters[i].Effects = nil
	}
	for _, i := range []int{0, 1} {
		s.Characters[i].Effects = []string{"結界"}
		s.Characters[i].BarrierTurn = s.Turn
	}
	attack := func() {
		t.Helper()
		err := s.ApplyAttack("b", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[3].ID, AttackIndex: 1, Direction: Position{1, 0}, Target: Position{3, 2}})
		if err != nil {
			t.Fatal(err)
		}
	}
	hp := s.Characters[1].HP
	attack()
	if s.Characters[1].HP != hp || s.hasEffect(1, "結界") {
		t.Fatal("victim barrier did not block exactly once")
	}
	if !s.hasEffect(0, "結界") {
		t.Fatal("unattacked ally barrier was consumed")
	}
	attack()
	if s.Characters[1].HP >= hp || !s.hasEffect(0, "結界") {
		t.Fatal("second hit/ally barrier incorrect")
	}
}
