package handlergrpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"auxilia/pb"
	repo "auxilia/domain/interface"
)

type RoomHandler struct {
	pb.UnimplementedRoomServiceServer
	repo repo.RoomRepository
}

func NewRoomHandler(repo repo.RoomRepository) *RoomHandler {
	return &RoomHandler{repo: repo}
}

var (
	ErrRoomNotFound = errors.New("room does not exist")
	ErrRoomFull     = errors.New("room is full")
)
func (h *RoomHandler) JoinRoom(ctx context.Context, req *pb.JoinRoomRequest) (*pb.JoinRoomResponse, error) {
	if err := h.repo.JoinRoom(req.RoomId, req.UserId); err != nil {
		if errors.Is(err, ErrRoomNotFound) {
			return nil, status.Errorf(codes.NotFound, "room with ID %d not found", req.RoomId)
		}

		if errors.Is(err, ErrRoomFull) {
			return nil, status.Errorf(codes.ResourceExhausted, "room with ID %d is full", req.RoomId)
		}
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	return &pb.JoinRoomResponse	{Success: true}, nil

}