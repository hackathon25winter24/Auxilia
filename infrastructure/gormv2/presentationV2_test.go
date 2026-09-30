package gormv2

import (
	game "auxilia/domain/gamev2"
	"context"
	"encoding/json"
	"testing"
	"time"
)

func batches(t *testing.T, v *View) []game.PresentationBatch {
	t.Helper()
	var out []game.PresentationBatch
	if err := json.Unmarshal([]byte(v.Match.PresentationJSON), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func TestPresentationPersistenceRetryAndRejectedCommands(t *testing.T) {
	s, ids, room := testStore(t)
	v := active(t, s, ids, room)
	ctx := context.Background()
	c := game.Command{ID: "events-end", ExpectedRevision: v.State.Revision}
	player := v.State.TurnPlayerID
	ended, err := s.Apply(ctx, v.Match.ID, player, "END_TURN", c)
	if err != nil {
		t.Fatal(err)
	}
	all := batches(t, ended)
	last := all[len(all)-1]
	if last.Sequence != ended.Match.LogSequence || last.CommandID != c.ID || last.BeforeRevision != c.ExpectedRevision || last.AfterRevision != ended.State.Revision || len(last.Events) == 0 {
		t.Fatal(last)
	}
	retry, err := New(s.DB).Apply(ctx, v.Match.ID, player, "END_TURN", c)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Match.PresentationJSON != ended.Match.PresentationJSON {
		t.Fatal("retry duplicated events")
	}
	c.ID = "bad"
	c.ExpectedRevision = 0
	if _, err := s.Apply(ctx, v.Match.ID, player, "END_TURN", c); err == nil {
		t.Fatal("stale accepted")
	}
	current, err := s.Read(ctx, v.Match.ID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if current.Match.PresentationJSON != ended.Match.PresentationJSON {
		t.Fatal("rejected command emitted events")
	}
	rows, err := s.Logs(ctx, v.Match.ID, ids[0], last.Sequence-1, 1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(last)
	if len(rows) != 1 || rows[0].PresentationJSON != string(raw) {
		t.Fatal("durable history differs from snapshot")
	}
	if err := New(s.DB).Tick(ctx, ended.State.PhaseDeadline.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	current, err = s.Read(ctx, v.Match.ID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	all = batches(t, current)
	last = all[len(all)-1]
	if last.ActionType != "TIMER" {
		t.Fatal("missing timer batch")
	}
	found := false
	for _, e := range last.Events {
		if e.Type == "TURN_CHANGED" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing automatic turn event")
	}
}
func TestPresentationWindowAndOlderHistory(t *testing.T) {
	s, ids, room := testStore(t)
	ctx := context.Background()
	v, err := s.Create(ctx, room, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < PresentationWindow+3; i++ {
		v, err = s.Select(ctx, v.Match.ID, ids[0], []string{"zina", "jude", "dana"})
		if err != nil {
			t.Fatal(err)
		}
	}
	window := batches(t, v)
	if len(window) != PresentationWindow || window[0].Sequence != v.Match.LogSequence-PresentationWindow+1 {
		t.Fatal("bad window")
	}
	for i := 1; i < len(window); i++ {
		if window[i].Sequence != window[i-1].Sequence+1 {
			t.Fatal("non-contiguous window")
		}
	}
	rows, err := s.Logs(ctx, v.Match.ID, ids[0], 0, 1)
	if err != nil || len(rows) != 1 || rows[0].PresentationJSON == "" {
		t.Fatal("window eviction deleted history", err)
	}
	// Simulate pre-event persisted rows: no fabricated historical events.
	if err := s.DB.Model(&Match{}).Where("id = ?", v.Match.ID).Update("presentation_json", "").Error; err != nil {
		t.Fatal(err)
	}
	v, err = s.Select(ctx, v.Match.ID, ids[0], []string{"zina", "jude", "dana"})
	if err != nil {
		t.Fatal(err)
	}
	window = batches(t, v)
	if len(window) != 1 || window[0].Sequence != v.Match.LogSequence {
		t.Fatal("legacy boundary fabricated events")
	}
}
