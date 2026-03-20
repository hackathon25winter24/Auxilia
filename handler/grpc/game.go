package handlergrpc

import (
	repository "auxilia/domain/interface"
	"auxilia/domain/model" // プロジェクト構造に合わせて調整してください
	"auxilia/pb"
	"context"
	"log"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	roomStreams   = make(map[uint32][]pb.BattleService_StreamGameServer)
	roomStreamsMu sync.Mutex
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
		Cost_1P:     uint32(m.Cost1P),
		Cost_2P:     uint32(m.Cost2P),
		Turn:        uint32(m.Turn),
		Is_1PTurn:   m.Is1PTurn,
		TurnStartAt: timestamppb.New(m.TurnStartAt),
		IsFinished:  m.IsFinished,
		P1RateDelta:  int32(m.Player1RateDelta),
		P2RateDelta:  int32(m.Player2RateDelta),
		P1Rate:       int32(m.Player1Rate),
		P2Rate:       int32(m.Player2Rate),
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

	for _, grid := range m.Grids {
		res.Grids = append(res.Grids, &pb.GridInfo{
			PositionX:     uint32(grid.PositionX),
			PositionY:     uint32(grid.PositionY),
			GridType:      grid.GridType,
			IsSelected:    grid.IsSelected,
			IsAttackRange: grid.IsAttackRange,
		})
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

func (h *BattleHandler) StreamGame(req *pb.StreamGameRequest, stream pb.BattleService_StreamGameServer) error {
	roomID := req.RoomId

	roomStreamsMu.Lock()
	roomStreams[roomID] = append(roomStreams[roomID], stream)
	roomStreamsMu.Unlock()

	defer func() {
		roomStreamsMu.Lock()
		streams := roomStreams[roomID]
		for i, s := range streams {
			if s == stream {
				roomStreams[roomID] = append(streams[:i], streams[i+1:]...)
				break
			}
		}
		roomStreamsMu.Unlock()
	}()

	// 初回接続時の情報送信（現在のステータスを即座に返す）
	gameData, err := h.repo.GetGameDataByRoomID(roomID)
	if err == nil {
		stream.Send(convertToResponse(gameData))
	}

	// クライアントから切断されるまで待機
	<-stream.Context().Done()
	return nil
}

func (h *BattleHandler) broadcastToGame(roomID uint32, response *pb.GameDataResponse) {
	roomStreamsMu.Lock()
	streams := roomStreams[roomID]
	roomStreamsMu.Unlock()

	for _, s := range streams {
		if err := s.Send(response); err != nil {
			log.Printf("[broadcastToGame] Error sending to stream: %v", err)
		}
	}
}

func (h *BattleHandler) ApplyMove(ctx context.Context, req *pb.PlayerAction) (*pb.GameDataResponse, error) {
	move := req.GetMove()
	if move == nil {
		return nil, status.Error(codes.InvalidArgument, "move is required")
	}

	gameData, err := h.repo.ApplyMove(req.RoomId, req.PlayerId, move.CharacterUniqueId, move.ToX, move.ToY)
	if err != nil {
		return nil, err
	}

	resp := convertToResponse(gameData)
	h.broadcastToGame(req.RoomId, resp)
	return resp, nil
}

func (h *BattleHandler) ApplyAttack(ctx context.Context, req *pb.PlayerAction) (*pb.GameDataResponse, error) {
	attack := req.GetAttack()
	if attack == nil {
		return nil, status.Error(codes.InvalidArgument, "attack is required")
	}

	gameData, attackInfo, err := h.repo.ApplyAttack(req.RoomId, req.PlayerId, attack.AttackerCharacterUniqueId, attack.AttackType, attack.IsStarted, attack.BaseHp1, attack.BaseHp2, attack.AttackedCharacterUniqueId, attack.NewHp)
	if err != nil {
		return nil, err
	}

	resp := convertToResponse(gameData)
	if attackInfo != nil {
		resp.AttackInfos = []*pb.AttackInfo{
			{
				Id:                  uint32(attackInfo.ID),
				RoomId:              uint32(attackInfo.RoomID),
				AttackerSide:        attackInfo.AttackerSide,
				IsStarted:           attackInfo.IsStarted,
				AttackerCharacterId: uint32(*attackInfo.AttackerCharacterID),
				AttackType:          attackInfo.AttackType,
				AttackedAt:          timestamppb.New(attackInfo.AttackedAt),
			},
		}
	}

	h.broadcastToGame(req.RoomId, resp)
	return resp, nil
}

func (h *BattleHandler) EndTurn(ctx context.Context, req *pb.PlayerAction) (*pb.GameDataResponse, error) {
	if !req.GetEndTurn() {
		return nil, status.Error(codes.InvalidArgument, "end_turn must be true")
	}

	gameData, err := h.repo.EndTurn(req.RoomId)
	if err != nil {
		return nil, err
	}

	resp := convertToResponse(gameData)
	h.broadcastToGame(req.RoomId, resp)
	return resp, nil
}

func (h *BattleHandler) ApplyGridUpdate(ctx context.Context, req *pb.PlayerAction) (*pb.GameDataResponse, error) {
	gridUpdate := req.GetGridUpdate()
	if gridUpdate == nil {
		return nil, status.Error(codes.InvalidArgument, "grid_update is required")
	}

	var modelGrids []model.Grid
	for _, g := range gridUpdate.Grids {
		modelGrids = append(modelGrids, model.Grid{
			PositionX:     uint(g.PositionX),
			PositionY:     uint(g.PositionY),
			GridType:      g.GridType,
			IsSelected:    g.IsSelected,
			IsAttackRange: g.IsAttackRange,
		})
	}

	gameData, err := h.repo.ApplyGridUpdate(req.RoomId, req.PlayerId, modelGrids)
	if err != nil {
		return nil, err
	}

	resp := convertToResponse(gameData)
	h.broadcastToGame(req.RoomId, resp)
	return resp, nil
}
