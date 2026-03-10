package handlergrpc

import (
	"context"
	"log"

	repository "auxilia/domain/interface"
	"auxilia/domain/model"
	"auxilia/pb"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)


type GameHandler struct {
    pb.UnimplementedGameServiceServer
    repo repository.GameRepository
}

func NewGameHandler(repo repository.GameRepository) *GameHandler {
    return &GameHandler{repo: repo}
}

// SaveGame: 試合状況を新規保存する
func (h *GameHandler) SaveGame(ctx context.Context, req *pb.SaveGameRequest) (*pb.GameResponse, error) {
    game := req.GetGame()
    if game == nil {
        return nil, status.Error(codes.InvalidArgument, "game field is required")
    }
    log.Printf("SaveGame requested: RoomID=%d", game.GetRoomId())

    modelGame := h.gameFromProto(game)
    if err := h.repo.SaveGame(ctx, modelGame); err != nil {
        log.Printf("Failed to save game: %v", err)
        return nil, status.Errorf(codes.Internal, "failed to save game data")
    }

    return &pb.GameResponse{Success: true, Game: game}, nil
}

// GetGame: RoomIDを指定して特定の試合データを取得する
func (h *GameHandler) GetGame(ctx context.Context, req *pb.GetGameRequest) (*pb.GameResponse, error) {
    log.Printf("GetGame requested: RoomID=%d", req.GetRoomId())

    game, err := h.repo.GetByRoomID(ctx, uint(req.GetRoomId()))
    if err != nil {
        return nil, status.Errorf(codes.NotFound, "game with RoomID %d not found", req.GetRoomId())
    }
    return &pb.GameResponse{Success: true, Game: h.protoFromModel(game)}, nil
}

// UpdateGame: 既存のRoomIDのデータを更新する
func (h *GameHandler) UpdateGame(ctx context.Context, req *pb.UpdateGameRequest) (*pb.GameResponse, error) {
    game := req.GetGame()
    if game == nil {
        return nil, status.Error(codes.InvalidArgument, "game field is required")
    }
    log.Printf("UpdateGame requested: RoomID=%d", game.GetRoomId())

    modelGame := h.gameFromProto(game)
    if err := h.repo.UpdateGame(ctx, modelGame); err != nil {
        log.Printf("Failed to update game: %v", err)
        return nil, status.Errorf(codes.Internal, "failed to update game data")
    }
    return &pb.GameResponse{Success: true, Game: game}, nil
}

// DeleteGame: 試合終了時などにデータを削除する
func (h *GameHandler) DeleteGame(ctx context.Context, req *pb.DeleteGameRequest) (*pb.DeleteGameResponse, error) {
    log.Printf("DeleteGame requested: RoomID=%d", req.GetRoomId())

    if err := h.repo.DeleteGame(ctx, uint(req.GetRoomId())); err != nil {
        log.Printf("Failed to delete game: %v", err)
        return nil, status.Errorf(codes.Internal, "failed to delete game data")
    }

    return &pb.DeleteGameResponse{Success: true}, nil
}

func (h *GameHandler) ListGames(ctx context.Context, req *pb.ListGamesRequest) (*pb.ListGamesResponse, error) {
	games, err := h.repo.ListGames(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list games")
	}

	var pbGames []*pb.Game
	for _, g := range games {
		pbGames = append(pbGames, h.protoFromModel(&g))
	}

	return &pb.ListGamesResponse{Games: pbGames}, nil
}

// --- 変換用ヘルパーメソッド ---

// gameFromProto converts a protobuf Game message into a domain model.
func (h *GameHandler) gameFromProto(g *pb.Game) *model.GameData {
	gameData := &model.GameData{
		RoomID:    uint(g.GetRoomId()),
		Player1ID: g.GetPlayer_1PId(),
		Player2ID: g.GetPlayer_2PId(),
		BaseHP1:   uint(g.GetBaseHp_1P()),
		BaseHP2:   uint(g.GetBaseHp_2P()),
		Turn:      uint(g.GetTurn()),
	}

	for _, c := range g.GetCharacters() {
		char := model.UniqueCharacter{
			RoomID:      uint(g.GetRoomId()),
			Is1P:        c.GetIs_1P(),
			CharacterID: uint(c.GetCharacterId()),
			HP:          uint(c.GetHp()),
			PositionX:   uint(c.GetX()),
			PositionY:   uint(c.GetY()),
		}

		for _, cond := range c.GetConditions() {
			char.Conditions = append(char.Conditions, model.CharacterCondition{
				ConditionID: int(cond.GetConditionId()),
				LastingTurn: int(cond.GetLastingTurn()),
			})
		}
		gameData.Characters = append(gameData.Characters, char)
	}
	return gameData
}

// protoFromModel converts a domain model into a protobuf Game message.
func (h *GameHandler) protoFromModel(g *model.GameData) *pb.Game {
	var pbChars []*pb.UniqueCharacter
	for _, c := range g.Characters {
		var pbConds []*pb.CharacterCondition
		for _, cond := range c.Conditions {
			pbConds = append(pbConds, &pb.CharacterCondition{
				ConditionId: uint32(cond.ConditionID),
				LastingTurn: uint32(cond.LastingTurn),
			})
		}

		pbChars = append(pbChars, &pb.UniqueCharacter{
			CharacterId: uint32(c.CharacterID),
			Hp:          int32(c.HP),
			X:           int32(c.PositionX),
			Y:           int32(c.PositionY),
			Is_1P:       c.Is1P,
			Conditions:  pbConds,
		})
	}

	return &pb.Game{
		RoomId:      uint32(g.RoomID),
		Player_1PId:  g.Player1ID,
		Player_2PId:  g.Player2ID,
		BaseHp_1P:    int32(g.BaseHP1),
		BaseHp_2P:    int32(g.BaseHP2),
		Turn:        uint32(g.Turn),
		Characters:  pbChars,
	}
}

