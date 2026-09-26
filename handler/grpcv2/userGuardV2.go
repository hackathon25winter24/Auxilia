package grpcv2

import (
	"auxilia/domain/model"
	legacy "auxilia/handler/grpc"
	legacydb "auxilia/infrastructure/gorm"
	store "auxilia/infrastructure/gormv2"
	"auxilia/pb"
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Keep account wire compatibility without permitting the old profile endpoint
// to overwrite the ratings and counters now owned by the V2 settlement.
type UserGuard struct {
	*legacy.UserHandler
	Battle *Handler
}

func NewUserGuard(h *Handler) *UserGuard {
	return &UserGuard{UserHandler: legacy.NewUserHandler(legacydb.NewUserRepository(h.Store.DB)), Battle: h}
}
func (h *UserGuard) UpdateUser(ctx context.Context, r *pb.UpdateUserRequest) (*pb.UserResponse, error) {
	id, err := h.Battle.player(ctx)
	if err != nil {
		return nil, err
	}
	if r.Id != id {
		return nil, status.Error(codes.PermissionDenied, "id must match session")
	}
	req := proto.Clone(r).(*pb.UpdateUserRequest)
	req.NumWins = -1
	req.NumBattles = -1
	req.Rate = -1
	var result *pb.UserResponse
	err = h.Battle.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", id).Error; err != nil {
			return err
		}
		var updateErr error
		result, updateErr = legacy.NewUserHandler(legacydb.NewUserRepository(tx)).UpdateUser(ctx, req)
		return updateErr
	})
	if err != nil {
		if status.Code(err) != codes.Unknown {
			return nil, err
		}
		return nil, rpcError(err)
	}
	return result, nil
}
func (h *UserGuard) DeleteUser(ctx context.Context, r *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	id, err := h.Battle.player(ctx)
	if err != nil {
		return nil, err
	}
	if r.Id != id {
		return nil, status.Error(codes.PermissionDenied, "id must match session")
	}
	err = h.Battle.Store.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", id).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.Room{}).Where("user_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return store.ErrPrecondition
		}
		if err := tx.Where("player_id = ?", id).Delete(&store.Session{}).Error; err != nil {
			return err
		}
		return tx.Delete(&user).Error
	})
	if err != nil {
		return nil, rpcError(err)
	}
	return &pb.DeleteUserResponse{Success: true}, nil
}
