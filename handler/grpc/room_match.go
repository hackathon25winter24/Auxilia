package grpc

import (
	"context"

	"auxilia/domain/model"
	"auxilia/pb"
)

// CreateRoomMatchを持つ型をRoomMatchRepositoryとして扱う
type RoomMatchRepository interface {
	CreateRoomMatch(room *model.RoomMatch) error
}

// gRPCのAPI実装クラス。Unity側で呼び出す関数が入っている。
type RoomMatchServer struct {
	pb.UnimplementedRoomMatchServiceServer
	repo RoomMatchRepository
}

// サーバー生成関数
func NewRoomMatchServer(repo RoomMatchRepository) *RoomMatchServer {
	return &RoomMatchServer{repo: repo}
}

// Unityから呼び出されるAPI。Unityからのリクエストをドメインモデルに変換しリポジトリを介してDBに保存する。レスポンスとしてDBで生成された部屋IDを返す。
func (s *RoomMatchServer) CreateRoomMatch(ctx context.Context, req *pb.CreateRoomMatchRequest) (*pb.CreateRoomMatchResponse, error) {

	room := &model.RoomMatch{
		Name: 	   req.Name,
		Owner:     req.Owner,
		IsPrivate: req.IsPrivate,
	}

	err := s.repo.CreateRoomMatch(room)
	if err != nil {
		return nil, err
	}

	return &pb.CreateRoomMatchResponse{
		RoomId: int32(room.ID),
	}, nil
}