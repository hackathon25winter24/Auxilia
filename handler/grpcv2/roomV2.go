package grpcv2

import (
	"auxilia/domain"
	"auxilia/domain/model"
	legacy "auxilia/infrastructure/gorm"
	store "auxilia/infrastructure/gormv2"
	"auxilia/pb"
	pbv2 "auxilia/pb/v2"
	"context"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"time"
)

// Keeps Auxilia's lobby protocol, with explicit V2 start and locked membership.
type RoomHandler struct {
	pbv2.UnimplementedRoomServiceV2Server
	Battle *Handler
}

func NewRoom(h *Handler) *RoomHandler { return &RoomHandler{Battle: h} }
func roomError(err error) error {
	switch {
	case errors.Is(err, domain.ErrRoomFull), errors.Is(err, domain.ErrRingFull):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, domain.ErrMatchStarted), errors.Is(err, domain.ErrSpectatorCannotReady):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrRoomNotFound):
		return status.Error(codes.NotFound, err.Error())
	}
	return rpcError(err)
}
func (h *RoomHandler) mutate(ctx context.Context, room int32, user string, fn func(*legacy.RoomRepository) error) error {
	id, err := h.Battle.player(ctx)
	if err != nil {
		return err
	}
	if id != user {
		return status.Error(codes.PermissionDenied, "user_id must match session")
	}
	if room <= 0 {
		return status.Error(codes.InvalidArgument, "invalid room_id")
	}
	err = h.Battle.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var m model.RoomMatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&m, "id = ?", room).Error; err != nil {
			return err
		}
		if m.IsGaming {
			return store.ErrPrecondition
		}
		if err := fn(legacy.NewRoomRepository(tx)); err != nil {
			return err
		}
		var players []model.Room
		if err := tx.Where("room_id = ? AND state IN ?", room, []int{1, 2}).Find(&players).Error; err != nil {
			return err
		}
		occupied := map[int32]bool{}
		for _, p := range players {
			if occupied[p.State] {
				return domain.ErrRingFull
			}
			occupied[p.State] = true
		}
		return nil
	})
	return roomError(err)
}
func (h *RoomHandler) list(ctx context.Context, room int32) ([]*pb.Room, error) {
	rows, err := legacy.NewRoomRepository(h.Battle.Store.DB).ListRoom(ctx, room)
	if err != nil {
		return nil, roomError(err)
	}
	result := make([]*pb.Room, 0, len(rows))
	for _, r := range rows {
		result = append(result, &pb.Room{RoomId: r.RoomID, UserId: r.UserID, State: r.State, IsReady: r.IsReady, JoinedAt: r.JoinedAt.Format(time.RFC3339)})
	}
	return result, nil
}
func (h *RoomHandler) JoinRoom(ctx context.Context, r *pb.JoinRoomRequest) (*pb.JoinRoomResponse, error) {
	err := h.mutate(ctx, r.RoomId, r.UserId, func(repo *legacy.RoomRepository) error { _, err := repo.JoinRoom(r.RoomId, r.UserId); return err })
	if err != nil {
		return nil, err
	}
	rows, err := h.list(ctx, r.RoomId)
	return &pb.JoinRoomResponse{Rooms: rows}, err
}
func (h *RoomHandler) LeaveRoom(ctx context.Context, r *pb.LeaveRoomRequest) (*pb.LeaveRoomResponse, error) {
	err := h.mutate(ctx, r.RoomId, r.UserId, func(repo *legacy.RoomRepository) error { return repo.LeaveRoom(r.RoomId, r.UserId) })
	if err != nil {
		return nil, err
	}
	rows, err := h.list(ctx, r.RoomId)
	return &pb.LeaveRoomResponse{Rooms: rows}, err
}
func (h *RoomHandler) EnterRing(ctx context.Context, r *pb.EnterRingRequest) (*pb.EnterRingResponse, error) {
	err := h.mutate(ctx, r.RoomId, r.UserId, func(repo *legacy.RoomRepository) error { return repo.EnterRing(r.RoomId, r.UserId) })
	if err != nil {
		return nil, err
	}
	rows, err := h.list(ctx, r.RoomId)
	return &pb.EnterRingResponse{Rooms: rows}, err
}
func (h *RoomHandler) LeaveRing(ctx context.Context, r *pb.LeaveRingRequest) (*pb.LeaveRingResponse, error) {
	err := h.mutate(ctx, r.RoomId, r.UserId, func(repo *legacy.RoomRepository) error { return repo.LeaveRing(r.RoomId, r.UserId) })
	if err != nil {
		return nil, err
	}
	rows, err := h.list(ctx, r.RoomId)
	return &pb.LeaveRingResponse{Rooms: rows}, err
}
func (h *RoomHandler) SetReady(ctx context.Context, r *pb.SetReadyRequest) (*pb.SetReadyResponse, error) {
	err := h.mutate(ctx, r.RoomId, r.UserId, func(repo *legacy.RoomRepository) error { return repo.SetReady(ctx, r.RoomId, r.UserId, r.Ready) })
	if err != nil {
		return nil, err
	}
	rows, err := h.list(ctx, r.RoomId)
	return &pb.SetReadyResponse{Rooms: rows}, err
}
func (h *RoomHandler) UpdateRoomState(ctx context.Context, r *pb.UpdateRoomStateRequest) (*pb.UpdateRoomStateResponse, error) {
	if r.State < 0 || r.State > 2 {
		return nil, status.Error(codes.InvalidArgument, "invalid state")
	}
	err := h.mutate(ctx, r.RoomId, r.UserId, func(repo *legacy.RoomRepository) error {
		return repo.UpdateRoomState(ctx, r.RoomId, r.UserId, r.State, r.IsReady)
	})
	if err != nil {
		return nil, err
	}
	rows, err := h.list(ctx, r.RoomId)
	return &pb.UpdateRoomStateResponse{Rooms: rows}, err
}
func (h *RoomHandler) ListRoom(ctx context.Context, r *pb.ListRoomRequest) (*pb.ListRoomResponse, error) {
	if _, err := h.Battle.player(ctx); err != nil {
		return nil, err
	}
	rows, err := h.list(ctx, r.RoomId)
	return &pb.ListRoomResponse{Rooms: rows}, err
}
func (h *RoomHandler) StartMatch(ctx context.Context, r *pb.StartMatchRequest) (*pb.StartMatchResponse, error) {
	if r.RoomId <= 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid room_id")
	}
	id, err := h.Battle.player(ctx)
	if err != nil {
		return nil, err
	}
	if _, err = h.Battle.Store.Create(ctx, uint32(r.RoomId), id); err != nil {
		return nil, rpcError(err)
	}
	rows, err := h.list(ctx, r.RoomId)
	return &pb.StartMatchResponse{Rooms: rows, Started: true}, err
}
func (h *RoomHandler) StreamRoom(stream grpc.BidiStreamingServer[pb.RoomStreamRequest, pb.ListRoomResponse]) error {
	id, err := h.Battle.player(stream.Context())
	if err != nil {
		return err
	}
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	if req.UserId != id {
		return status.Error(codes.PermissionDenied, "user_id must match session")
	}
	// Read until EOF without concurrent Send calls; supports clients that keep sending heartbeats.
	done := make(chan error, 1)
	go func() {
		for {
			_, err := stream.Recv()
			if err != nil {
				done <- err
				return
			}
		}
	}()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := h.Battle.player(stream.Context()); err != nil {
			return err
		}
		rows, err := h.list(stream.Context(), req.RoomId)
		if err != nil {
			return err
		}
		if err := stream.Send(&pb.ListRoomResponse{Rooms: rows}); err != nil {
			return err
		}
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case err := <-done:
			if err != io.EOF {
				return err
			}
			done = nil
		case <-ticker.C:
		}
	}
}
