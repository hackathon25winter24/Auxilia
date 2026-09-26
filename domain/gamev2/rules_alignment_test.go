package game

import (
	"fmt"
	"testing"
)

func TestSophieStartAndDeparturePassives(t *testing.T) {
	s := NewPendingState("sowing", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"sophie", "dana"}, {"jude", "dana"}})
	if s.hasEffect(0, "俊足") {
		t.Fatal("buff before battle started")
	}
	s.Ready("a")
	s.Ready("b")
	if !s.hasEffect(0, "俊足") || !s.hasEffect(1, "俊足") || s.hasEffect(2, "俊足") {
		t.Fatal("wrong start targets")
	}
	s.Characters[0].HP = 0
	s.checkWinner()
	if !s.hasEffect(2, "鈍足") || s.hasEffect(3, "鈍足") {
		t.Fatal("wrong departure targets / Dana immunity")
	}
	s.clearDebuffs(2)
	s.checkWinner()
	if s.hasEffect(2, "鈍足") {
		t.Fatal("departure triggered twice")
	}
}

func TestAoiShiokumiDamagesEnemiesAndBuffsAllies(t *testing.T) {
	s := NewState("shiokumi", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"aoi", "dana"}, {"dana"}})
	s.TurnPlayerID = "a"
	s.Characters[0].Position = Position{3, 2}
	s.Characters[1].Position = Position{3, 1}
	s.Characters[2].Position = Position{4, 2}
	err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, Target: Position{3, 1}, Direction: Position{1, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Characters[1].HP != 200 || !s.hasEffect(1, "俊敏化") || s.hasEffect(0, "俊敏化") || s.Characters[2].HP != 150 || s.cost("a") != 30 {
		t.Fatalf("wrong combined attack: %+v", s.Characters)
	}
}

func TestNadiaExtraAttackRateAndCost(t *testing.T) {
	hits := 0
	for n := 0; n < 2000; n++ {
		s := NewState(fmt.Sprintf("overdose-%d", n), [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"nadia"}, {"dana"}})
		s.TurnPlayerID = "a"
		s.Characters[0].Position = Position{3, 2}
		s.Characters[1].Position = Position{4, 2}
		err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, Target: Position{4, 2}, Direction: Position{1, 0}})
		if err != nil {
			t.Fatal(err)
		}
		switch s.Characters[1].HP {
		case 170:
			hits++
		default:
			t.Fatal("unexpected damage / chained extra attack")
		}
		if s.cost("a") != 40 {
			t.Fatal("extra attack consumed cost")
		}
	}
	if hits != 2000 {
		t.Fatalf("extra attack rate %d/2000, expected 100%%", hits)
	}
}

func TestBereniceConsumesOnlyMinesInAttackRange(t *testing.T) {
	s := NewState("mines", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"berenice"}, {"dana"}})
	s.TurnPlayerID = "a"
	s.Characters[0].Position = Position{3, 2}
	s.Characters[1].Position = Position{5, 2}
	s.setTile(Position{4, 2}, "地雷", "a")
	s.setTile(Position{5, 1}, "地雷", "b")
	s.setTile(Position{0, 0}, "地雷", "b")
	s.setTile(Position{5, 3}, "毒ガス", "b")
	err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, AttackIndex: 1, Target: Position{5, 2}, Direction: Position{1, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Characters[1].HP != 120 || len(s.TileEffects) != 2 || s.tileAt(Position{0, 0}) < 0 || s.tileAt(Position{5, 3}) < 0 {
		t.Fatalf("mine removal / bonus wrong: %+v", s)
	}
}

func TestShichoRecoveryAndCleanseIncludeEnemies(t *testing.T) {
	for _, attack := range []int{1, 2} {
		s := NewState("shicho", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"shicho"}, {"jude"}})
		s.TurnPlayerID = "a"
		s.Characters[0].Position = Position{3, 2}
		s.Characters[1].Position = Position{4, 2}
		for j := range s.Characters {
			s.Characters[j].HP = 30
			s.Characters[j].Effects = []string{"毒"}
		}
		err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, AttackIndex: attack, Target: Position{4, 2}, Direction: Position{1, 0}})
		if err != nil {
			t.Fatal(err)
		}
		for j := range s.Characters {
			if attack == 1 && s.Characters[j].HP != 70 {
				t.Fatal("heal missed target")
			}
			if attack == 2 && s.hasEffect(j, "毒") {
				t.Fatal("cleanse missed target")
			}
		}
	}
}
