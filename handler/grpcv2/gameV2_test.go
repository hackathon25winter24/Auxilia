package grpcv2

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	game "auxilia/domain/gamev2"
	"auxilia/domain/model"
	store "auxilia/infrastructure/gormv2"
	oldpb "auxilia/pb"
	pb "auxilia/pb/v2"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type transportFixture struct {
	store    *store.Store
	server   *grpc.Server
	client   pb.BattleServiceV2Client
	rooms    pb.RoomServiceV2Client
	matches  pb.RoomMatchServiceV2Client
	contexts [2]context.Context
	ids      [2]string
	conn     *grpc.ClientConn
}

func transport(t *testing.T) *transportFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() { raw.Close() })
	if err := db.AutoMigrate(&model.User{}, &model.Room{}, &model.RoomMatch{}); err != nil {
		t.Fatal(err)
	}
	s := store.New(db)
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alice", "bob"} {
		hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&model.User{Name: name, Hash: string(hash), Rate: 1500}).Error; err != nil {
			t.Fatal(err)
		}
	}
	server := grpc.NewServer()
	h := Register(server, s)
	oldpb.RegisterUserServiceServer(server, NewUserGuard(h))
	listener := bufconn.Listen(1024 * 1024)
	go server.Serve(listener)
	t.Cleanup(func() { server.Stop(); listener.Close() })
	conn, err := grpc.NewClient("passthrough:///v2-test", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	f := &transportFixture{store: s, server: server, conn: conn, client: pb.NewBattleServiceV2Client(conn), rooms: pb.NewRoomServiceV2Client(conn), matches: pb.NewRoomMatchServiceV2Client(conn)}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	for i, name := range []string{"alice", "bob"} {
		login, err := f.client.Login(ctx, &pb.LoginRequest{Name: name, Password: "test-password"})
		if err != nil {
			t.Fatal(err)
		}
		f.contexts[i] = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+login.Token)
		f.ids[i] = login.PlayerId
	}
	schedulerCtx, stop := context.WithCancel(ctx)
	t.Cleanup(stop)
	go h.Run(schedulerCtx)
	return f
}
func startRPC(t *testing.T, f *transportFixture) *pb.Snapshot {
	t.Helper()
	created, err := f.matches.CreateRoomMatch(f.contexts[0], &oldpb.CreateRoomMatchRequest{RoomName: "integration", OwnerId: f.ids[0]})
	if err != nil {
		t.Fatal(err)
	}
	room := created.Room.RoomId
	if _, err := f.rooms.JoinRoom(f.contexts[1], &oldpb.JoinRoomRequest{RoomId: room, UserId: f.ids[1]}); err != nil {
		t.Fatal(err)
	}
	for i, ctx := range f.contexts {
		if _, err := f.rooms.UpdateRoomState(ctx, &oldpb.UpdateRoomStateRequest{RoomId: room, UserId: f.ids[i], State: int32(i + 1), IsReady: true}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.rooms.StartMatch(f.contexts[0], &oldpb.StartMatchRequest{RoomId: room}); err != nil {
		t.Fatal(err)
	}
	v, err := f.client.GetRoomGame(f.contexts[0], &pb.CreateGameRequest{RoomId: uint32(room)})
	if err != nil {
		t.Fatal(err)
	}
	for _, ctx := range f.contexts {
		v, err = f.client.RegisterCharacters(ctx, &pb.SelectionRequest{MatchId: v.State.MatchId, DefinitionIds: []string{"zina", "jude", "dana"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, ctx := range f.contexts {
		v, err = f.client.Ready(ctx, &pb.GameRequest{MatchId: v.State.MatchId})
		if err != nil {
			t.Fatal(err)
		}
	}
	if !v.State.Started || len(v.State.Characters) != 6 {
		t.Fatal("bad initial snapshot")
	}
	return v
}
func TestGRPCLifecycleStreamsAndLegacyDisabled(t *testing.T) {
	f := transport(t)
	if _, err := f.client.GetDefinitions(context.Background(), &pb.Empty{}); status.Code(err) != codes.Unauthenticated {
		t.Fatal(err)
	}
	if _, err := oldpb.NewBattleServiceClient(f.conn).GetGameData(context.Background(), &oldpb.GetGameDataRequest{RoomId: 1}); status.Code(err) != codes.Unimplemented {
		t.Fatal("legacy battle registered", err)
	}
	if _, err := oldpb.NewRoomServiceClient(f.conn).ListRoom(context.Background(), &oldpb.ListRoomRequest{RoomId: 1}); status.Code(err) != codes.Unimplemented {
		t.Fatal("legacy lobby registered", err)
	}
	v := startRPC(t, f)
	if _, err := f.rooms.LeaveRoom(f.contexts[0], &oldpb.LeaveRoomRequest{RoomId: int32(v.RoomId), UserId: f.ids[0]}); status.Code(err) != codes.FailedPrecondition {
		t.Fatal("active membership changed", err)
	}
	if _, err := f.rooms.SetReady(f.contexts[0], &oldpb.SetReadyRequest{RoomId: int32(v.RoomId), UserId: f.ids[1], Ready: true}); status.Code(err) != codes.PermissionDenied {
		t.Fatal("identity spoof accepted", err)
	}
	stream, err := f.client.StreamGame(f.contexts[1], &pb.GameRequest{MatchId: v.State.MatchId})
	if err != nil {
		t.Fatal(err)
	}
	initial, err := stream.Recv()
	if err != nil || initial.State.Revision != v.State.Revision {
		t.Fatal("initial stream", err)
	}
	current := 0
	if v.State.TurnPlayerId == f.ids[1] {
		current = 1
	}
	cmd := &pb.ActionRequest{MatchId: v.State.MatchId, CommandId: "end", ExpectedRevision: v.State.Revision}
	changed, err := f.client.EndTurn(f.contexts[current], cmd)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := f.client.EndTurn(f.contexts[current], cmd)
	if err != nil || retry.LastLogSequence != changed.LastLogSequence {
		t.Fatal("retry not idempotent", err)
	}
	update, err := stream.Recv()
	if err != nil || update.State.Phase != "turn_end" {
		t.Fatal("missing turn end", err)
	}
	next, err := stream.Recv()
	if err != nil || next.State.Turn != 2 || next.State.Phase != "action" {
		t.Fatal("scheduler stream", err)
	}
	result, err := f.client.Surrender(f.contexts[0], &pb.ActionRequest{MatchId: v.State.MatchId, CommandId: "quit", ExpectedRevision: next.State.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if !result.State.Finished || result.P2Rate != 1516 {
		t.Fatal("bad settlement")
	}
	log, err := f.client.FetchActionLog(f.contexts[0], &pb.LogRequest{MatchId: v.State.MatchId})
	if err != nil {
		t.Fatal(err)
	}
	if len(log.Logs) == 0 || log.Logs[len(log.Logs)-1].Before == nil || !log.Logs[len(log.Logs)-1].After.Finished {
		t.Fatal("missing transition snapshots")
	}
}
func TestStateRoundTripPreservesEveryEngineField(t *testing.T) {
	st := game.NewState("all-fields", [2]game.Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"louise", "kasuima", "suima"}, {"liberette", "zina", "verbulus"}})
	st.Characters[0].CombatStance = true
	st.Characters[0].UsedSkills = map[string]int{"dance": 2}
	st.Characters[0].TemporaryBuffs = []string{"俊足"}
	st.Characters[0].HangoverUntil = 7
	st.Characters[0].HangoverTurn = 5
	st.Characters[0].BarrierTurn = 3
	st.Characters[0].DepartureUsed = true
	st.Characters[0].ReviveUsed = true
	st.Characters[0].DrankTurn = 3
	st.Characters[0].Wriggling = true
	st.TileEffects = []game.TileEffect{{Position: game.Position{X: 2, Y: 1}, Type: "毒ガス", OwnerID: "a", HP: 10}}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	p, err := stateProto(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	c := p.Characters[0]
	if c.MaxHp != 100 || !c.CombatStance || c.UsedSkills["dance"] != 2 || c.HangoverUntil != 7 || !c.Wriggling || !c.ReviveUsed || p.TileEffects[0].Hp != 10 || p.TurnDeadline == "" {
		t.Fatal("state lost fields")
	}
}
func TestGRPCWebUnaryAndServerStream(t *testing.T) {
	f := transport(t)
	v := startRPC(t, f)
	wrapped := grpcweb.WrapServer(f.server, grpcweb.WithOriginFunc(func(string) bool { return true }))
	server := httptest.NewServer(wrapped)
	defer server.Close()
	md, _ := metadata.FromOutgoingContext(f.contexts[0])
	token := md.Get("authorization")[0]
	call := func(method string, message proto.Message) *http.Response {
		t.Helper()
		raw, err := proto.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		frame := make([]byte, 5+len(raw))
		binary.BigEndian.PutUint32(frame[1:5], uint32(len(raw)))
		copy(frame[5:], raw)
		req, err := http.NewRequestWithContext(f.contexts[0], "POST", server.URL+"/game.network.v2.BattleServiceV2/"+method, bytes.NewReader(frame))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/grpc-web+proto")
		req.Header.Set("X-Grpc-Web", "1")
		req.Header.Set("Authorization", token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	readSnapshot := func(resp *http.Response) *pb.Snapshot {
		t.Helper()
		header := make([]byte, 5)
		if _, err := io.ReadFull(resp.Body, header); err != nil {
			t.Fatal(err)
		}
		if header[0] != 0 {
			t.Fatalf("expected data frame: %v", header)
		}
		raw := make([]byte, binary.BigEndian.Uint32(header[1:]))
		if _, err := io.ReadFull(resp.Body, raw); err != nil {
			t.Fatal(err)
		}
		out := &pb.Snapshot{}
		if err := proto.Unmarshal(raw, out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	unary := call("GetGameData", &pb.GameRequest{MatchId: v.State.MatchId})
	if got := readSnapshot(unary); got.State.MatchId != v.State.MatchId {
		t.Fatal("grpc-web snapshot mismatch")
	}
	unary.Body.Close()
	streaming := call("StreamGame", &pb.GameRequest{MatchId: v.State.MatchId})
	defer streaming.Body.Close()
	if got := readSnapshot(streaming); got.State.Revision != v.State.Revision {
		t.Fatal("grpc-web initial snapshot mismatch")
	}
	current := 0
	if v.State.TurnPlayerId == f.ids[1] {
		current = 1
	}
	if _, err := f.client.EndTurn(f.contexts[current], &pb.ActionRequest{MatchId: v.State.MatchId, CommandId: "web-end", ExpectedRevision: v.State.Revision}); err != nil {
		t.Fatal(err)
	}
	if got := readSnapshot(streaming); got.State.Phase != "turn_end" {
		t.Fatal("grpc-web update missing")
	}
}
