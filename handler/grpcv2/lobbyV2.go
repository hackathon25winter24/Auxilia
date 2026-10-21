package grpcv2

import (
	pb "auxilia/pb/v2"
	"context"
	"google.golang.org/grpc"
	"time"
)

type LobbyHandler struct {
	pb.UnimplementedLobbyServiceV2Server
	Battle *Handler
}

func (h *LobbyHandler) CreateLobby(ctx context.Context, r *pb.CreateLobbyRequest) (*pb.RoomSnapshot, error) {
	id, err := h.Battle.player(ctx)
	if err != nil {
		return nil, err
	}
	result, err := h.Battle.Store.CreateLobby(ctx, id, r.RoomName)
	return result, rpcError(err)
}
func (h *LobbyHandler) JoinLobby(ctx context.Context, r *pb.LobbyRequest) (*pb.RoomSnapshot, error) {
	id, err := h.Battle.player(ctx)
	if err != nil {
		return nil, err
	}
	result, err := h.Battle.Store.JoinLobby(ctx, r.RoomId, id)
	return result, rpcError(err)
}
func (h *LobbyHandler) GetLobby(ctx context.Context, r *pb.LobbyRequest) (*pb.RoomSnapshot, error) {
	id, err := h.Battle.player(ctx)
	if err != nil {
		return nil, err
	}
	result, err := h.Battle.Store.GetLobby(ctx, r.RoomId, id)
	return result, rpcError(err)
}
func (h *LobbyHandler) ApplyRoomCommand(ctx context.Context, r *pb.RoomCommand) (*pb.RoomSnapshot, error) {
	id, err := h.Battle.player(ctx)
	if err != nil {
		return nil, err
	}
	result, err := h.Battle.Store.ApplyRoomCommand(ctx, id, r)
	return result, rpcError(err)
}
func (h *LobbyHandler) WatchRoom(r *pb.LobbyRequest, stream grpc.ServerStreamingServer[pb.RoomSnapshot]) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	var revision uint64
	for {
		// Revalidates session and membership on each iteration, including reconnects.
		snapshot, err := h.GetLobby(stream.Context(), r)
		if err != nil {
			return err
		}
		if snapshot.Revision != revision {
			if err := stream.Send(snapshot); err != nil {
				return err
			}
			revision = snapshot.Revision
		}
		select {
		case <-stream.Context().Done():
			return rpcError(stream.Context().Err())
		case <-ticker.C:
		}
	}
}
