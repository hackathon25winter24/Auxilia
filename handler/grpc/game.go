package handlergrpc

import (
	"auxilia/domain"
	repository "auxilia/domain/interface"
	"auxilia/domain/model" // プロジェクト構造に合わせて調整してください
	"auxilia/pb"
	"context"
	"errors"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type BattleHandler struct {
	pb.UnimplementedBattleServiceServer
	// handler はインターフェース型に依存するようにして
	// テストや実装の差し替えを容易にします。
	repo repository.BattleRepository
}

func NewBattleHandler(repo repository.BattleRepository) *BattleHandler {
	return &BattleHandler{repo: repo}
}

// CreateGame: 試合の初期登録（ハンドラー層）
func (h *BattleHandler) CreateGame(ctx context.Context, req *pb.CreateGameRequest) (*pb.GameDataResponse, error) {
	// リポジトリ層にDB保存を依頼
	gameData, err := h.repo.CreateGame(req.RoomId, req.Player1Id, req.Player2Id)
	if err != nil {
		return nil, err
	}

	return convertToResponse(gameData), nil
}

// GetGameData: 試合情報の取得
func (h *BattleHandler) GetGameData(ctx context.Context, req *pb.GetGameDataRequest) (*pb.GameDataResponse, error) {
	gameData, err := h.repo.GetGameDataByRoomID(req.RoomId)
	if err != nil {
		return nil, err
	}

	return convertToResponse(gameData), nil
}

// モデルからgRPCレスポンスへの変換ルーチン（ハンドラー内で共通利用）
func convertToResponse(m *model.GameData) *pb.GameDataResponse {
	res := &pb.GameDataResponse{
		Id:          uint32(m.ID),
		RoomId:      uint32(m.RoomID),
		Player1Id:   m.Player1ID,
		Player2Id:   m.Player2ID,
		BaseHp1:     uint32(m.BaseHP1),
		BaseHp2:     uint32(m.BaseHP2),
		Turn:        uint32(m.Turn),
		Is_1PTurn:   m.Is1PTurn,
		TurnStartAt: timestamppb.New(m.TurnStartAt),
		IsFinished:  m.IsFinished,
	}

	if m.WinnerPlayerID != nil {
		res.WinnerPlayerId = *m.WinnerPlayerID
	}
	if m.FinishedAt != nil {
		res.FinishedAt = timestamppb.New(*m.FinishedAt)
	}

	for _, c := range m.Characters {
		char := &pb.UniqueCharacter{
			Id:          uint32(c.ID),
			CharacterId: uint32(c.CharacterID),
			Is_1P:       c.Is1P,
			Hp:          uint32(c.HP),
			PositionX:   uint32(c.PositionX),
			PositionY:   uint32(c.PositionY),
			IsSelected:  c.IsSelected,
		}
		for _, cond := range c.Conditions {
			char.Conditions = append(char.Conditions, &pb.CharacterCondition{
				Id:          uint32(cond.ID),
				ConditionId: int32(cond.ConditionID),
				LastingTurn: int32(cond.LastingTurn),
			})
		}
		res.Characters = append(res.Characters, char)
	}
	return res
}

func (h *BattleHandler) RegisterCharacters(ctx context.Context, req *pb.RegisterCharactersRequest) (*pb.RegisterCharactersResponse, error) {
	chars, err := h.repo.RegisterCharacters(req.RoomId, req.Is_1P, req.CharacterIds)
	if err != nil {
		return nil, err
	}

	// レスポンス用に変換
	var pbChars []*pb.UniqueCharacter
	for _, c := range chars {
		pbChars = append(pbChars, &pb.UniqueCharacter{
			Id:          uint32(c.ID),
			CharacterId: uint32(c.CharacterID),
			Is_1P:       c.Is1P,
			Hp:          uint32(c.HP),
			PositionX:   uint32(c.PositionX),
			PositionY:   uint32(c.PositionY),
			IsSelected:  c.IsSelected,
		})
	}

	return &pb.RegisterCharactersResponse{
		RegisteredCharacters: pbChars,
	}, nil
}

func (h *BattleHandler) StreamGame(stream pb.BattleService_StreamGameServer) error {
	for {
		action, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		switch a := action.GetAction().(type) {
		case *pb.PlayerAction_Move:
			if a.Move == nil {
				continue
			}

			gameData, err := h.repo.ApplyMove(action.RoomId, action.PlayerId, a.Move.CharacterUniqueId, a.Move.ToX, a.Move.ToY)
			if err != nil {
				if errors.Is(err, domain.ErrGameNotFound) || errors.Is(err, domain.ErrCharacterNotFound) {
					return status.Errorf(codes.NotFound, "game or character not found in room ID %d", action.RoomId)
				}
				if errors.Is(err, domain.ErrInvalidTurn) || errors.Is(err, domain.ErrForbiddenAction) {
					return status.Errorf(codes.FailedPrecondition, "invalid move: %v", err)
				}
				return status.Errorf(codes.Internal, "failed to apply move: %v", err)
			}

			if err := stream.Send(convertToResponse(gameData)); err != nil {
				return err
			}
		case *pb.PlayerAction_Attack:
			if a.Attack == nil {
				continue
			}

			gameData, err := h.repo.ApplyAttack(action.RoomId, action.PlayerId, a.Attack.AttackerCharacterUniqueId, a.Attack.AttackType, a.Attack.IsStarted, a.Attack.BaseHp1, a.Attack.BaseHp2, a.Attack.AttackedCharacterUniqueId, a.Attack.NewHp)
			if err != nil {
				if errors.Is(err, domain.ErrGameNotFound) || errors.Is(err, domain.ErrCharacterNotFound) {
					return status.Errorf(codes.NotFound, "game or character not found in room ID %d", action.RoomId)
				}
				if errors.Is(err, domain.ErrInvalidTurn) || errors.Is(err, domain.ErrForbiddenAction) {
					return status.Errorf(codes.FailedPrecondition, "invalid attack: %v", err)
				}
				return status.Errorf(codes.Internal, "failed to apply attack: %v", err)
			}

			if err := stream.Send(convertToResponse(gameData)); err != nil {
				return err
			}
		case *pb.PlayerAction_EndTurn:
			if !a.EndTurn {
				continue
			}

			gameData, err := h.repo.EndTurn(action.RoomId)
			if err != nil {
				if errors.Is(err, domain.ErrGameNotFound) {
					return status.Errorf(codes.NotFound, "game with room ID %d not found", action.RoomId)
				}
				return status.Errorf(codes.Internal, "failed to end turn: %v", err)
			}

			if err := stream.Send(convertToResponse(gameData)); err != nil {
				return err
			}
		default:
			gameData, err := h.repo.GetGameDataByRoomID(action.RoomId)
			if err != nil {
				if errors.Is(err, domain.ErrGameNotFound) {
					return status.Errorf(codes.NotFound, "game with room ID %d not found", action.RoomId)
				}
				return status.Errorf(codes.Internal, "failed to fetch game data: %v", err)
			}

			if err := stream.Send(convertToResponse(gameData)); err != nil {
				return err
			}
		}
	}
}
