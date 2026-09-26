package game

import "testing"

func TestZinaDebuffedTargetDamage(t *testing.T) {
	for _, tc := range []struct {
		name    string
		effects []string
		target  string
		want    int
	}{
		{"no effects", nil, "tsukiha", 60},
		{"buff only", []string{"俊足"}, "tsukiha", 60},
		{"poison", []string{"毒"}, "tsukiha", 120},
		{"paralysis", []string{"麻痺"}, "tsukiha", 120},
		{"slow move", []string{"鈍足"}, "tsukiha", 120},
		{"slow attack", []string{"鈍化"}, "tsukiha", 120},
		{"bleed", []string{"出血"}, "tsukiha", 120},
		{"hangover", []string{"二日酔い"}, "tsukiha", 120},
		{"multiple debuffs", []string{"毒", "出血"}, "tsukiha", 120},
		{"damage reduction", []string{"毒"}, "jude", 80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewState("zina-passive", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"zina", "jude", "tsukiha"}, {tc.target, "sena", "aoi"}})
			s.TurnPlayerID = "a"
			s.Players[0].Cost = 50
			positions := []Position{{2, 2}, {1, 2}, {0, 0}, {3, 2}, {7, 4}, {7, 3}}
			for i := range s.Characters {
				s.Characters[i].Position = positions[i]
				s.Characters[i].Effects = nil
			}
			s.Characters[3].MaxHP = 300
			s.Characters[3].HP = 300
			s.Characters[3].Effects = tc.effects
			if s.passiveBoost(1) != 0 {
				t.Fatal("old aura still applied")
			}
			err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, AttackIndex: 2, Direction: Position{1, 0}, Target: Position{3, 2}})
			if err != nil {
				t.Fatal(err)
			}
			if got := 300 - s.Characters[3].HP; got != tc.want {
				t.Fatalf("damage=%d want=%d", got, tc.want)
			}
		})
	}
}
