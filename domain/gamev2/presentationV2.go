package game

import "fmt"

// Presentation events describe resolved changes; they never participate in rules,
// revisions or random rolls. Identity is (match ID, batch sequence, event index).
type PresentationEvent struct {
	Index             uint32    `json:"index"`
	Type              string    `json:"type"`
	Cause             string    `json:"cause"`
	SourceCharacterID string    `json:"sourceCharacterId"`
	SourcePlayerID    string    `json:"sourcePlayerId"`
	TargetKind        string    `json:"targetKind"`
	TargetID          string    `json:"targetId"`
	DefinitionID      string    `json:"definitionId"`
	SkillKey          string    `json:"skillKey"`
	SkillName         string    `json:"skillName"`
	AttackIndex       int       `json:"attackIndex"`
	Amount            int       `json:"amount"`
	BeforeValue       int       `json:"beforeValue"`
	AfterValue        int       `json:"afterValue"`
	Effect            string    `json:"effect"`
	Property          string    `json:"property"`
	BeforeText        string    `json:"beforeText"`
	AfterText         string    `json:"afterText"`
	From              *Position `json:"from,omitempty"`
	To                *Position `json:"to,omitempty"`
	Direction         *Position `json:"direction,omitempty"`
}
type PresentationBatch struct {
	Version        uint32              `json:"version"`
	Sequence       uint64              `json:"sequence"`
	BeforeRevision uint64              `json:"beforeRevision"`
	AfterRevision  uint64              `json:"afterRevision"`
	CommandID      string              `json:"commandId"`
	ActionType     string              `json:"actionType"`
	Events         []PresentationEvent `json:"events"`
}
type PresentationTrace struct {
	state    *State
	previous State
	context  PresentationEvent
	events   []PresentationEvent
}

func StartPresentation(s *State, player, action string, command *Command) *PresentationTrace {
	t := &PresentationTrace{state: s, previous: presentationCopy(s), context: PresentationEvent{Cause: action, SourcePlayerID: player}}
	s.presentation = t
	if command != nil && (action == "ATTACK" || action == "MOVE") {
		t.context.SourceCharacterID = command.CharacterID
		if action == "ATTACK" {
			if i, d, err := s.actor(player, command.CharacterID); err == nil && command.AttackIndex >= 0 && command.AttackIndex < len(d.Attacks) {
				stance := "normal"
				if (s.Characters[i].Wriggling || s.Characters[i].CombatStance) && d.AlternateAttacks != nil {
					stance = "alternate"
				}
				t.context.SkillKey = fmt.Sprintf("%s/%s/%d", d.ID, stance, command.AttackIndex)
				t.context.SkillName = d.Attacks[command.AttackIndex].Name
				t.context.AttackIndex = command.AttackIndex
				t.emit(PresentationEvent{Type: "SKILL_USED", TargetKind: "CELL", To: positionPtr(command.Target), Direction: positionPtr(command.Direction)})
			}
		}
	}
	return t
}
func (t *PresentationTrace) Finish(s *State) []PresentationEvent {
	t.capture(s)
	t.state.presentation = nil
	s.presentation = nil
	for i := range t.events {
		t.events[i].Index = uint32(i)
	}
	return t.events
}
func positionPtr(p Position) *Position { return &p }
func presentationCopy(s *State) State {
	copy := *s
	copy.presentation = nil
	copy.Characters = append([]Character(nil), s.Characters...)
	for i := range copy.Characters {
		copy.Characters[i].Effects = append([]string(nil), s.Characters[i].Effects...)
	}
	copy.TileEffects = append([]TileEffect(nil), s.TileEffects...)
	return copy
}
func (t *PresentationTrace) emit(e PresentationEvent) {
	e.Cause = t.context.Cause
	e.SourceCharacterID = t.context.SourceCharacterID
	e.SourcePlayerID = t.context.SourcePlayerID
	e.SkillKey = t.context.SkillKey
	e.SkillName = t.context.SkillName
	e.AttackIndex = t.context.AttackIndex
	t.events = append(t.events, e)
}
func (s *State) observePresentation() {
	if s.presentation != nil {
		s.presentation.capture(s)
	}
}

// A scope flushes previous changes before changing attribution. source=-1 means
// an environmental/system source, not the player who happened to end the turn.
func (s *State) presentationScope(cause string, source int) func() {
	if s.presentation == nil {
		return func() {}
	}
	t := s.presentation
	t.capture(s)
	previous := t.context
	t.context.Cause = cause
	t.context.SourceCharacterID = ""
	t.context.SourcePlayerID = ""
	if source >= 0 && source < len(s.Characters) {
		t.context.SourceCharacterID = s.Characters[source].ID
		t.context.SourcePlayerID = s.Characters[source].OwnerID
	}
	if cause != "FOLLOW_UP" && cause != "KNOCKBACK" {
		t.context.SkillKey = ""
		t.context.SkillName = ""
		t.context.AttackIndex = 0
	}
	return func() { t.capture(s); t.context = previous }
}
func (s *State) presentationSignal(kind, targetKind, targetID, effect string) {
	if s.presentation != nil {
		s.observePresentation()
		s.presentation.emit(PresentationEvent{Type: kind, TargetKind: targetKind, TargetID: targetID, Effect: effect})
	}
}
func hasString(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func (t *PresentationTrace) capture(s *State) {
	old := &t.previous
	for _, c := range s.Characters {
		var before *Character
		for i := range old.Characters {
			if old.Characters[i].ID == c.ID {
				before = &old.Characters[i]
				break
			}
		}
		if before == nil || before.DefinitionID != c.DefinitionID {
			t.emit(PresentationEvent{Type: "CHARACTER_SPAWNED", TargetKind: "CHARACTER", TargetID: c.ID, DefinitionID: c.DefinitionID, To: positionPtr(c.Position), AfterValue: c.HP})
			continue
		}
		e := PresentationEvent{TargetKind: "CHARACTER", TargetID: c.ID}
		if before.Position != c.Position {
			move := e
			move.Type = "MOVED"
			move.From = positionPtr(before.Position)
			move.To = positionPtr(c.Position)
			t.emit(move)
		}
		if before.HP != c.HP {
			hp := e
			hp.Type = "DAMAGED"
			hp.Amount = before.HP - c.HP
			hp.BeforeValue = before.HP
			hp.AfterValue = c.HP
			if c.HP > before.HP {
				hp.Type = "HEALED"
				hp.Amount = c.HP - before.HP
				if before.HP <= 0 {
					hp.Type = "REVIVED"
				}
			}
			t.emit(hp)
			if before.HP > 0 && c.HP <= 0 {
				dead := e
				dead.Type = "DEFEATED"
				t.emit(dead)
			}
		}
		for _, effect := range before.Effects {
			if !hasString(c.Effects, effect) {
				event := e
				event.Type = "EFFECT_REMOVED"
				event.Effect = effect
				t.emit(event)
			}
		}
		for _, effect := range c.Effects {
			if !hasString(before.Effects, effect) {
				event := e
				event.Type = "EFFECT_ADDED"
				event.Effect = effect
				t.emit(event)
			}
		}
		for _, change := range []struct {
			name          string
			before, after bool
		}{{"combat_stance", before.CombatStance, c.CombatStance}, {"wriggling", before.Wriggling, c.Wriggling}} {
			if change.before != change.after {
				event := e
				event.Type = "FORM_CHANGED"
				event.Property = change.name
				if change.before {
					event.BeforeValue = 1
				}
				if change.after {
					event.AfterValue = 1
				}
				t.emit(event)
			}
		}
	}
	for _, before := range old.Characters {
		found := false
		for _, c := range s.Characters {
			if c.ID == before.ID {
				found = true
				break
			}
		}
		if !found {
			t.emit(PresentationEvent{Type: "CHARACTER_REMOVED", TargetKind: "CHARACTER", TargetID: before.ID})
		}
	}
	for i, b := range s.Bases {
		if old.Bases[i].HP != b.HP {
			kind := "DAMAGED"
			amount := old.Bases[i].HP - b.HP
			if amount < 0 {
				kind = "HEALED"
				amount = -amount
			}
			t.emit(PresentationEvent{Type: kind, TargetKind: "BASE", TargetID: b.OwnerID, Amount: amount, BeforeValue: old.Bases[i].HP, AfterValue: b.HP, To: positionPtr(b.Position)})
		}
	}
	for _, before := range old.TileEffects {
		index := -1
		for i, tile := range s.TileEffects {
			if tile.Position == before.Position && tile.Type == before.Type && tile.OwnerID == before.OwnerID {
				index = i
				break
			}
		}
		e := PresentationEvent{TargetKind: "TILE", TargetID: before.OwnerID, Effect: before.Type, To: positionPtr(before.Position), BeforeValue: before.HP}
		if index < 0 {
			e.Type = "TILE_REMOVED"
			t.emit(e)
		} else if s.TileEffects[index].HP != before.HP {
			e.Type = "TILE_HP_CHANGED"
			e.AfterValue = s.TileEffects[index].HP
			e.Amount = before.HP - e.AfterValue
			t.emit(e)
		}
	}
	for _, tile := range s.TileEffects {
		found := false
		for _, before := range old.TileEffects {
			if tile.Position == before.Position && tile.Type == before.Type && tile.OwnerID == before.OwnerID {
				found = true
				break
			}
		}
		if !found {
			t.emit(PresentationEvent{Type: "TILE_ADDED", TargetKind: "TILE", TargetID: tile.OwnerID, Effect: tile.Type, To: positionPtr(tile.Position), AfterValue: tile.HP})
		}
	}
	for i, p := range s.Players {
		if old.Players[i].Cost != p.Cost {
			t.emit(PresentationEvent{Type: "COST_CHANGED", TargetKind: "PLAYER", TargetID: p.ID, BeforeValue: old.Players[i].Cost, AfterValue: p.Cost})
		}
	}
	if !old.Started && s.Started {
		t.emit(PresentationEvent{Type: "MATCH_STARTED", TargetKind: "MATCH"})
	}
	if old.Turn != s.Turn || old.TurnPlayerID != s.TurnPlayerID {
		t.emit(PresentationEvent{Type: "TURN_CHANGED", TargetKind: "PLAYER", TargetID: s.TurnPlayerID, BeforeValue: old.Turn, AfterValue: s.Turn})
	}
	if old.Phase != s.Phase {
		t.emit(PresentationEvent{Type: "PHASE_CHANGED", TargetKind: "MATCH", BeforeText: old.Phase, AfterText: s.Phase})
	}
	if !old.Finished && s.Finished {
		t.emit(PresentationEvent{Type: "MATCH_FINISHED", TargetKind: "MATCH", AfterText: s.WinnerID})
	}
	t.previous = presentationCopy(s)
}
