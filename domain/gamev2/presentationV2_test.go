package game

import (
	"encoding/json"
	"testing"
)

func tracedAttack(t *testing.T, s *State, c Command) []PresentationEvent {
	t.Helper()
	c.ExpectedRevision = s.Revision
	c.CharacterID = s.Characters[0].ID
	trace := StartPresentation(s, "a", "ATTACK", &c)
	if err := s.ApplyAttack("a", c); err != nil {
		t.Fatal(err)
	}
	return trace.Finish(s)
}
func presentationFixture(first, second string) *State {
	s := NewState("presentation", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{first}, {second}})
	s.TurnPlayerID = "a"
	s.Characters[0].Position = Position{2, 2}
	s.Characters[1].Position = Position{3, 2}
	return s
}
func TestPresentationFollowUpDefeatAndRevivalAreOrdered(t *testing.T) {
	s := presentationFixture("nadia", "verbulus")
	s.Characters[1].HP = 25
	events := tracedAttack(t, s, Command{Target: Position{3, 2}, Direction: Position{1, 0}})
	if events[0].Type != "SKILL_USED" || events[0].SkillKey != "nadia/normal/0" || events[0].SourceCharacterID != s.Characters[0].ID {
		t.Fatalf("skill: %+v", events[0])
	}
	var resolved []PresentationEvent
	for i, e := range events {
		if e.Index != uint32(i) {
			t.Fatal("unstable event index")
		}
		if e.TargetID == s.Characters[1].ID && (e.Type == "DAMAGED" || e.Type == "DEFEATED" || e.Type == "REVIVED") {
			resolved = append(resolved, e)
		}
	}
	if len(resolved) != 4 {
		t.Fatalf("expected damage/damage/defeat/revive: %+v", resolved)
	}
	if resolved[0].Amount != 20 || resolved[0].Cause != "ATTACK" || resolved[1].Amount != 5 || resolved[1].Cause != "FOLLOW_UP" || resolved[2].Type != "DEFEATED" || resolved[3].Type != "REVIVED" || resolved[3].Amount != 50 || resolved[3].Cause != "REVIVE" {
		t.Fatalf("wrong order or amounts: %+v", resolved)
	}
	if resolved[3].SourceCharacterID != s.Characters[1].ID {
		t.Fatal("revival attributed to attacker")
	}
}
func TestPresentationKnockbackThenMine(t *testing.T) {
	s := presentationFixture("kasuima", "dana")
	s.setTile(Position{5, 2}, "地雷", "a")
	events := tracedAttack(t, s, Command{AttackIndex: 2, Target: Position{3, 2}, Direction: Position{1, 0}})
	damageIndex, moveIndex, mineIndex, removedIndex := -1, -1, -1, -1
	for i, e := range events {
		if e.Type == "DAMAGED" && e.Cause == "ATTACK" {
			damageIndex = i
			if e.Amount != 10 {
				t.Fatal(e)
			}
		}
		if e.Type == "MOVED" {
			moveIndex = i
			if e.Cause != "KNOCKBACK" || e.From == nil || *e.From != (Position{3, 2}) || *e.To != (Position{5, 2}) {
				t.Fatal(e)
			}
		}
		if e.Type == "DAMAGED" && e.Cause == "MINE" {
			mineIndex = i
			if e.Amount != 100 || e.SourceCharacterID != "" || e.SourcePlayerID != "a" {
				t.Fatal(e)
			}
		}
		if e.Type == "TILE_REMOVED" {
			removedIndex = i
		}
	}
	if damageIndex < 0 || moveIndex <= damageIndex || mineIndex <= moveIndex || removedIndex <= mineIndex {
		t.Fatalf("wrong event order: %+v", events)
	}
}
func TestPresentationTurnHealingAndPoisonAreSeparate(t *testing.T) {
	s := presentationFixture("shicho", "dana")
	s.Characters[0].HP = 50
	s.Characters[0].Effects = []string{"毒"}
	trace := StartPresentation(s, "a", "END_TURN", nil)
	if err := s.EndTurn("a", s.Revision); err != nil {
		t.Fatal(err)
	}
	events := trace.Finish(s)
	var hp []PresentationEvent
	for _, e := range events {
		if e.Type == "HEALED" || e.Type == "DAMAGED" {
			hp = append(hp, e)
		}
	}
	if len(hp) != 2 || hp[0].Amount != 10 || hp[0].Cause != "PASSIVE" || hp[0].SourceCharacterID != s.Characters[0].ID || hp[1].Amount != 40 || hp[1].Cause != "POISON" || hp[1].SourceCharacterID != "" || hp[1].BeforeValue != 60 || hp[1].AfterValue != 20 {
		t.Fatalf("healing/poison lost: %+v", hp)
	}
}
func TestPresentationAlternateSkillAndEffectConsumption(t *testing.T) {
	s := presentationFixture("louise", "dana")
	events := tracedAttack(t, s, Command{AttackIndex: 2, Target: s.Characters[0].Position, Direction: Position{1, 0}})
	if events[0].SkillKey != "louise/normal/2" {
		t.Fatal(events[0])
	}
	form := false
	for _, e := range events {
		if e.Type == "FORM_CHANGED" && e.Property == "combat_stance" && e.AfterValue == 1 {
			form = true
		}
	}
	if !form {
		t.Fatal("no stance event")
	}
	events = tracedAttack(t, s, Command{AttackIndex: 2, Target: s.Characters[0].Position, Direction: Position{1, 0}})
	if events[0].SkillKey != "louise/alternate/2" {
		t.Fatal(events[0])
	}
	s = presentationFixture("nadia", "sena")
	s.Characters[1].Effects = []string{"結界"}
	s.Characters[1].BarrierTurn = s.Turn
	events = tracedAttack(t, s, Command{Target: s.Characters[1].Position, Direction: Position{1, 0}})
	blocked, removed := false, false
	for _, e := range events {
		if e.Type == "ATTACK_BLOCKED" {
			blocked = true
		}
		if e.Type == "EFFECT_REMOVED" && e.Effect == "結界" {
			removed = true
		}
		if e.Type == "DAMAGED" {
			t.Fatal("blocked hit emitted damage")
		}
	}
	if !blocked || !removed {
		t.Fatal("missing block/effect events")
	}
}
func TestPresentationDoesNotChangeRulesOrSerializedState(t *testing.T) {
	for _, pair := range [][2]string{{"nadia", "verbulus"}, {"kasuima", "dana"}, {"zina", "sena"}} {
		s := presentationFixture(pair[0], pair[1])
		raw, _ := json.Marshal(s)
		var plain State
		if err := json.Unmarshal(raw, &plain); err != nil {
			t.Fatal(err)
		}
		c := Command{ID: "same", ExpectedRevision: s.Revision, CharacterID: s.Characters[0].ID, AttackIndex: 2, Target: s.Characters[1].Position, Direction: Position{1, 0}}
		trace := StartPresentation(s, "a", "ATTACK", &c)
		err1 := s.ApplyAttack("a", c)
		trace.Finish(s)
		err2 := plain.ApplyAttack("a", c)
		a, _ := json.Marshal(s)
		b, _ := json.Marshal(&plain)
		if (err1 == nil) != (err2 == nil) || string(a) != string(b) {
			t.Fatalf("presentation changed rules for %v", pair)
		}
	}
}
