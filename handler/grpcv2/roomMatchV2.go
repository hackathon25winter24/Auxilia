package grpcv2

import (
	"auxilia/domain/model"
	store "auxilia/infrastructure/gormv2"
	"auxilia/pb"
	pbv2 "auxilia/pb/v2"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
)

type RoomMatchHandler struct {
	pbv2.UnimplementedRoomMatchServiceV2Server
	Battle *Handler
}

func NewRoomMatch(h *Handler) *RoomMatchHandler { return &RoomMatchHandler{Battle: h} }
func roomPB(m model.RoomMatch) *pb.RoomMatch {
	return &pb.RoomMatch{RoomId: int32(m.ID), RoomName: m.RoomName, OwnerId: m.OwnerID, IsGaming: m.IsGaming}
}
func (h *RoomMatchHandler) CreateRoomMatch(ctx context.Context, r *pb.CreateRoomMatchRequest) (*pb.RoomMatchResponse, error) {
	id, err := h.Battle.player(ctx)
	if err != nil {
		return nil, err
	}
	if r.OwnerId != id || r.IsGaming || strings.TrimSpace(r.RoomName) == "" || len(r.RoomName) > 128 {
		return nil, status.Error(codes.InvalidArgument, "invalid room owner, name or gaming flag")
	}
	m := model.RoomMatch{RoomName: r.RoomName, OwnerID: id}
	err = h.Battle.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		return tx.Create(&model.Room{RoomID: int32(m.ID), UserID: id, State: model.StateSpectator}).Error
	})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.RoomMatchResponse{Room: roomPB(m)}, nil
}
func (h *RoomMatchHandler) ListRoomMatch(ctx context.Context, _ *pb.ListRoomMatchRequest) (*pb.ListRoomMatchResponse, error) {
	if _, err := h.Battle.player(ctx); err != nil {
		return nil, err
	}
	var rows []model.RoomMatch
	if err := h.Battle.Store.DB.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, rpcError(err)
	}
	out := &pb.ListRoomMatchResponse{}
	for _, m := range rows {
		out.Rooms = append(out.Rooms, roomPB(m))
	}
	return out, nil
}
func (h *RoomMatchHandler) UpdateRoomMatch(ctx context.Context, r *pb.UpdateRoomMatchRequest) (*pb.RoomMatchResponse, error) {
	id, err := h.Battle.player(ctx)
	if err != nil {
		return nil, err
	}
	var m model.RoomMatch
	if strings.TrimSpace(r.RoomName) == "" || len(r.RoomName) > 128 {
		return nil, status.Error(codes.InvalidArgument, "invalid room name")
	}
	err = h.Battle.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&m, "id = ?", r.RoomId).Error; err != nil {
			return err
		}
		if m.OwnerID != id || r.OwnerId != id {
			return store.ErrForbidden
		}
		if m.IsGaming || r.IsGaming {
			return store.ErrPrecondition
		}
		m.RoomName = r.RoomName
		return tx.Save(&m).Error
	})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.RoomMatchResponse{Room: roomPB(m)}, nil
}
