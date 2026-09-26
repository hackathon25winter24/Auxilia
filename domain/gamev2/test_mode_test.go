package game

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTestModeControlsBothIndependentSides(t *testing.T) {
	s := NewTestState("test", "guest-owner", "テスター", []string{"dana", "jude", "aoi"})
	if s.Players[0].ID != "テスター1" || s.Players[1].ID != "テスター2" || len(s.Characters) != 6 || !s.Started {
		t.Fatal("incorrect test setup")
	}
	for j := 0; j < 3; j++ {
		if s.Characters[j].DefinitionID != s.Characters[j+3].DefinitionID || s.Characters[j].ID == s.Characters[j+3].ID {
			t.Fatal("selection not cloned independently")
		}
	}
	s.Characters[0].HP = 10
	s.addEffect(1, "毒")
	if s.Characters[3].HP != 200 || s.hasEffect(4, "毒") {
		t.Fatal("sides share state")
	}
	if s.ControlledPlayer("other") != "other" || s.EndTest("other", s.Revision) == nil {
		t.Fatal("non-owner gained access")
	}
	s.TurnPlayerID = s.Players[0].ID
	for turn := 0; turn < 2; turn++ {
		actor := turn * 3
		target := Position{0, 3}
		if turn == 1 {
			target = Position{7, 3}
		}
		err := s.ApplyMove(s.ControlledPlayer("guest-owner"), Command{ExpectedRevision: s.Revision, CharacterID: s.Characters[actor].ID, Target: target})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.EndTurn(s.ControlledPlayer("guest-owner"), s.Revision); err != nil {
			t.Fatal(err)
		}
		s.ExpireTurn(s.PhaseDeadline.Add(time.Millisecond))
	}
	raw, _ := json.Marshal(s)
	var restored State
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ControlledPlayer("guest-owner") != restored.TurnPlayerID {
		t.Fatal("control lost after persistence")
	}
	if err := restored.EndTest("guest-owner", restored.Revision); err != nil {
		t.Fatal(err)
	}
	if !restored.Finished || restored.WinnerID != "" {
		t.Fatal("ending test counted as surrender")
	}
}

func TestTestModeTimeoutEndsInsteadOfChangingTurn(t *testing.T) {
	s := NewTestState("test", "owner", "tester", []string{"dana", "jude", "aoi"})
	before := s.TurnPlayerID
	s.ExpireTurn(s.TurnDeadline.Add(time.Millisecond))
	if !s.Finished || s.WinnerID != "" || s.Turn != 1 || s.TurnPlayerID != before {
		t.Fatal("timeout did not end test")
	}
	normal := NewState("normal", s.Players, [2][]string{{"dana"}, {"dana"}})
	normal.ExpireTurn(normal.TurnDeadline.Add(time.Millisecond))
	if normal.Finished || normal.Phase != "turn_end" {
		t.Fatal("normal timeout changed")
	}
}
