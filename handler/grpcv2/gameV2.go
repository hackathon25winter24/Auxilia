package grpcv2

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	game "auxilia/domain/gamev2"
	store "auxilia/infrastructure/gormv2"
	pb "auxilia/pb/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
)

type Handler struct {
	pb.UnimplementedBattleServiceV2Server
	Store *store.Store
}

func New(s *store.Store) *Handler { return &Handler{Store: s} }

// Register is shared by the executable and transport integration tests.
func Register(server *grpc.Server, s *store.Store) *Handler {
	h := New(s)
	pb.RegisterBattleServiceV2Server(server, h)
	pb.RegisterRoomServiceV2Server(server, NewRoom(h))
	pb.RegisterRoomMatchServiceV2Server(server, NewRoomMatch(h))
	return h
}
func rpcError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, store.ErrUnauthenticated):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, store.ErrForbidden):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, gorm.ErrRecordNotFound):
		return status.Error(codes.NotFound, "match or room not found")
	case errors.Is(err, game.ErrStaleRevision):
		return status.Error(codes.Aborted, err.Error())
	case errors.Is(err, store.ErrPrecondition), errors.Is(err, game.ErrNotYourTurn):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, game.ErrInvalidAction):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, store.ErrCommandConflict):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, context.Canceled):
		return status.Error(codes.Canceled, "request cancelled")
	case errors.Is(err, context.DeadlineExceeded):
		return status.Error(codes.DeadlineExceeded, "deadline exceeded")
	default:
		log.Printf("battle V2: %v", err)
		return status.Error(codes.Internal, "battle operation failed")
	}
}
func (h *Handler) player(ctx context.Context) (string, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") {
		return "", status.Error(codes.Unauthenticated, "Bearer token required")
	}
	id, err := h.Store.Authenticate(ctx, strings.TrimPrefix(values[0], "Bearer "))
	return id, rpcError(err)
}
func stateProto(raw string) (*pb.State, error) {
	s := &pb.State{}
	if err := protojson.Unmarshal([]byte(raw), s); err != nil {
		return nil, err
	}
	return s, nil
}
func snapshot(v *store.View, err error) (*pb.Snapshot, error) {
	if err != nil {
		return nil, rpcError(err)
	}
	v.State.ServerTime = time.Now().UTC()
	raw, err := json.Marshal(v.State)
	if err != nil {
		return nil, rpcError(err)
	}
	state, err := stateProto(string(raw))
	if err != nil {
		return nil, rpcError(err)
	}
	result := &pb.Snapshot{RoomId: v.Match.RoomID, State: state, LastLogSequence: v.Match.LogSequence, RulesVersion: store.RulesVersion, P1Rate: int32(v.Match.P1Rate), P2Rate: int32(v.Match.P2Rate), P1RateDelta: int32(v.Match.P1RateDelta), P2RateDelta: int32(v.Match.P2RateDelta)}
	for i, picks := range v.Selections {
		if len(picks) == 3 {
			result.SelectedPlayerIds = append(result.SelectedPlayerIds, v.State.Players[i].ID)
		}
	}
	return result, nil
}
func (h *Handler) Login(ctx context.Context, r *pb.LoginRequest) (*pb.LoginResponse, error) {
	token, id, expires, err := h.Store.Login(ctx, r.Name, r.Password)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.LoginResponse{Token: token, PlayerId: id, ExpiresAt: expires.Format(time.RFC3339Nano)}, nil
}
func (h *Handler) GetDefinitions(ctx context.Context, _ *pb.Empty) (*pb.DefinitionsResponse, error) {
	if _, err := h.player(ctx); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(game.Definitions)
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.DefinitionsResponse{DefinitionsJson: string(raw), RulesVersion: store.RulesVersion}, nil
}
func (h *Handler) CreateGame(ctx context.Context, r *pb.CreateGameRequest) (*pb.Snapshot, error) {
	id, err := h.player(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot(h.Store.Create(ctx, r.RoomId, id))
}

func (h *Handler) GetRoomGame(ctx context.Context, r *pb.CreateGameRequest) (*pb.Snapshot, error) {
	id, err := h.player(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot(h.Store.RoomGame(ctx, r.RoomId, id))
}
func (h *Handler) RegisterCharacters(ctx context.Context, r *pb.SelectionRequest) (*pb.Snapshot, error) {
	id, err := h.player(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot(h.Store.Select(ctx, r.MatchId, id, r.DefinitionIds))
}
func (h *Handler) Ready(ctx context.Context, r *pb.GameRequest) (*pb.Snapshot, error) {
	id, err := h.player(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot(h.Store.Ready(ctx, r.MatchId, id))
}
func (h *Handler) CancelGame(ctx context.Context, r *pb.GameRequest) (*pb.Snapshot, error) {
	id, err := h.player(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot(h.Store.Cancel(ctx, r.MatchId, id))
}
func (h *Handler) GetGameData(ctx context.Context, r *pb.GameRequest) (*pb.Snapshot, error) {
	id, err := h.player(ctx)
	if err != nil {
		return nil, err
	}
	return snapshot(h.Store.Read(ctx, r.MatchId, id))
}
func (h *Handler) apply(ctx context.Context, r *pb.ActionRequest, kind string) (*pb.Snapshot, error) {
	id, err := h.player(ctx)
	if err != nil {
		return nil, err
	}
	c := game.Command{ID: r.CommandId, ExpectedRevision: r.ExpectedRevision, CharacterID: r.CharacterId, AttackIndex: int(r.AttackIndex)}
	if r.Target != nil {
		c.Target = game.Position{X: int(r.Target.X), Y: int(r.Target.Y)}
	}
	if r.Direction != nil {
		c.Direction = game.Position{X: int(r.Direction.X), Y: int(r.Direction.Y)}
	}
	if (kind == "MOVE" || kind == "ATTACK") && r.Target == nil {
		return nil, status.Error(codes.InvalidArgument, "target required")
	}
	if kind == "ATTACK" && (r.Direction == nil || abs(c.Direction.X)+abs(c.Direction.Y) != 1) {
		return nil, status.Error(codes.InvalidArgument, "cardinal direction required")
	}
	return snapshot(h.Store.Apply(ctx, r.MatchId, id, kind, c))
}
func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}
func (h *Handler) ApplyMove(ctx context.Context, r *pb.ActionRequest) (*pb.Snapshot, error) {
	return h.apply(ctx, r, "MOVE")
}
func (h *Handler) ApplyAttack(ctx context.Context, r *pb.ActionRequest) (*pb.Snapshot, error) {
	return h.apply(ctx, r, "ATTACK")
}
func (h *Handler) EndTurn(ctx context.Context, r *pb.ActionRequest) (*pb.Snapshot, error) {
	return h.apply(ctx, r, "END_TURN")
}
func (h *Handler) Surrender(ctx context.Context, r *pb.ActionRequest) (*pb.Snapshot, error) {
	return h.apply(ctx, r, "SURRENDER")
}

// One sender per stream; DB revisions also propagate commits made by other replicas.
func (h *Handler) StreamGame(r *pb.GameRequest, stream grpc.ServerStreamingServer[pb.Snapshot]) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var last uint64
	for {
		id, err := h.player(stream.Context())
		if err != nil {
			return err
		}
		v, err := h.Store.Read(stream.Context(), r.MatchId, id)
		if err != nil {
			return rpcError(err)
		}
		if last != v.Match.LogSequence {
			resp, err := snapshot(v, nil)
			if err != nil {
				return err
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
			last = v.Match.LogSequence
		}
		if v.State.Finished {
			return nil
		}
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case <-ticker.C:
		}
	}
}
func (h *Handler) FetchActionLog(ctx context.Context, r *pb.LogRequest) (*pb.LogResponse, error) {
	id, err := h.player(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.Store.Logs(ctx, r.MatchId, id, r.AfterSequence, r.Limit)
	if err != nil {
		return nil, rpcError(err)
	}
	result := &pb.LogResponse{NextSequence: r.AfterSequence}
	for _, row := range rows {
		entry := &pb.ActionLog{Sequence: row.Sequence, PlayerId: row.PlayerID, ActionType: row.ActionType}
		if row.BeforeJSON != "" {
			entry.Before, err = stateProto(row.BeforeJSON)
			if err != nil {
				return nil, rpcError(err)
			}
		}
		entry.After, err = stateProto(row.AfterJSON)
		if err != nil {
			return nil, rpcError(err)
		}
		if row.CommandJSON != "" {
			var c game.Command
			if err := json.Unmarshal([]byte(row.CommandJSON), &c); err != nil {
				return nil, rpcError(err)
			}
			entry.Command = &pb.ActionRequest{MatchId: r.MatchId, CommandId: c.ID, ExpectedRevision: c.ExpectedRevision, CharacterId: c.CharacterID, AttackIndex: int32(c.AttackIndex), Target: &pb.Position{X: int32(c.Target.X), Y: int32(c.Target.Y)}, Direction: &pb.Position{X: int32(c.Direction.X), Y: int32(c.Direction.Y)}}
		}
		result.Logs = append(result.Logs, entry)
		result.NextSequence = row.Sequence
	}
	return result, nil
}
func (h *Handler) Run(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := h.Store.Tick(ctx, time.Now()); err != nil && ctx.Err() == nil {
			log.Printf("battle V2 scheduler: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
