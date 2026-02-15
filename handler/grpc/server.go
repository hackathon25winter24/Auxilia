package handlergrpc

import (
	"context"
	"fmt"
	"log"

	"auxilia/domain/model"
	"auxilia/pb"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Server struct {
    pb.UnimplementedUserServiceServer
    db *gorm.DB
}

func NewServer(db *gorm.DB) *Server {
    return &Server{db: db}
}

func (s *Server) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.UserResponse, error) {
    user := model.User{
        Hash:  req.Hash,
        Story: int(req.Story),
        Rate:  int(req.Rate),
    }

    if err := s.db.Create(&user).Error; err != nil {
        return nil, err
    }

    return &pb.UserResponse{
        Id:    user.ID.String(),
        Hash:  user.Hash,
        Story: int32(user.Story),
        Rate:  int32(user.Rate),
    }, nil
}

func (s *Server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.UserResponse, error) {
    uid, err := uuid.Parse(req.Id)
    if err != nil {
        return nil, err
    }

    var user model.User
    if err := s.db.First(&user, "id = ?", uid).Error; err != nil {
        return nil, err
    }

    return &pb.UserResponse{
        Id:    user.ID.String(),
        Hash:  user.Hash,
        Story: int32(user.Story),
        Rate:  int32(user.Rate),
    }, nil
}

func (s *Server) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
    var users []model.User
    if err := s.db.Find(&users).Error; err != nil {
        return nil, fmt.Errorf("failed to fetch users: %v", err)
    }

    var pbUsers []*pb.UserResponse
    for _, u := range users {
        pbUsers = append(pbUsers, &pb.UserResponse{
            Id:    u.ID.String(),
            Hash:  u.Hash,
            Story: int32(u.Story),
            Rate:  int32(u.Rate),
        })
    }

    return &pb.ListUsersResponse{Users: pbUsers}, nil
}

func (s *Server) Login(ctx context.Context, req *pb.LoginRequest) (*pb.UserResponse, error) {
    var user model.User
    if err := s.db.Where("hash = ?", req.Hash).First(&user).Error; err != nil {
        log.Printf("Login failed: user not found for hash %s", req.Hash)
        return nil, fmt.Errorf("invalid hash or user not found")
    }

    return &pb.UserResponse{
        Id:    user.ID.String(),
        Hash:  user.Hash,
        Story: int32(user.Story),
        Rate:  int32(user.Rate),
    }, nil
}
