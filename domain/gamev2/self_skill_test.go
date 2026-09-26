package game

import (
	"encoding/json"
	"testing"
)

func TestSelfSkillsOncePerTurn(t *testing.T) {
	for _, tt := range []struct {
		id        string
		attack    int
		wriggling bool
	}{
		{"kasuima", 0, false}, {"suima", 1, false}, {"suima", 2, true},
	} {
		t.Run(tt.id+string(rune('0'+tt.attack)), func(t *testing.T) {
			s := NewState("self-skills", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{tt.id}, {"jude"}})
			s.TurnPlayerID = "a"
			s.Characters[0].Wriggling = tt.wriggling
			s.Characters[0].HP = 50
			command := Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, AttackIndex: tt.attack, Target: s.Characters[0].Position}
			// An invalid target must not consume the skill's allowance.
			bad := command
			bad.Target = Position{4, 4}
			if err := s.ApplyAttack("a", bad); err == nil {
				t.Fatal("invalid target accepted")
			}
			if err := s.ApplyAttack("a", command); err != nil {
				t.Fatal(err)
			}
			if tt.id == "kasuima" && !s.hasEffect(0, "威力上昇") {
				t.Fatal("drink did not activate")
			}
			if tt.id == "suima" && tt.attack == 1 && (!s.hasEffect(0, "俊足") || !s.hasEffect(0, "威力上昇")) {
				t.Fatal("star_struck did not activate")
			}
			if tt.attack == 2 && s.Characters[0].HP != 100 {
				t.Fatal("sleep did not heal")
			}
			raw, _ := json.Marshal(s)
			var restored State
			if err := json.Unmarshal(raw, &restored); err != nil {
				t.Fatal(err)
			}
			command.ExpectedRevision = restored.Revision
			before, _ := json.Marshal(&restored)
			if err := restored.ApplyAttack("a", command); err == nil {
				t.Fatal("second use accepted after reload")
			}
			after, _ := json.Marshal(&restored)
			if string(before) != string(after) {
				t.Fatal("rejected use changed state or cost")
			}
			restored.Turn += 2
			restored.Players[0].Cost = MaxCost
			if err := restored.ApplyAttack("a", command); err != nil {
				t.Fatal("not reusable on subsequent turn:", err)
			}
		})
	}
}
