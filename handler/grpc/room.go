package handlergrpc

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"auxilia/domain"
	repo "auxilia/domain/interface"
	"auxilia/pb"
)

var (
	roomLobbyStreams   = make(map[int32][]pb.RoomService_StreamRoomServer)
	roomLobbyStreamsMu sync.Mutex
)

type RoomHandler struct {
	pb.UnimplementedRoomServiceServer
	repo       repo.RoomRepository
	battleRepo repo.BattleRepository
}

func NewRoomHandler(repo repo.RoomRepository, battleRepo repo.BattleRepository) *RoomHandler {
	return &RoomHandler{repo: repo, battleRepo: battleRepo}
}

func (h *RoomHandler) StreamRoom(stream pb.RoomService_StreamRoomServer) error {
	req, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}

	roomID := req.RoomId

	roomLobbyStreamsMu.Lock()
	roomLobbyStreams[roomID] = append(roomLobbyStreams[roomID], stream)
	roomLobbyStreamsMu.Unlock()

	defer func() {
		roomLobbyStreamsMu.Lock()
		streams := roomLobbyStreams[roomID]
		for i, s := range streams {
			if s == stream {
				roomLobbyStreams[roomID] = append(streams[:i], streams[i+1:]...)
				break
			}
		}
		roomLobbyStreamsMu.Unlock()
	}()

	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (h *RoomHandler) broadcastToRoom(roomID int32, response *pb.ListRoomResponse) {
	roomLobbyStreamsMu.Lock()
	streams := roomLobbyStreams[roomID]
	roomLobbyStreamsMu.Unlock()
	for _, s := range streams {
		_ = s.Send(response)
	}
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

	h.broadcastToRoom(req.RoomId, response)

	return &pb.JoinRoomResponse{
		Rooms: response.Rooms,
	}, nil

}

func (h *RoomHandler) LeaveRoom(ctx context.Context, req *pb.LeaveRoomRequest) (*pb.LeaveRoomResponse, error) {
	if err := h.repo.LeaveRoom(req.RoomId, req.UserId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	response, err := h.ListRoom(ctx, &pb.ListRoomRequest{RoomId: req.RoomId})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list rooms after leaving: %v", err)
	}

	h.broadcastToRoom(req.RoomId, response)

	return &pb.LeaveRoomResponse{
		Rooms: response.Rooms,
	}, nil
}

func (h *RoomHandler) ListRoom(ctx context.Context, req *pb.ListRoomRequest) (*pb.ListRoomResponse, error) {
	// 1. IDが同じ全てのroomを取得
	rooms, err := h.repo.ListRoom(ctx, req.RoomId)
	if err != nil {
		return nil, status.Error(codes.Internal, "部屋一覧の取得に失敗しました")
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

func (h *RoomHandler) EnterRing(ctx context.Context, req *pb.EnterRingRequest) (*pb.EnterRingResponse, error) {
	if err := h.repo.EnterRing(req.RoomId, req.UserId); err != nil {

		if errors.Is(err, domain.ErrRingFull) {
			return nil, status.Errorf(codes.FailedPrecondition, "ring in room %d is full", req.RoomId)
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	response, err := h.ListRoom(ctx, &pb.ListRoomRequest{RoomId: req.RoomId})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list rooms after entering ring: %v", err)
	}

	h.broadcastToRoom(req.RoomId, response)

	return &pb.EnterRingResponse{
		Rooms: response.Rooms,
	}, nil
}

func (h *RoomHandler) LeaveRing(ctx context.Context, req *pb.LeaveRingRequest) (*pb.LeaveRingResponse, error) {
	if err := h.repo.LeaveRing(req.RoomId, req.UserId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	response, err := h.ListRoom(ctx, &pb.ListRoomRequest{RoomId: req.RoomId})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list rooms after leaving ring: %v", err)
	}

	h.broadcastToRoom(req.RoomId, response)

	return &pb.LeaveRingResponse{
		Rooms: response.Rooms,
	}, nil
}

func (h *RoomHandler) SetReady(ctx context.Context, req *pb.SetReadyRequest) (*pb.SetReadyResponse, error) {
	if err := h.repo.SetReady(ctx, req.RoomId, req.UserId, req.Ready); err != nil {

		if errors.Is(err, domain.ErrSpectatorCannotReady) {
			return nil, status.Error(codes.FailedPrecondition, "spectator cannot ready")
		}

		return nil, status.Error(codes.Internal, err.Error())
	}

	// 試合開始チェック
	p1, p2, err := h.repo.StartMatch(ctx, req.RoomId)
	if err == nil {
		_, err = h.battleRepo.CreateGame(uint32(req.RoomId), p1, p2)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	} else if !errors.Is(err, domain.ErrMatchStarted) &&
		!errors.Is(err, domain.ErrNotAllUsersReady) &&
		!errors.Is(err, domain.ErrPlayerSlotsNotFilled) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	response, err := h.ListRoom(ctx, &pb.ListRoomRequest{RoomId: req.RoomId})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list rooms after ready change: %v", err)
	}

	h.broadcastToRoom(req.RoomId, response)

	return &pb.SetReadyResponse{
		Rooms: response.Rooms,
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

	h.broadcastToRoom(req.RoomId, response)

	return &pb.UpdateRoomStateResponse{Rooms: response.Rooms}, nil
}

func (h *RoomHandler) StartMatch(ctx context.Context, req *pb.StartMatchRequest) (*pb.StartMatchResponse, error) {
	p1ID, p2ID, err := h.repo.StartMatch(ctx, req.RoomId)
	if err != nil {
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

	_, err = h.battleRepo.CreateGame(uint32(req.RoomId), p1ID, p2ID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create game: %v", err)
	}

	response, err := h.ListRoom(ctx, &pb.ListRoomRequest{RoomId: req.RoomId})
	if err != nil {
		return nil, err
	}

	h.broadcastToRoom(req.RoomId, response)

	return &pb.StartMatchResponse{
		Rooms:   response.Rooms,
		Started: true,
	}, nil
}
