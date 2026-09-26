package game

import (
	"encoding/json"
	"reflect"
	"testing"
)

func verbulusFixture() *State {
	s := NewState("verbulus", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"verbulus", "sophie", "jude"}, {"dana", "chiyo", "zina"}})
	s.TurnPlayerID = "a"
	positions := []Position{{3, 2}, {3, 3}, {1, 0}, {4, 2}, {4, 3}, {7, 4}}
	for i := range s.Characters {
		s.Characters[i].Position = positions[i]
	}
	return s
}

func verbulusAct(s *State, attack int, target Position) error {
	return s.ApplyAttack("a", Command{CharacterID: s.Characters[0].ID, ExpectedRevision: s.Revision, AttackIndex: attack, Target: target, Direction: Position{1, 0}})
}

func TestVerbulusDispelAndHeal(t *testing.T) {
	s := verbulusFixture()
	for i := range s.Characters {
		s.Characters[i].Effects = []string{"威力上昇", "俊足", "俊敏化", "毒"}
	}
	s.Characters[0].Effects = nil
	s.Characters[1].Position = Position{0, 4}
	before := s.Characters[3].HP
	if err := verbulusAct(s, 0, Position{4, 2}); err != nil {
		t.Fatal(err)
	}
	if s.Characters[3].HP != before-30 || !reflect.DeepEqual(s.Characters[3].Effects, []string{"毒"}) {
		t.Fatal("dispel damaged or failed to remove buffs")
	}
	for _, i := range []int{1, 5} {
		if !s.hasEffect(i, "威力上昇") {
			t.Fatalf("non-target %d lost buff", i)
		}
	}
	if s.LastEvent.Type != "ATTACKED" || s.cost("a") != 30 {
		t.Fatal("wrong dispel event/cost")
	}

	s = verbulusFixture()
	for i := range s.Characters {
		s.Characters[i].HP = 50
	}
	if err := verbulusAct(s, 2, Position{3, 2}); err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{100, 100, 50, 50, 50, 50} {
		if s.Characters[i].HP != want {
			t.Errorf("heal target %d hp=%d want=%d", i, s.Characters[i].HP, want)
		}
	}
	if s.cost("a") != 25 || s.LastEvent.Type != "RECOVERED" {
		t.Fatal("wrong heal event/cost")
	}
}

func TestImmutablePlacementAndPersistence(t *testing.T) {
	s := verbulusFixture()
	if err := verbulusAct(s, 1, Position{4, 2}); err != nil {
		t.Fatal(err)
	}
	if len(s.TileEffects) != 1 || s.TileEffects[0].HP != 170 || s.cost("a") != 30 {
		t.Fatal("placement failed")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var restored State
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.TileEffects, restored.TileEffects) {
		t.Fatal("tile HP not persisted")
	}
	s.TurnPlayerID = "b"
	if err := s.ApplyMove("b", Command{CharacterID: s.Characters[3].ID, ExpectedRevision: s.Revision, Target: Position{4, 1}}); err == nil {
		t.Fatal("trapped character escaped")
	}
	s.Characters[3].DefinitionID = "tsukiha"
	if err := s.ApplyMove("b", Command{CharacterID: s.Characters[3].ID, ExpectedRevision: s.Revision, Target: Position{4, 1}}); err != nil {
		t.Fatalf("tile immune character cannot escape: %v", err)
	}
	if err := s.ApplyMove("b", Command{CharacterID: s.Characters[3].ID, ExpectedRevision: s.Revision, Target: Position{4, 2}}); err == nil {
		t.Fatal("entered impassable tile")
	}
}

func TestImmutableCannotBeOverwrittenOrPlacedOnForbiddenCells(t *testing.T) {
	for _, kind := range []string{"base", "blocked", "immutable"} {
		s := verbulusFixture()
		switch kind {
		case "base":
			s.Bases[1].Position = Position{4, 2}
		case "blocked":
			s.BlockedCells = append(s.BlockedCells, Position{4, 2})
		case "immutable":
			s.setTile(Position{4, 2}, "不変", "b")
		}
		if err := verbulusAct(s, 1, Position{4, 2}); err == nil || s.cost("a") != 50 {
			t.Fatalf("invalid placement accepted: %s", kind)
		}
	}
	for _, id := range []string{"dana", "tsukiha", "berenice"} {
		s := verbulusFixture()
		s.Characters[0].DefinitionID = id
		s.Characters[3].Position = Position{6, 2}
		s.setTile(Position{4, 2}, "不変", "b")
		attack := 0
		if id == "tsukiha" {
			attack = 2
		}
		if err := verbulusAct(s, attack, Position{4, 2}); err == nil {
			t.Fatalf("%s overwrote immutable tile", id)
		}
	}
	s := verbulusFixture()
	s.setTile(Position{4, 2}, "毒ガス", "b")
	if err := verbulusAct(s, 1, Position{4, 2}); err == nil || s.TileEffects[0].Type != "毒ガス" {
		t.Fatal("must not replace an existing tile")
	}
}

func TestImmutableDecayAndDestruction(t *testing.T) {
	s := verbulusFixture()
	s.setTile(Position{4, 2}, "不変", "a")
	before := s.Characters[3].HP
	for i, player := range []string{"a", "b", "a", "b"} {
		s.processTurnEnd(player)
		if i < 3 && s.TileEffects[0].HP != 170-50*(i+1) {
			t.Fatal("wrong decay")
		}
	}
	if len(s.TileEffects) != 0 || s.Characters[3].HP != before {
		t.Fatal("decay did not destroy only tile")
	}
	s.TurnPlayerID = "b"
	if err := s.ApplyMove("b", Command{CharacterID: s.Characters[3].ID, ExpectedRevision: s.Revision, Target: Position{4, 1}}); err != nil {
		t.Fatal("still trapped after decay")
	}
}

func TestImmutableCanBeAttackedByEitherSide(t *testing.T) {
	for _, owner := range []string{"a", "b"} {
		s := verbulusFixture()
		s.Characters[0].DefinitionID = "chiyo"
		s.Characters[0].HP = 100                  // no full HP bonus
		s.Characters[1].Position = Position{0, 4} // no Sophie aura
		s.Characters[3].Position = Position{6, 2} // empty tile can be targeted
		s.setTile(Position{4, 2}, "不変", owner)
		if err := verbulusAct(s, 1, Position{4, 2}); err != nil {
			t.Fatal(err)
		}
		if s.TileEffects[0].HP != 110 {
			t.Fatal("wrong attack damage")
		}
		if err := verbulusAct(s, 1, Position{4, 2}); err != nil {
			t.Fatal(err)
		}
		s.Players[0].Cost = MaxCost
		if err := verbulusAct(s, 1, Position{4, 2}); err != nil {
			t.Fatal(err)
		}
		if len(s.TileEffects) != 0 {
			t.Fatal("tile survived lethal damage")
		}
	}
}

func TestClearBuffsPreservesDebuffs(t *testing.T) {
	s := fixture()
	s.Characters[0].Effects = []string{"威力上昇", "毒", "俊足", "麻痺", "俊敏化", "出血", "鈍足", "鈍化"}
	s.clearBuffs(0)
	for _, effect := range []string{"威力上昇", "俊足", "俊敏化"} {
		if s.hasEffect(0, effect) {
			t.Errorf("buff retained: %s", effect)
		}
	}
	for _, effect := range []string{"毒", "麻痺", "出血", "鈍足", "鈍化"} {
		if !s.hasEffect(0, effect) {
			t.Errorf("debuff removed: %s", effect)
		}
	}
}

func TestVerbulusRevivesOnceBeforeDefeatAndPersistsUsage(t *testing.T) {
	s := verbulusFixture()
	s.Characters[0].Effects = []string{"威力上昇", "俊足", "俊敏化", "毒", "麻痺", "出血", "鈍足", "鈍化", "二日酔い"}
	s.Characters[1].HP, s.Characters[2].HP = 0, 0
	s.Characters[3].Position = Position{6, 2}
	s.Characters[4].Position = Position{4, 2}
	s.TurnPlayerID = "b"
	attack := func(state *State) error {
		state.Players[1].Cost = MaxCost
		return state.ApplyAttack("b", Command{CharacterID: state.Characters[4].ID, ExpectedRevision: state.Revision, AttackIndex: 2, Target: Position{3, 2}, Direction: Position{-1, 0}})
	}
	if err := attack(s); err != nil {
		t.Fatal(err)
	}
	if s.Finished || s.Characters[0].HP != 50 || !s.Characters[0].ReviveUsed || s.LastEvent.Type != "REVIVED" {
		t.Fatal("revival must precede defeat")
	}
	if len(s.Characters[0].Effects) != 0 {
		t.Fatal("revival must remove every buff and debuff")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var restored State
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Characters[0].Effects) != 0 {
		t.Fatal("cleared effects returned after reload")
	}
	if err := attack(&restored); err != nil {
		t.Fatal(err)
	}
	if !restored.Finished || restored.WinnerID != "b" || restored.Characters[0].HP != 0 {
		t.Fatal("revival reused after reload")
	}
}

func TestVerbulusRevivesFromPoisonAndMine(t *testing.T) {
	for _, cause := range []string{"poison", "mine"} {
		t.Run(cause, func(t *testing.T) {
			s := verbulusFixture()
			s.Characters[0].HP = 30
			var err error
			if cause == "poison" {
				s.Characters[0].Effects = []string{"毒"}
				err = s.EndTurn("a", s.Revision)
			} else {
				s.setTile(Position{3, 1}, "地雷", "b")
				err = s.ApplyMove("a", Command{CharacterID: s.Characters[0].ID, ExpectedRevision: s.Revision, Target: Position{3, 1}})
			}
			if err != nil {
				t.Fatal(err)
			}
			if s.Characters[0].HP != 50 || !s.Characters[0].ReviveUsed {
				t.Fatal("lethal damage did not trigger revival")
			}
			if len(s.Characters[0].Effects) != 0 {
				t.Fatal("lethal status effect survived revival")
			}
		})
	}
}

func TestVerbulusDispelHitsEverySurroundingEnemy(t *testing.T) {
	for _, offset := range []Position{{-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1}, {2, 0}} {
		s := verbulusFixture()
		s.Characters[1].Position = Position{0, 4}
		s.Characters[3].Position = Position{3 + offset.X, 2 + offset.Y}
		s.Characters[3].Effects = []string{"威力上昇", "毒"}
		before := s.Characters[3].HP
		err := verbulusAct(s, 0, s.Characters[3].Position)
		if offset.X == 2 {
			if err == nil || s.Characters[3].HP != before {
				t.Fatal("out-of-range attack accepted")
			}
		} else if err != nil || s.Characters[3].HP != before-30 || s.hasEffect(3, "威力上昇") || !s.hasEffect(3, "毒") {
			t.Fatalf("wrong damage/dispel at %v", offset)
		}
	}
}

func TestImmutableDoesNotPreventActionsAndReceivesAreaDamage(t *testing.T) {
	s := verbulusFixture()
	s.setTile(s.Characters[0].Position, "不変", "b")
	s.Characters[0].HP = 50
	if err := verbulusAct(s, 2, s.Characters[0].Position); err != nil || s.Characters[0].HP != 100 {
		t.Fatal("trapped character cannot heal")
	}
	s = verbulusFixture()
	s.Characters[0].DefinitionID = "chiyo"
	s.Characters[0].HP = 100
	s.Characters[1].Position = Position{0, 4}
	s.setTile(Position{4, 2}, "不変", "b")
	before := s.Characters[3].HP
	if err := verbulusAct(s, 1, Position{4, 2}); err != nil {
		t.Fatal(err)
	}
	if s.Characters[3].HP != before-60 || s.TileEffects[0].HP != 110 {
		t.Fatal("attack must damage both enemy and tile")
	}
}
