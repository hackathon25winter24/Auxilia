package handlergrpc

import (
	repository "auxilia/domain/interface"
	"auxilia/domain/model"
	"auxilia/pb"
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	roomStreams   = make(map[uint32][]pb.BattleService_StreamGameServer)
	roomStreamsMu sync.Mutex
)

type BattleHandler struct {
	pb.UnimplementedBattleServiceServer
	repo repository.BattleRepository
	timerMgr *TurnTimerManager
}

func NewBattleHandler(repo repository.BattleRepository) *BattleHandler {
	h := &BattleHandler{repo: repo}

	// 💡 補助関数（最新データを取得してpbレスポンスにする共通ロジック）
	getLatestResp := func(roomID uint32) *pb.GameDataResponse {
		gameData, _ := repo.GetGameDataByRoomID(roomID)
		if gameData != nil {
			return convertToResponse(gameData)
		}
		return nil
	}

	// 💡 タイマー制限時間を60秒としてマネージャーを初期化
	h.timerMgr = NewTurnTimerManager(repo, h.broadcastToGame, getLatestResp, 60*time.Second)

	return h
}

// CreateGame: 試合の初期登録
func (h *BattleHandler) CreateGame(ctx context.Context, req *pb.CreateGameRequest) (*pb.GameDataResponse, error) {
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

// モデルからgRPCレスポンスへの変換ルーチン
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
		P1RateDelta: int32(m.Player1RateDelta),
		P2RateDelta: int32(m.Player2RateDelta),
		P1Rate:      int32(m.Player1Rate),
		P2Rate:      int32(m.Player2Rate),
		IsTurnEnded: m.IsTurnEnded, // 💡 最新の .proto フィールドに対応
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
			RemainingTurn: int32(grid.RemainingTurn),
		})
	}

	// 💡 最新アクションにおける各キャラの被弾ダメージ情報を詰める（履歴演出用）
	// 必要に応じてリポジトリやモデル側からデータを引っ張る形に調整してください
	// 現状はプレースホルダーとして初期化のみ行っています
	res.AttackInfos = make([]*pb.AttackInfo, 0)

	// GameActionLog を変換して返す
	if m.CurrentAction.ID != 0 {
		var targetIDs []uint32
		if m.CurrentAction.TargetCharacterIDs != "" {
			parts := strings.Split(m.CurrentAction.TargetCharacterIDs, ",")
			for _, p := range parts {
				if id, err := strconv.ParseUint(strings.TrimSpace(p), 10, 32); err == nil {
					targetIDs = append(targetIDs, uint32(id))
				}
			}
		}
		res.GameActionLog = &pb.GameActionLog{
			Id:                       uint32(m.CurrentAction.ID),
			RoomId:                   uint32(m.CurrentAction.RoomID),
			Sequence:                 uint32(m.CurrentAction.Sequence),
			PlayerId:                 m.CurrentAction.PlayerID,
			ActionType:               m.CurrentAction.ActionType,
			ActorCharacterUniqueId:   uint32(m.CurrentAction.ActorCharacterUniqueID),
			ToX:                      uint32(m.CurrentAction.ToX),
			ToY:                      uint32(m.CurrentAction.ToY),
			AttackType:               int32(m.CurrentAction.AttackType),
			TargetCharacterUniqueIds: targetIDs,
		}
	}

	return res
}

func (h *BattleHandler) RegisterCharacters(ctx context.Context, req *pb.RegisterCharactersRequest) (*pb.RegisterCharactersResponse, error) {
	chars, err := h.repo.RegisterCharacters(req.RoomId, req.Is_1P, req.CharacterIds)
	if err != nil {
		return nil, err
	}

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

	// 初回接続時の情報送信
	gameData, err := h.repo.GetGameDataByRoomID(roomID)
	if err == nil {
		stream.Send(convertToResponse(gameData))
	}

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

func (h *BattleHandler) ApplyMove(ctx context.Context, req *pb.MoveAction) (*pb.AcceptResponse, error) {
	if req.CharacterId == 0 {
		return nil, status.Error(codes.InvalidArgument, "character_id is required")
	}

	err := h.repo.ApplyMove(req.RoomId, req.PlayerId, req.CharacterId, req.ToX, req.ToY)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to apply move: %v", err)
	}

	gameData, _ := h.repo.GetGameDataByRoomID(req.RoomId)
	if gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{Success: true, Message: "move applied successfully"}, nil
} 

func (h *BattleHandler) ApplyAttack(ctx context.Context, req *pb.AttackAction) (*pb.AcceptResponse, error) {
	if req.AttackerCharacterId == 0 {
		return nil, status.Error(codes.InvalidArgument, "attacker_character_id is required")
	}

	// AttackInfosをモデルに変換
	var attackInfos []model.AttackInfoData
	for _, info := range req.AttackInfos { // 💡 protoの変数名修正（attack_infos）に追従
		attackInfos = append(attackInfos, model.AttackInfoData{
			AttackedCharacterID: uint(info.AttackedCharacterId),
			NewHP:               uint(info.NewHp),
		})
	}

	err := h.repo.ApplyAttack(req.RoomId, req.PlayerId, req.AttackerCharacterId, req.AttackType, attackInfos)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to apply attack: %v", err)
	}

	gameData, _ := h.repo.GetGameDataByRoomID(req.RoomId)
	if gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{Success: true, Message: "attack applied successfully"}, nil
}

func (h *BattleHandler) EndTurn(ctx context.Context, req *pb.EndTurnRequest) (*pb.AcceptResponse, error) {
	err := h.repo.EndTurn(req.RoomId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to end turn: %v", err)
	}

	// 手動で終了されたので、走っているタイマーを安全にストップ
	h.timerMgr.StopTimer(req.RoomId)

	gameData, _ := h.repo.GetGameDataByRoomID(req.RoomId)
	if gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{Success: true, Message: "turn ended successfully"}, nil
}

func (h *BattleHandler) ApplyGridUpdate(ctx context.Context, req *pb.GridUpdateAction) (*pb.AcceptResponse, error) {
	var modelGrids []model.Grid
	for _, g := range req.Grids {
		modelGrids = append(modelGrids, model.Grid{
			PositionX:     uint(g.PositionX),
			PositionY:     uint(g.PositionY),
			GridType:      g.GridType,
			RemainingTurn: int32(g.RemainingTurn),
		})
	}

	err := h.repo.ApplyGridUpdate(req.RoomId, req.PlayerId, modelGrids)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to apply grid update: %v", err)
	}

	gameData, _ := h.repo.GetGameDataByRoomID(req.RoomId)
	if gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{Success: true, Message: "grid updated successfully"}, nil
} // 💡 閉じ括弧を修正

func (h *BattleHandler) FetchActionLog(ctx context.Context, req *pb.FetchActionLogRequest) (*pb.GameActionLog, error) {
	logData, err := h.repo.FetchActionLog(req.RoomId, req.Sequence) // 💡 組み込み変数「log」との衝突を回避するため変数名を logData に変更
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "action log not found: %v", err)
	}

	var targetIDs []uint32
	if logData.TargetCharacterIDs != "" {
		parts := strings.Split(logData.TargetCharacterIDs, ",")
		for _, p := range parts {
			if id, err := strconv.ParseUint(strings.TrimSpace(p), 10, 32); err == nil {
				targetIDs = append(targetIDs, uint32(id))
			}
		}
	}

	return &pb.GameActionLog{
		Id:                       uint32(logData.ID),
		RoomId:                   uint32(logData.RoomID),
		Sequence:                 uint32(logData.Sequence),
		PlayerId:                 logData.PlayerID,
		ActionType:               logData.ActionType,
		ActorCharacterUniqueId:   uint32(logData.ActorCharacterUniqueID),
		ToX:                      uint32(logData.ToX),
		ToY:                      uint32(logData.ToY),
		AttackType:               int32(logData.AttackType),
		TargetCharacterUniqueIds: targetIDs,
	}, nil
}

func (h *BattleHandler) ApplyEffect(ctx context.Context, req *pb.ApplyEffectRequest) (*pb.AcceptResponse, error) {
	err := h.repo.ApplyEffect(req.RoomId, req.PlayerId, req.CharacterId, req.EffectType, req.NewHp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to apply effect: %v", err)
	}

	gameData, _ := h.repo.GetGameDataByRoomID(req.RoomId)
	if gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{Success: true, Message: "effect applied successfully"}, nil
}

func (h *BattleHandler) NewTurn(ctx context.Context, req *pb.NewTurnRequest) (*pb.AcceptResponse, error) {
	err := h.repo.NewTurn(req.RoomId, req.PlayerId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to start new turn: %v", err)
	}

	// 💡 新しいターンが開幕した瞬間に、次の30秒タイマーを始動！
	h.timerMgr.StartTimer(req.RoomId)

	gameData, _ := h.repo.GetGameDataByRoomID(req.RoomId)
	if gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{Success: true, Message: "new turn started successfully"}, nil
}