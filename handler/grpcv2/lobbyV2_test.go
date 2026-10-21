package grpcv2

import (
	pb "auxilia/pb/v2"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

func TestLobbyRPCSnapshotWatchAndBattleHandoff(t *testing.T) {
	f := transport(t)
	client := pb.NewLobbyServiceV2Client(f.conn)
	if _, err := client.CreateLobby(context.Background(), &pb.CreateLobbyRequest{RoomName: "test"}); status.Code(err) != codes.Unauthenticated {
		t.Fatal(err)
	}
	room, err := client.CreateLobby(f.contexts[0], &pb.CreateLobbyRequest{RoomName: " lobby "})
	if err != nil {
		t.Fatal(err)
	}
	req := &pb.LobbyRequest{RoomId: room.RoomId}
	if room.RoomName != "lobby" || room.Members[0].Seat != 1 || room.CanStart || room.Revision == 0 {
		t.Fatal(room)
	}
	if _, err := client.GetLobby(f.contexts[1], req); status.Code(err) != codes.PermissionDenied {
		t.Fatal("nonmember read", err)
	}
	watchCtx, cancel := context.WithCancel(f.contexts[0])
	defer cancel()
	watch, err := client.WatchRoom(watchCtx, req)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := watch.Recv()
	if err != nil || initial.Revision != room.Revision {
		t.Fatal(initial, err)
	}
	joined, err := client.JoinLobby(f.contexts[1], req)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := watch.Recv()
	if err != nil || updated.Revision != joined.Revision || len(updated.Members) != 2 {
		t.Fatal(updated, err)
	}
	duplicate, err := client.JoinLobby(f.contexts[1], req)
	if err != nil || duplicate.Revision != joined.Revision {
		t.Fatal("rejoin", duplicate, err)
	}
	if _, err := client.ApplyRoomCommand(f.contexts[0], &pb.RoomCommand{RoomId: req.RoomId, Action: pb.RoomAction_START_MATCH, ExpectedRevision: initial.Revision}); status.Code(err) != codes.Aborted {
		t.Fatal("stale accepted", err)
	}
	if _, err := client.ApplyRoomCommand(f.contexts[0], &pb.RoomCommand{RoomId: req.RoomId, Action: pb.RoomAction_START_MATCH, ExpectedRevision: joined.Revision}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("unready start", err)
	}
	command := func(who int, action pb.RoomAction, ready bool) *pb.RoomSnapshot {
		t.Helper()
		current, e := client.GetLobby(f.contexts[who], req)
		if e != nil {
			t.Fatal(e)
		}
		result, e := client.ApplyRoomCommand(f.contexts[who], &pb.RoomCommand{RoomId: req.RoomId, Action: action, Ready: ready, ExpectedRevision: current.Revision})
		if e != nil {
			t.Fatal(e)
		}
		return result
	}
	command(0, pb.RoomAction_SET_READY, true)
	command(1, pb.RoomAction_SET_READY, true)
	owner, err := client.GetLobby(f.contexts[0], req)
	if err != nil || !owner.CanStart {
		t.Fatal(owner, err)
	}
	if _, err := f.client.CreateGame(f.contexts[1], &pb.CreateGameRequest{RoomId: req.RoomId}); status.Code(err) != codes.PermissionDenied {
		t.Fatal("alternate start bypass", err)
	}
	selected := command(0, pb.RoomAction_START_MATCH, false)
	if selected.Phase != pb.LobbyPhase_SELECTING || selected.MatchId == "" {
		t.Fatal(selected)
	}
	if _, err := client.ApplyRoomCommand(f.contexts[0], &pb.RoomCommand{RoomId: req.RoomId, Action: pb.RoomAction_START_MATCH, ExpectedRevision: owner.Revision}); status.Code(err) != codes.Aborted {
		t.Fatal("start replay", err)
	}
	if _, err := client.JoinLobby(f.contexts[1], req); err != nil {
		t.Fatal("active rejoin", err)
	}
	for _, ctx := range f.contexts {
		if _, err := f.client.RegisterCharacters(ctx, &pb.SelectionRequest{MatchId: selected.MatchId, DefinitionIds: []string{"zina", "jude", "dana"}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, ctx := range f.contexts {
		if _, err := f.client.Ready(ctx, &pb.GameRequest{MatchId: selected.MatchId}); err != nil {
			t.Fatal(err)
		}
	}
	battle, err := client.GetLobby(f.contexts[0], req)
	if err != nil || battle.Phase != pb.LobbyPhase_BATTLING || battle.Revision <= selected.Revision {
		t.Fatal(battle, err)
	}
	state, err := f.client.GetGameData(f.contexts[0], &pb.GameRequest{MatchId: selected.MatchId})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.client.Surrender(f.contexts[0], &pb.ActionRequest{MatchId: selected.MatchId, ExpectedRevision: state.State.Revision, CommandId: "lobby-finish"}); err != nil {
		t.Fatal(err)
	}
	waiting, err := client.GetLobby(f.contexts[0], req)
	if err != nil || waiting.Phase != pb.LobbyPhase_WAITING || waiting.MatchId != "" {
		t.Fatal(waiting, err)
	}
	for _, m := range waiting.Members {
		if m.Ready {
			t.Fatal("ready not reset")
		}
	}
	cancel()
	// A cancelled subscription must terminate, even if earlier updates were buffered.
	deadline := time.Now().Add(time.Second)
	for {
		_, err = watch.Recv()
		if err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("watch did not close")
		}
	}
	if status.Code(err) != codes.Canceled {
		t.Fatal(err)
	}
}

func TestLobbyOwnerTransferAndClosed(t *testing.T) {
	f := transport(t)
	c := pb.NewLobbyServiceV2Client(f.conn)
	room, err := c.CreateLobby(f.contexts[0], &pb.CreateLobbyRequest{RoomName: "leave"})
	if err != nil {
		t.Fatal(err)
	}
	req := &pb.LobbyRequest{RoomId: room.RoomId}
	if _, err := c.JoinLobby(f.contexts[1], req); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		before, err := c.GetLobby(f.contexts[i], req)
		if err != nil {
			t.Fatal(err)
		}
		after, err := c.ApplyRoomCommand(f.contexts[i], &pb.RoomCommand{RoomId: room.RoomId, Action: pb.RoomAction_LEAVE_ROOM, ExpectedRevision: before.Revision})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && (after.OwnerId != f.ids[1] || after.CanStart || len(after.Members) != 1) {
			t.Fatal(after)
		}
		if i == 1 && (after.Phase != pb.LobbyPhase_CLOSED || after.OwnerId != "") {
			t.Fatal(after)
		}
	}
	if _, err := c.GetLobby(f.contexts[1], req); status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
}
