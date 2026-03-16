package handlergrpc

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"auxilia/domain"
	repo "auxilia/domain/interface"
	"auxilia/pb"
)

type RoomHandler struct {
	pb.UnimplementedRoomServiceServer
	repo repo.RoomRepository
}

func NewRoomHandler(repo repo.RoomRepository) *RoomHandler {
	return &RoomHandler{repo: repo}
}

func (h *RoomHandler) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest) (*pb.JoinRoomResponse, error) {
	if err := h.repo.JoinRoom(req.RoomId, req.UserId); err != nil {
		if errors.Is(err, domain.ErrRoomNotFound) {
			return nil, status.Errorf(codes.NotFound, "room with ID %d not found", req.RoomId)
		}

		if errors.Is(err, domain.ErrRoomFull) {
			return nil, status.Errorf(codes.ResourceExhausted, "room with ID %d is full", req.RoomId)
		}

		if errors.Is(err, domain.ErrMatchStarted) {
			return nil, status.Errorf(codes.FailedPrecondition, "match in room with ID %d has already started", req.RoomId)
		}
		return nil, status.Error(codes.Internal, err.Error())
	}

	response, err := h.ListRoom(ctx, &pb.ListRoomRequest{RoomId: req.RoomId})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list rooms after joining: %v", err)
	}

	return &pb.JoinRoomResponse{
		Rooms: response.Rooms,
	}, nil

}

func (h *RoomHandler) ListRoom(ctx context.Context, req *pb.ListRoomRequest) (*pb.ListRoomResponse, error) {
	// 1. IDが同じ全てのroomを取得
	rooms, err := h.repo.ListRoom(ctx, req.RoomId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "部屋一覧の取得に失敗しました")
	}

	// 2. pb.Room のスライスに変換
	var pbRooms []*pb.Room
	for _, r := range rooms {
		pbRooms = append(pbRooms, &pb.Room{
			RoomId:   r.RoomID,
			UserId:   r.UserID,
			State:    r.State,
			IsReady:  r.IsReady,
			JoinedAt: r.JoinedAt.Format(time.RFC3339),
		})
	}

	return &pb.ListRoomResponse{
		Rooms: pbRooms,
	}, nil
}

func (h *RoomHandler) UpdateRoomState(ctx context.Context, req *pb.UpdateRoomStateRequest) (*pb.UpdateRoomStateResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.State < 0 || req.State > 2 {
		return nil, status.Error(codes.InvalidArgument, "state must be 0 (spectator), 1 (1P), or 2 (2P)")
	}

	if err := h.repo.UpdateRoomState(ctx, req.RoomId, req.UserId, req.State, req.IsReady); err != nil {
		if errors.Is(err, domain.ErrRoomNotFound) {
			return nil, status.Errorf(codes.NotFound, "user %s is not in room %d", req.UserId, req.RoomId)
		}
		return nil, status.Errorf(codes.Internal, "failed to update room state: %v", err)
	}

	response, err := h.ListRoom(ctx, &pb.ListRoomRequest{RoomId: req.RoomId})
	if err != nil {
		return nil, err
	}

	return &pb.UpdateRoomStateResponse{Rooms: response.Rooms}, nil
}

func (h *RoomHandler) StartMatch(ctx context.Context, req *pb.StartMatchRequest) (*pb.StartMatchResponse, error) {
	if err := h.repo.StartMatch(ctx, req.RoomId); err != nil {
		if errors.Is(err, domain.ErrRoomNotFound) {
			return nil, status.Errorf(codes.NotFound, "room with ID %d not found", req.RoomId)
		}
		if errors.Is(err, domain.ErrMatchStarted) {
			return nil, status.Errorf(codes.FailedPrecondition, "match in room with ID %d has already started", req.RoomId)
		}
		if errors.Is(err, domain.ErrNotAllUsersReady) || errors.Is(err, domain.ErrPlayerSlotsNotFilled) {
			return nil, status.Errorf(codes.FailedPrecondition, "%v", err)
		}
		return nil, status.Errorf(codes.Internal, "failed to start match: %v", err)
	}

	response, err := h.ListRoom(ctx, &pb.ListRoomRequest{RoomId: req.RoomId})
	if err != nil {
		return nil, err
	}

	return &pb.StartMatchResponse{
		Rooms:   response.Rooms,
		Started: true,
	}, nil
}
