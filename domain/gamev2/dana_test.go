package game

import (
	"fmt"
	"testing"
)

func TestDanaSpreadAlwaysPoisons(t *testing.T) {
	for seed := 0; seed < 100; seed++ {
		s := NewState(fmt.Sprintf("dana-%d", seed), [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"dana"}, {"tsukiha"}})
		s.TurnPlayerID = "a"
		s.Players[0].Cost = 50
		s.Characters[0].Position = Position{2, 2}
		s.Characters[1].Position = Position{3, 2}
		err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, AttackIndex: 1, Target: Position{3, 2}, Direction: Position{1, 0}})
		if err != nil {
			t.Fatal(err)
		}
		if !s.hasEffect(1, "毒") {
			t.Fatalf("seed %d did not poison", seed)
		}
	}
}
