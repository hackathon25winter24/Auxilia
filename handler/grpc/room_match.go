package handlergrpc

import (
	"context"

	repo "auxilia/domain/interface"
	"auxilia/domain/model"
	"auxilia/pb"

	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// gRPCのAPI実装クラス。Unity側で呼び出す関数が入っている。
type RoomMatchServer struct {
	pb.UnimplementedRoomMatchServiceServer
	repo repo.RoomMatchRepository
}

// サーバー生成関数
func NewRoomMatchServer(repo repo.RoomMatchRepository) *RoomMatchServer {
	return &RoomMatchServer{repo: repo}
}

// Unityから呼び出されるAPI。Unityからのリクエストをドメインモデルに変換しリポジトリを介してDBに保存する。レスポンスとしてDBで生成された部屋IDを返す。
func (s *RoomMatchServer) CreateRoomMatch(ctx context.Context, req *pb.CreateRoomMatchRequest) (*pb.RoomMatchResponse, error) {

	if utf8.RuneCountInString(req.RoomName) > 10 {
		return nil, status.Error(codes.InvalidArgument, "部屋名は10文字以内で入力してください")
	}

	room := &model.RoomMatch{
		RoomName:  req.RoomName,
		OwnerID:   req.OwnerId,
		IsPrivate: req.IsPrivate,
	}

	err := s.repo.CreateRoomMatch(room)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "部屋の作成に失敗しました: %v", err)
	}

	return &pb.RoomMatchResponse{
		Room: &pb.RoomMatch{
			RoomId:    int32(room.ID),
			RoomName:  room.RoomName,
			OwnerId:   room.OwnerID,
			IsPrivate: room.IsPrivate,
		},
	}, nil
}

func (s *RoomMatchServer) ListRoomMatch(ctx context.Context, req *pb.ListRoomMatchRequest) (*pb.ListRoomMatchResponse, error) {
	// 1. 全ての部屋を取得
	rooms, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "部屋一覧の取得に失敗しました")
	}

	// 2. pb.RoomMatch のスライスに変換
	var pbRooms []*pb.RoomMatch
	for _, r := range rooms {
		pbRooms = append(pbRooms, &pb.RoomMatch{
			RoomId:    int32(r.ID),
			RoomName:  r.RoomName,
			OwnerId:   r.OwnerID,
			IsPrivate: r.IsPrivate,
		})
	}

	return &pb.ListRoomMatchResponse{
		Rooms: pbRooms,
	}, nil
}
