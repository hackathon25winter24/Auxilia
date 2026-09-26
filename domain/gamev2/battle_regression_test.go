package game

import (
	"fmt"
	"testing"
	"time"
)

func TestNormalMovePreservesHP(t *testing.T) {
	s := fixture()
	before := s.Characters[0].HP
	if err := s.ApplyMove("a", Command{CharacterID: s.Characters[0].ID, ExpectedRevision: s.Revision, Target: Position{1, 4}}); err != nil {
		t.Fatal(err)
	}
	if s.Characters[0].HP != before || s.LastEvent.Type != "MOVED" {
		t.Fatalf("normal move: hp=%d event=%s", s.Characters[0].HP, s.LastEvent.Type)
	}
}

func TestAoiHealingExcludesSelfAndEnemy(t *testing.T) {
	s := NewState("healing", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"aoi", "jude", "dana"}, {"sophie", "chiyo", "zina"}})
	for i := range s.Characters {
		s.Characters[i].HP = 50
	}
	s.Characters[0].Position = Position{3, 2}
	s.Characters[1].Position = Position{4, 3}
	s.Characters[2].Position = Position{6, 4}
	s.Characters[3].Position = Position{3, 3}
	s.processTurnEnd("a")
	for i, want := range []int{50, 80, 50, 50} {
		if s.Characters[i].HP != want {
			t.Errorf("character %d hp=%d want=%d", i, s.Characters[i].HP, want)
		}
	}
}

func TestTurnDurationIs120Seconds(t *testing.T) {
	s := fixture()
	if TurnDuration != 120*time.Second {
		t.Fatal(TurnDuration)
	}
	s.ExpireTurn(s.TurnDeadline.Add(-time.Second))
	if s.Phase != "action" {
		t.Fatal("turn expired early")
	}
	s.ExpireTurn(s.TurnDeadline.Add(time.Millisecond))
	if s.Phase != "turn_end" {
		t.Fatal("turn did not expire")
	}
	s.ExpireTurn(s.PhaseDeadline)
	if remaining := time.Until(s.TurnDeadline); remaining < 119*time.Second || remaining > 120*time.Second {
		t.Fatal(remaining)
	}
}

func TestDanaPoisonObservedRates(t *testing.T) {
	const trials = 10000
	for _, mode := range []string{"attack", "tile"} {
		t.Run(mode, func(t *testing.T) {
			hits := 0
			for n := 0; n < trials; n++ {
				s := NewState(fmt.Sprintf("poison-%d", n), [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"dana", "jude", "aoi"}, {"sophie", "chiyo", "zina"}})
				s.TurnPlayerID = "a"
				s.Revision = uint64(n + 1)
				s.Characters[0].Position = Position{2, 2}
				s.Characters[3].Position = Position{3, 2}
				if mode == "attack" {
					if err := s.ApplyAttack("a", Command{CharacterID: s.Characters[0].ID, ExpectedRevision: s.Revision, AttackIndex: 1, Target: Position{3, 2}, Direction: Position{1, 0}}); err != nil {
						t.Fatal(err)
					}
				} else {
					s.setTile(Position{3, 2}, "毒ガス", "a")
					s.triggerTile(3)
				}
				if s.hasEffect(3, "毒") {
					hits++
				}
			}
			t.Logf("%s: %d/%d (%.2f%%)", mode, hits, trials, float64(hits)*100/trials)
			wantPercent := 50 // 毒ガスマスは50%、直接攻撃は改定後100%。
			if mode == "attack" {
				wantPercent = 100
			}
			if hits < (wantPercent-3)*trials/100 || hits > (wantPercent+3)*trials/100 {
				t.Fatalf("observed rate differs from %d%%: %d/%d", wantPercent, hits, trials)
			}
		})
	}
}

func TestPoisonGasEndTurnAndImmunities(t *testing.T) {
	for _, id := range []string{"sophie", "dana", "tsukiha"} {
		s := NewState("gas", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{id, "jude", "aoi"}, {"sophie", "chiyo", "zina"}})
		s.setTile(s.Characters[0].Position, "毒ガス", "b")
		s.processTurnEnd("a")
		if s.hasEffect(0, "毒") != (id == "sophie") {
			t.Errorf("gas immunity/end-turn: %s", id)
		}
	}
}
