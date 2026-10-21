package gormv2

import (
	game "auxilia/domain/gamev2"
	"auxilia/domain/model"
	pb "auxilia/pb/v2"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestLobbyConcurrentSeatsAndStaleCommands(t *testing.T) {
	s, ids, _ := testStore(t)
	ctx := context.Background()
	room, err := s.CreateLobby(ctx, ids[0], "concurrent")
	if err != nil {
		t.Fatal(err)
	}
	users := []string{ids[1]}
	for i := 0; i < 6; i++ {
		u := model.User{Name: fmt.Sprintf("guest%d", i), Rate: 1000}
		if err := s.DB.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		users = append(users, u.ID.String())
	}
	var wg sync.WaitGroup
	failures := make(chan error, len(users))
	for _, id := range users {
		wg.Add(1)
		go func(id string) { defer wg.Done(); _, err := s.JoinLobby(ctx, room.RoomId, id); failures <- err }(id)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	current, err := s.GetLobby(ctx, room.RoomId, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	seats := map[int32]int{}
	for _, m := range current.Members {
		seats[m.Seat]++
	}
	if len(current.Members) != 8 || seats[1] != 1 || seats[2] != 1 {
		t.Fatal(current)
	}
	extra := model.User{Name: "extra"}
	if err := s.DB.Create(&extra).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinLobby(ctx, room.RoomId, extra.ID.String()); !errors.Is(err, ErrLobbyFull) {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.ApplyRoomCommand(ctx, ids[0], &pb.RoomCommand{RoomId: room.RoomId, Action: pb.RoomAction_SET_READY, Ready: true, ExpectedRevision: current.Revision})
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	success, stale := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, game.ErrStaleRevision) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || stale != 1 {
		t.Fatal(success, stale)
	}
	ready, err := s.GetLobby(ctx, room.RoomId, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	rejoined, err := s.JoinLobby(ctx, room.RoomId, ids[0])
	if err != nil || rejoined.Revision != ready.Revision {
		t.Fatal(rejoined, err)
	}
}

func TestSpectatorOwnerCanStartAndSeatChangeResetsReady(t *testing.T) {
	s, ids, _ := testStore(t)
	ctx := context.Background()
	room, err := s.CreateLobby(ctx, ids[0], "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinLobby(ctx, room.RoomId, ids[1]); err != nil {
		t.Fatal(err)
	}
	guest := model.User{Name: "guest"}
	if err := s.DB.Create(&guest).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.JoinLobby(ctx, room.RoomId, guest.ID.String()); err != nil {
		t.Fatal(err)
	}
	command := func(id string, action pb.RoomAction, ready bool) *pb.RoomSnapshot {
		t.Helper()
		before, err := s.GetLobby(ctx, room.RoomId, id)
		if err != nil {
			t.Fatal(err)
		}
		after, err := s.ApplyRoomCommand(ctx, id, &pb.RoomCommand{RoomId: room.RoomId, Action: action, Ready: ready, ExpectedRevision: before.Revision})
		if err != nil {
			t.Fatal(err)
		}
		return after
	}
	before, err := s.GetLobby(ctx, room.RoomId, guest.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyRoomCommand(ctx, guest.ID.String(), &pb.RoomCommand{RoomId: room.RoomId, Action: pb.RoomAction_SET_READY, Ready: true, ExpectedRevision: before.Revision}); !errors.Is(err, ErrPrecondition) {
		t.Fatal(err)
	}
	command(ids[1], pb.RoomAction_SET_READY, true)
	changed := command(ids[0], pb.RoomAction_LEAVE_RING, false)
	for _, m := range changed.Members {
		if m.Ready {
			t.Fatal("seat change retained ready")
		}
	}
	command(guest.ID.String(), pb.RoomAction_ENTER_RING, false)
	command(guest.ID.String(), pb.RoomAction_SET_READY, true)
	command(ids[1], pb.RoomAction_SET_READY, true)
	started := command(ids[0], pb.RoomAction_START_MATCH, false)
	if started.Phase != pb.LobbyPhase_SELECTING {
		t.Fatal(started)
	}
	if _, err := s.Read(ctx, started.MatchId, ids[0]); err != nil {
		t.Fatal("spectator owner read", err)
	}
	if _, err := s.Cancel(ctx, started.MatchId, ids[1]); err != nil {
		t.Fatal(err)
	}
	waiting, err := s.GetLobby(ctx, room.RoomId, ids[0])
	if err != nil || waiting.Phase != pb.LobbyPhase_WAITING {
		t.Fatal(waiting, err)
	}
}
