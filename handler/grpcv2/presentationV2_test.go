package grpcv2

import (
	game "auxilia/domain/gamev2"
	store "auxilia/infrastructure/gormv2"
	pb "auxilia/pb/v2"
	"google.golang.org/protobuf/proto"
	"testing"
)

func lastPresentation(t *testing.T, s *pb.Snapshot) *pb.PresentationBatch {
	t.Helper()
	if len(s.PresentationBatches) == 0 {
		t.Fatal("snapshot omitted presentation events")
	}
	last := s.PresentationBatches[len(s.PresentationBatches)-1]
	if last.Version != 1 || last.Sequence != s.LastLogSequence || last.AfterRevision != s.State.Revision || s.PresentationFromSequence != s.PresentationBatches[0].Sequence {
		t.Fatal("bad batch metadata")
	}
	return last
}
func TestPresentationTransportIncludesMissedUpdatesAndReplayIdentity(t *testing.T) {
	f := transport(t)
	v := startRPC(t, f)
	lastPresentation(t, v)
	current := 0
	if v.State.TurnPlayerId == f.ids[1] {
		current = 1
	}
	first, err := f.client.EndTurn(f.contexts[current], &pb.ActionRequest{MatchId: v.State.MatchId, CommandId: "end-present", ExpectedRevision: v.State.Revision})
	if err != nil {
		t.Fatal(err)
	}
	eventBatch := lastPresentation(t, first)
	if eventBatch.CommandId != "end-present" || len(eventBatch.Events) == 0 {
		t.Fatal("empty turn-end presentation")
	}
	// A second action can finish before a stream poll. Both batches must survive.
	request := &pb.ActionRequest{MatchId: v.State.MatchId, CommandId: "quit-present", ExpectedRevision: first.State.Revision}
	finished, err := f.client.Surrender(f.contexts[0], request)
	if err != nil {
		t.Fatal(err)
	}
	last := lastPresentation(t, finished)
	if finished.PresentationBatches[len(finished.PresentationBatches)-2].Sequence != eventBatch.Sequence {
		t.Fatal("missed intermediate transition")
	}
	stream, err := f.client.StreamGame(f.contexts[1], &pb.GameRequest{MatchId: v.State.MatchId})
	if err != nil {
		t.Fatal(err)
	}
	streamed, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(last, lastPresentation(t, streamed)) {
		t.Fatal("unary/stream events differ")
	}
	retry, err := f.client.Surrender(f.contexts[0], request)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(last, lastPresentation(t, retry)) {
		t.Fatal("retry changed event identity")
	}
	history, err := f.client.FetchActionLog(f.contexts[1], &pb.LogRequest{MatchId: v.State.MatchId, AfterSequence: first.LastLogSequence, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Logs) != 1 || !proto.Equal(history.Logs[0].Presentation, last) {
		t.Fatal("history/stream events differ")
	}
}
func TestPresentationLegacySnapshotSignalsEmptyWindow(t *testing.T) {
	st := game.NewPendingState("legacy", [2]game.Player{{ID: "a"}, {ID: "b"}}, [2][]string{})
	result, err := snapshot(&store.View{State: st, Match: store.Match{ID: "legacy", LogSequence: 12}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PresentationBatches) != 0 || result.PresentationFromSequence != 13 {
		t.Fatal("legacy events fabricated")
	}
}
