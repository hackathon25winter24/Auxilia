package game

import (
	"reflect"
	"testing"
)

func TestAoiOnnadateBuffsOnlyOtherAllies(t *testing.T) {
	s := NewState("aoi-pending", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"aoi", "jude", "sophie"}, {"dana"}})
	s.TurnPlayerID = "a"
	s.Characters[0].Position = Position{3, 2}
	s.Characters[1].Position = Position{4, 3}
	s.Characters[2].Position = Position{2, 1}
	s.Characters[3].Position = Position{4, 2}
	for i := range s.Characters {
		s.Characters[i].HP = 50
		s.Characters[i].Effects = []string{"出血"}
	}
	before := append([]Character(nil), s.Characters...)
	before[1].Effects = []string{"出血", "威力上昇"}
	before[2].Effects = []string{"出血", "威力上昇"}
	bases := s.Bases
	err := s.ApplyAttack("a", Command{CharacterID: s.Characters[0].ID, ExpectedRevision: s.Revision, AttackIndex: 1, Target: Position{4, 3}, Direction: Position{1, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, s.Characters) || !reflect.DeepEqual(bases, s.Bases) {
		t.Fatal("buff must affect only other allies, without damage or healing")
	}
	if s.cost("a") != 20 || s.LastEvent.Type != "SKILL_USED" {
		t.Fatalf("cost=%d event=%s", s.cost("a"), s.LastEvent.Type)
	}
}

func TestAoiRecoveryTargetsSurroundingAllies(t *testing.T) {
	for _, direction := range []Position{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
		for _, offset := range []Position{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}, {2, 0}} {
			s := NewState("aoi-recovery", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"aoi", "dana", "jude"}, {"sophie"}})
			s.TurnPlayerID = "a"
			s.Characters[0].Position = Position{3, 2}
			s.Characters[1].Position = Position{3 + offset.X, 2 + offset.Y}
			s.Characters[2].Position = Position{0, 0}
			s.Characters[3].Position = Position{3, 3}
			for i := range s.Characters {
				s.Characters[i].HP = 50
			}
			err := s.ApplyAttack("a", Command{CharacterID: s.Characters[0].ID, ExpectedRevision: s.Revision, AttackIndex: 2, Target: Position{3, 2}, Direction: direction})
			if err != nil {
				t.Fatal(err)
			}
			want := 80
			if offset.X == 2 {
				want = 50
			}
			if s.Characters[0].HP != 80 || s.Characters[1].HP != want || s.Characters[2].HP != 50 || s.Characters[3].HP != 50 {
				t.Fatalf("direction=%v offset=%v characters=%v", direction, offset, s.Characters)
			}
			if s.cost("a") != 30 || s.LastEvent.Type != "RECOVERED" {
				t.Fatal("wrong recovery cost/event")
			}
		}
	}
}

func TestSenaAndBereniceAttackFootprints(t *testing.T) {
	tests := []struct {
		name, id       string
		attack, damage int
		cells          []Position
	}{
		{"sena thrust", "sena", 0, 40, []Position{{4, 2}, {5, 2}}},
		{"sena sweep", "sena", 1, 60, []Position{{5, 1}, {5, 2}, {5, 3}}},
		{"sena slash", "sena", 2, 90, []Position{{5, 2}, {6, 2}}},
		{"berenice explosion", "berenice", 1, 60, []Position{{4, 2}, {5, 1}, {5, 2}, {5, 3}, {6, 2}}},
		{"berenice small bomb", "berenice", 2, 50, []Position{{4, 1}, {4, 2}, {4, 3}, {5, 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for y := 0; y < Height; y++ {
				for x := 0; x < Width; x++ {
					target := Position{x, y}
					if target == (Position{3, 2}) {
						continue
					}
					s := NewState("footprint", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{tt.id}, {"dana"}})
					s.TurnPlayerID = "a"
					s.Characters[0].Position = Position{3, 2}
					s.Characters[1].Position = target
					// Isolate character targets from bases for this attack footprint test.
					s.Bases[0].Position = Position{-1, -1}
					s.Bases[1].Position = Position{-2, -2}
					before := s.Characters[1].HP
					err := s.ApplyAttack("a", Command{CharacterID: s.Characters[0].ID, ExpectedRevision: s.Revision, AttackIndex: tt.attack, Target: target, Direction: Position{1, 0}})
					if containsPosition(tt.cells, target) {
						if err != nil || s.Characters[1].HP != before-tt.damage {
							t.Errorf("at %v: err=%v hp=%d", target, err, s.Characters[1].HP)
						}
					} else if err == nil || s.Characters[1].HP != before {
						t.Errorf("outside pattern %v accepted/damaged", target)
					}
				}
			}
		})
	}
}

func TestTsukihaBleedingUsesSpecifiedThresholds(t *testing.T) {
	for attack, chance := range []int{30, 20} {
		hits := 0
		for revision := uint64(1); revision <= 1000; revision++ {
			s := NewState("tsukiha-bleed", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"tsukiha"}, {"sophie"}})
			s.TurnPlayerID = "a"
			s.Revision = revision
			s.Characters[0].Position = Position{2, 2}
			s.Characters[1].Position = Position{4 + attack, 2}
			want := s.roll(0, 1, "出血", chance)
			err := s.ApplyAttack("a", Command{CharacterID: s.Characters[0].ID, ExpectedRevision: s.Revision, AttackIndex: attack, Target: s.Characters[1].Position, Direction: Position{1, 0}})
			if err != nil {
				t.Fatal(err)
			}
			if s.hasEffect(1, "出血") != want {
				t.Fatalf("attack %d revision %d: wrong bleeding threshold", attack+1, revision)
			}
			if want {
				hits++
			}
		}
		if hits == 0 || hits == 1000 {
			t.Fatalf("attack %d never varied", attack+1)
		}
	}
}
