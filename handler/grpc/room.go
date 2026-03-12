package handlergrpc

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"auxilia/pb"
	repo "auxilia/domain/interface"
	"auxilia/domain"
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
		return nil, status.Errorf(codes.Internal, err.Error())
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
			RoomId:  r.RoomID,
			UserId:  r.UserID,
			State:   r.State,
			IsReady: r.IsReady,
			JoinedAt: r.JoinedAt.Format(time.RFC3339),
		})
	}

	return &pb.ListRoomResponse{
		Rooms: pbRooms,
	}, nil
}