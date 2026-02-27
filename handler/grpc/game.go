package handlergrpc

import (
	"context"
	"log"

	"auxilia/domain/model"
	"auxilia/infrastructure/gorm"
	"auxilia/pb"
)

type GameHandler struct {
	pb.UnimplementedGameServiceServer
	repo gorm.GameRepository
}

func NewGameHandler(repo gorm.GameRepository) *GameHandler {
	return &GameHandler{
		repo: repo,
	}
}

// SaveGame: Unityからの試合状況をDBに保存する
func (h *GameHandler) SaveGame(ctx context.Context, req *pb.SaveGameRequest) (*pb.SaveGameResponse, error) {
	log.Printf("SaveGame requested: RoomID=%d, Turn=%d", req.RoomId, req.Turn)

	// 1. 親となる GameData の作成
	gameData := &model.GameData{
		RoomID:    uint(req.RoomId),
		Player1ID: req.Player_1PId,
		Player2ID: req.Player_2PId,
		BaseHP1:   uint(req.BaseHp_1P),
		BaseHP2:   uint(req.BaseHp_2P),
		Turn:      uint(req.Turn),
	}

	// 2. Characters スライスの詰め替え
	for _, c := range req.Characters {
		uniqueChar := model.UniqueCharacter{
			RoomID:      uint(req.RoomId),
			Is1P:        c.Is_1P,
			CharacterID: uint(c.CharacterId),
			HP:          uint(c.Hp),
			PositionX:   uint(c.X),
			PositionY:   uint(c.Y),
		}

		// 3. 各キャラの状態異常 (Conditions) の詰め替え
		for _, cond := range c.Conditions {
			uniqueChar.Conditions = append(uniqueChar.Conditions, model.CharacterCondition{
				ConditionID: int(cond.ConditionId),
				LastingTurn: int(cond.LastingTurn),
			})
		}

		gameData.Characters = append(gameData.Characters, uniqueChar)
	}

	// リポジトリを呼んで保存を実行
	if err := h.repo.SaveGame(ctx, gameData); err != nil {
		log.Printf("Failed to save game data: %v", err)
		return nil, err
	}

	return &pb.SaveGameResponse{Success: true}, nil
}

// DeleteGame: 試合終了時などにデータを削除する
func (h *GameHandler) DeleteGame(ctx context.Context, req *pb.DeleteGameRequest) (*pb.DeleteGameResponse, error) {
	log.Printf("DeleteGame requested: RoomID=%d", req.RoomId)

	if err := h.repo.DeleteGame(ctx, uint(req.RoomId)); err != nil {
		log.Printf("Failed to delete game data: %v", err)
		return nil, err
	}

	return &pb.DeleteGameResponse{Success: true}, nil
}

func (h *GameHandler) ListGames(ctx context.Context, req *pb.ListGamesRequest) (*pb.ListGamesResponse, error) {
	games, err := h.repo.ListGames(ctx)
	if err != nil {
		return nil, err
	}

	var pbGames []*pb.SaveGameRequest
	for _, g := range games {
		// キャラクターの詰め替え
		var pbChars []*pb.UniqueCharacter
		for _, c := range g.Characters {
			// 状態異常の詰め替え
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

		pbGames = append(pbGames, &pb.SaveGameRequest{
			RoomId:      uint32(g.RoomID),
			Player_1PId: g.Player1ID,
			Player_2PId: g.Player2ID,
			BaseHp_1P:   int32(g.BaseHP1),
			BaseHp_2P:   int32(g.BaseHP2),
			Turn:        uint32(g.Turn),
			Characters:  pbChars,
		})
	}

	return &pb.ListGamesResponse{Games: pbGames}, nil
}