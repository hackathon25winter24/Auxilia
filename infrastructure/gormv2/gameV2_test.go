package gormv2

import (
	game "auxilia/domain/gamev2"
	"auxilia/domain/model"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"testing"
	"time"
)

func testStore(t *testing.T) (*Store, [2]string, uint32) {
	t.Helper()
	db := mysqlTestDB(t)
	var err error
	if db == nil {
		db, err = gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	}
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if db.Dialector.Name() == "sqlite" {
		sqlDB.SetMaxOpenConns(1)
	} else {
		sqlDB.SetMaxOpenConns(8)
	}
	t.Cleanup(func() { sqlDB.Close() })
	if err := db.AutoMigrate(&model.User{}, &model.Room{}, &model.RoomMatch{}); err != nil {
		t.Fatal(err)
	}
	s := New(db)
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	ids := [2]string{uuid.NewString(), uuid.NewString()}
	room := model.RoomMatch{OwnerID: ids[0], RoomName: "test"}
	if err := db.Create(&room).Error; err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		user := model.User{ID: uuid.MustParse(id), Name: fmt.Sprintf("p%d", i), Rate: 1500}
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.Room{RoomID: int32(room.ID), UserID: id, State: int32(i + 1), IsReady: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return s, ids, uint32(room.ID)
}
func active(t *testing.T, s *Store, ids [2]string, room uint32) *View {
	t.Helper()
	ctx := context.Background()
	v, err := s.Create(ctx, room, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		v, err = s.Select(ctx, v.Match.ID, id, []string{"zina", "jude", "dana"})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range ids {
		v, err = s.Ready(ctx, v.Match.ID, id)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !v.State.Started {
		t.Fatal("not started")
	}
	return v
}
func TestAtomicCreationSelectionAndParticipantChecks(t *testing.T) {
	s, ids, room := testStore(t)
	ctx := context.Background()
	if _, err := s.Create(ctx, room, "intruder"); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	v, err := s.Create(ctx, room, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.Create(ctx, room, ids[1])
	if err != nil || duplicate.Match.ID != v.Match.ID {
		t.Fatal("duplicate creation", err)
	}
	if _, err := s.Ready(ctx, v.Match.ID, ids[0]); !errors.Is(err, ErrPrecondition) {
		t.Fatal("ready without selections", err)
	}
	for _, bad := range [][]string{{"zina"}, {"zina", "zina", "dana"}, {"unknown", "jude", "dana"}} {
		if _, err := s.Select(ctx, v.Match.ID, ids[0], bad); !errors.Is(err, game.ErrInvalidAction) {
			t.Fatal(err)
		}
	}
	if _, err := s.Read(ctx, v.Match.ID, "intruder"); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.Select(ctx, v.Match.ID, "intruder", []string{"zina", "jude", "dana"}); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.Cancel(ctx, v.Match.ID, ids[0]); err != nil {
		t.Fatal(err)
	}
	var m model.RoomMatch
	if err := s.DB.First(&m, room).Error; err != nil {
		t.Fatal(err)
	}
	if m.IsGaming {
		t.Fatal("cancel left room locked")
	}
	var user model.User
	s.DB.First(&user, "id = ?", ids[0])
	if user.NumBattles != 0 {
		t.Fatal("cancel counted as battle")
	}
}
func TestCommandDeduplicationStaleRevisionAndDurableLogs(t *testing.T) {
	s, ids, room := testStore(t)
	v := active(t, s, ids, room)
	ctx := context.Background()
	player := v.State.TurnPlayerID
	c := game.Command{ID: "end-1", ExpectedRevision: v.State.Revision}
	first, err := s.Apply(ctx, v.Match.ID, player, "END_TURN", c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Apply(ctx, v.Match.ID, player, "END_TURN", c)
	if err != nil {
		t.Fatal(err)
	}
	if first.Match.LogSequence != second.Match.LogSequence {
		t.Fatal("retry applied twice")
	}
	c.ExpectedRevision++
	if _, err := s.Apply(ctx, v.Match.ID, player, "END_TURN", c); !errors.Is(err, ErrCommandConflict) {
		t.Fatal("conflicting retry", err)
	}
	c.ID = "stale"
	c.ExpectedRevision = 1
	if _, err := s.Apply(ctx, v.Match.ID, player, "END_TURN", c); !errors.Is(err, game.ErrStaleRevision) {
		t.Fatal("stale accepted", err)
	}
	rows, err := s.Logs(ctx, v.Match.ID, player, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != int(first.Match.LogSequence) {
		t.Fatal("missing history")
	}
	for i, row := range rows {
		if row.Sequence != uint64(i+1) {
			t.Fatal("non-contiguous transition sequence")
		}
	}
	if rows[len(rows)-1].BeforeJSON == "" || rows[len(rows)-1].CommandJSON == "" {
		t.Fatal("missing structured transition data")
	}
}
func TestExpiryPersistsOnRejectedCommandAndSurvivesRestart(t *testing.T) {
	s, ids, room := testStore(t)
	v := active(t, s, ids, room)
	ctx := context.Background()
	v.State.TurnDeadline = time.Now().Add(-time.Second)
	raw, _ := json.Marshal(v.State)
	if err := s.DB.Model(&Match{}).Where("id = ?", v.Match.ID).Update("state_json", string(raw)).Error; err != nil {
		t.Fatal(err)
	}
	_, err := s.Apply(ctx, v.Match.ID, v.State.TurnPlayerID, "END_TURN", game.Command{ID: "late", ExpectedRevision: v.State.Revision})
	if !errors.Is(err, game.ErrStaleRevision) {
		t.Fatal(err)
	}
	restored := New(s.DB)
	v, err = restored.Read(ctx, v.Match.ID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if v.State.Phase != "turn_end" {
		t.Fatal("expiry rolled back with stale command")
	}
	oldPlayer := v.State.TurnPlayerID
	if err := restored.Tick(ctx, v.State.PhaseDeadline.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	v, err = restored.Read(ctx, v.Match.ID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if v.State.Phase != "action" || v.State.TurnPlayerID == oldPlayer || v.State.Turn != 2 {
		t.Fatal("scheduler did not advance persisted state")
	}
}
func TestSurrenderSettlesExactlyOnceAndRematchIsSeparate(t *testing.T) {
	s, ids, room := testStore(t)
	v := active(t, s, ids, room)
	ctx := context.Background()
	c := game.Command{ID: "surrender", ExpectedRevision: v.State.Revision}
	finished, err := s.Apply(ctx, v.Match.ID, ids[0], "SURRENDER", c)
	if err != nil {
		t.Fatal(err)
	}
	if !finished.State.Finished || finished.State.WinnerID != ids[1] {
		t.Fatal("wrong result")
	}
	if finished.Match.P1Rate != 1484 || finished.Match.P2Rate != 1516 {
		t.Fatal("wrong rating")
	}
	if _, err := New(s.DB).Apply(ctx, v.Match.ID, ids[0], "SURRENDER", c); err != nil {
		t.Fatal(err)
	}
	if err := s.Tick(ctx, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		var user model.User
		if err := s.DB.First(&user, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if user.NumBattles != 1 || user.NumWins != i {
			t.Fatal("duplicate settlement")
		}
	}
	if err := s.DB.Model(&model.Room{}).Where("room_id = ?", room).Update("is_ready", true).Error; err != nil {
		t.Fatal(err)
	}
	next, err := s.Create(ctx, room, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if next.Match.ID == v.Match.ID {
		t.Fatal("rematch reused old ID")
	}
	old, err := s.Read(ctx, v.Match.ID, ids[0])
	if err != nil || !old.State.Finished {
		t.Fatal("old result destroyed", err)
	}
}
func TestFailedSettlementRollsBackWholeAction(t *testing.T) {
	s, ids, room := testStore(t)
	v := active(t, s, ids, room)
	ctx := context.Background()
	if err := s.DB.Where("id = ?", ids[1]).Delete(&model.User{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, v.Match.ID, ids[0], "SURRENDER", game.Command{ID: "fail", ExpectedRevision: v.State.Revision}); err == nil {
		t.Fatal("missing user accepted")
	}
	restored, err := s.Read(ctx, v.Match.ID, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if restored.State.Finished || restored.Match.LogSequence != v.Match.LogSequence {
		t.Fatal("partial commit")
	}
}

func TestFinishedCommandReplayAfterRoomRemoval(t *testing.T) {
	s, ids, room := testStore(t)
	v := active(t, s, ids, room)
	ctx := context.Background()
	command := game.Command{ID: "final", ExpectedRevision: v.State.Revision}
	result, err := s.Apply(ctx, v.Match.ID, ids[0], "SURRENDER", command)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Where("id = ?", room).Delete(&model.RoomMatch{}).Error; err != nil {
		t.Fatal(err)
	}
	replay, err := s.Apply(ctx, v.Match.ID, ids[0], "SURRENDER", command)
	if err != nil {
		t.Fatal(err)
	}
	if replay.Match.LogSequence != result.Match.LogSequence || !replay.State.Finished {
		t.Fatal("finished retry changed state")
	}
}
