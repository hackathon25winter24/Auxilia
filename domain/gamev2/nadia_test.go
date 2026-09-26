package game

import (
	"fmt"
	"testing"
)

func TestNadiaGuaranteedHalfDamageFollowUp(t *testing.T) {
	for attack, base := range []int{20, 40, 60} {
		for seed := 0; seed < 100; seed++ {
			s := NewState(fmt.Sprintf("nadia-%d", seed), [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"nadia", "tsukiha", "zina"}, {"tsukiha", "zina", "sena"}})
			s.TurnPlayerID = "a"
			s.Players[0].Cost = 50
			positions := []Position{{2, 2}, {0, 0}, {0, 4}, {3, 2}, {7, 4}, {7, 3}}
			for i := range s.Characters {
				s.Characters[i].Position = positions[i]
				s.Characters[i].Effects = nil
			}
			s.Characters[3].MaxHP = 300
			s.Characters[3].HP = 300
			err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, AttackIndex: attack, Target: Position{3, 2}, Direction: Position{1, 0}})
			if err != nil {
				t.Fatal(err)
			}
			if got := 300 - s.Characters[3].HP; got != base+base/2 {
				t.Fatalf("attack %d seed %d: damage %d", attack, seed, got)
			}
			if s.Players[0].Cost != 50-(attack+1)*10 {
				t.Fatal("extra attack consumed cost")
			}
		}
	}
}
