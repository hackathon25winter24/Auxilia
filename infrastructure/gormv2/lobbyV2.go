package gormv2

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"strings"
	"time"

	game "auxilia/domain/gamev2"
	"auxilia/domain/model"
	legacy "auxilia/infrastructure/gorm"
	pb "auxilia/pb/v2"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Persisted revision shared by all server instances and all viewers.
type LobbyVersion struct {
	RoomID   uint32 `gorm:"primaryKey;autoIncrement:false"`
	Revision uint64
	Hash     string `gorm:"size:64"`
}

func (LobbyVersion) TableName() string { return "lobby_v2_versions" }

func validRoomName(name string) bool {
	return strings.TrimSpace(name) != "" && len(strings.TrimSpace(name)) <= 128
}

func (s *Store) CreateLobby(ctx context.Context, player, name string) (*pb.RoomSnapshot, error) {
	if !validRoomName(name) {
		return nil, game.ErrInvalidAction
	}
	var result *pb.RoomSnapshot
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		room := model.RoomMatch{RoomName: strings.TrimSpace(name), OwnerID: player}
		if err := tx.Create(&room).Error; err != nil {
			return err
		}
		member := model.Room{RoomID: int32(room.ID), UserID: player, State: 1}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}
		var err error
		result, err = lobbySnapshot(tx, room, player)
		return err
	})
	return result, err
}

// Every read locks the room first, so profile/membership/match reads form a single
// committed room view and cannot race a revision assignment on another server.
func (s *Store) lobby(ctx context.Context, id uint32, player string, fn func(*gorm.DB, *model.RoomMatch) error) (*pb.RoomSnapshot, error) {
	if id == 0 || id > math.MaxInt32 {
		return nil, game.ErrInvalidAction
	}
	var result *pb.RoomSnapshot
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var room model.RoomMatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&room, "id = ?", id).Error; err != nil {
			return err
		}
		if fn != nil {
			if err := fn(tx, &room); err != nil {
				return err
			}
		}
		var err error
		result, err = lobbySnapshot(tx, room, player)
		return err
	})
	return result, err
}

func (s *Store) GetLobby(ctx context.Context, id uint32, player string) (*pb.RoomSnapshot, error) {
	return s.lobby(ctx, id, player, func(tx *gorm.DB, room *model.RoomMatch) error { return lobbyMember(tx, id, player) })
}
func lobbyMember(tx *gorm.DB, id uint32, player string) error {
	var count int64
	if err := tx.Model(&model.Room{}).Where("room_id = ? AND user_id = ?", id, player).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrForbidden
	}
	return nil
}
func (s *Store) JoinLobby(ctx context.Context, id uint32, player string) (*pb.RoomSnapshot, error) {
	return s.lobby(ctx, id, player, func(tx *gorm.DB, room *model.RoomMatch) error {
		var members []model.Room
		if err := tx.Where("room_id = ?", id).Find(&members).Error; err != nil {
			return err
		}
		for _, m := range members {
			if m.UserID == player {
				return nil
			}
		}
		if room.IsGaming {
			return ErrPrecondition
		}
		if len(members) >= 8 {
			return ErrLobbyFull
		}
		occupied := map[int32]bool{}
		for _, m := range members {
			occupied[m.State] = true
		}
		var seat int32
		if !occupied[1] {
			seat = 1
		} else if !occupied[2] {
			seat = 2
		}
		if err := tx.Create(&model.Room{RoomID: int32(id), UserID: player, State: seat}).Error; err != nil {
			return err
		}
		return resetLobbyReady(tx, id)
	})
}

var ErrLobbyFull = errors.New("room or player seats full")

func resetLobbyReady(tx *gorm.DB, id uint32) error {
	return tx.Model(&model.Room{}).Where("room_id = ?", id).Update("is_ready", false).Error
}

func (s *Store) ApplyRoomCommand(ctx context.Context, player string, cmd *pb.RoomCommand) (*pb.RoomSnapshot, error) {
	return s.lobby(ctx, cmd.RoomId, player, func(tx *gorm.DB, room *model.RoomMatch) error {
		if err := lobbyMember(tx, cmd.RoomId, player); err != nil {
			return err
		}
		before, err := lobbySnapshot(tx, *room, player)
		if err != nil {
			return err
		}
		if cmd.ExpectedRevision == 0 || before.Revision != cmd.ExpectedRevision {
			return game.ErrStaleRevision
		}
		if before.Phase != pb.LobbyPhase_WAITING {
			return ErrPrecondition
		}
		repo := legacy.NewRoomRepository(tx)
		switch cmd.Action {
		case pb.RoomAction_ENTER_RING:
			if !before.CanEnterRing {
				return ErrPrecondition
			}
			if err := repo.EnterRing(int32(cmd.RoomId), player); err != nil {
				return err
			}
			return resetLobbyReady(tx, cmd.RoomId)
		case pb.RoomAction_LEAVE_RING:
			if !before.CanLeaveRing {
				return ErrPrecondition
			}
			if err := repo.LeaveRing(int32(cmd.RoomId), player); err != nil {
				return err
			}
			return resetLobbyReady(tx, cmd.RoomId)
		case pb.RoomAction_SET_READY:
			if !before.CanReady {
				return ErrPrecondition
			}
			return repo.SetReady(ctx, int32(cmd.RoomId), player, cmd.Ready)
		case pb.RoomAction_RENAME:
			if !before.CanRename {
				return ErrForbidden
			}
			if !validRoomName(cmd.RoomName) {
				return game.ErrInvalidAction
			}
			room.RoomName = strings.TrimSpace(cmd.RoomName)
			return tx.Save(room).Error
		case pb.RoomAction_START_MATCH:
			if room.OwnerID != player {
				return ErrForbidden
			}
			if !before.CanStart {
				return ErrPrecondition
			}
			if _, err := New(tx).Create(ctx, cmd.RoomId, player); err != nil {
				return err
			}
			room.IsGaming = true
			return nil
		case pb.RoomAction_LEAVE_ROOM:
			if err := repo.LeaveRoom(int32(cmd.RoomId), player); err != nil {
				return err
			}
			if err := resetLobbyReady(tx, cmd.RoomId); err != nil {
				return err
			}
			err := tx.First(room, "id = ?", cmd.RoomId).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				room.OwnerID = ""
				return nil
			}
			return err
		default:
			return game.ErrInvalidAction
		}
	})
}

func lobbySnapshot(tx *gorm.DB, room model.RoomMatch, viewer string) (*pb.RoomSnapshot, error) {
	out := &pb.RoomSnapshot{RoomId: uint32(room.ID), RoomName: room.RoomName, OwnerId: room.OwnerID, Phase: pb.LobbyPhase_WAITING}
	var members []model.Room
	if err := tx.Where("room_id = ?", room.ID).Order("joined_at ASC, user_id ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	for _, m := range members {
		var user model.User
		if err := tx.First(&user, "id = ?", m.UserID).Error; err != nil {
			return nil, err
		}
		out.Members = append(out.Members, &pb.LobbyMember{UserId: m.UserID, Name: user.Name, Rate: int32(user.Rate), Seat: m.State, Ready: m.IsReady, JoinedAt: m.JoinedAt.UTC().Format(time.RFC3339Nano)})
	}
	if len(members) == 0 {
		out.Phase = pb.LobbyPhase_CLOSED
		out.OwnerId = ""
	}
	if room.IsGaming {
		var match Match
		if err := tx.Where("room_id = ? AND finished = ?", room.ID, false).First(&match).Error; err != nil {
			return nil, err
		}
		out.MatchId = match.ID
		out.Phase = pb.LobbyPhase_SELECTING
		if match.Started {
			out.Phase = pb.LobbyPhase_BATTLING
		}
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(out)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(raw)
	digest := hex.EncodeToString(hash[:])
	version := LobbyVersion{RoomID: out.RoomId}
	err = tx.First(&version, "room_id = ?", out.RoomId).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if version.Hash != digest {
		version.Revision++
		version.Hash = digest
		if err := tx.Save(&version).Error; err != nil {
			return nil, err
		}
	}
	out.Revision = version.Revision
	waiting := out.Phase == pb.LobbyPhase_WAITING
	var self *pb.LobbyMember
	occupied := map[int32]bool{}
	ready := map[int32]bool{}
	for _, m := range out.Members {
		occupied[m.Seat] = true
		ready[m.Seat] = m.Ready
		if m.UserId == viewer {
			self = m
		}
	}
	out.CanRename = waiting && self != nil && out.OwnerId == viewer
	out.CanReady = waiting && self != nil && self.Seat != 0
	out.CanLeaveRing = out.CanReady
	out.CanEnterRing = waiting && self != nil && self.Seat == 0 && (!occupied[1] || !occupied[2])
	out.CanLeaveRoom = waiting && self != nil
	switch {
	case !waiting:
		out.StartBlockReason = "NOT_WAITING"
	case self == nil:
		out.StartBlockReason = "NOT_MEMBER"
	case out.OwnerId != viewer:
		out.StartBlockReason = "NOT_OWNER"
	case !occupied[1] || !occupied[2]:
		out.StartBlockReason = "SEATS_NOT_FILLED"
	case !ready[1] || !ready[2]:
		out.StartBlockReason = "NOT_READY"
	default:
		out.CanStart = true
	}
	return out, nil
}
