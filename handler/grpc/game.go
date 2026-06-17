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
	"fmt"

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

	getLatestResp := func(roomID uint32) *pb.GameDataResponse {
		ctx := context.Background()
		
		gameData, _ := repo.GetGameDataByRoomID(ctx, roomID)
		if gameData != nil {
			return convertToResponse(gameData)
		}
		return nil
	}

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
	gameData, err := h.repo.GetGameDataByRoomID(ctx, req.RoomId)
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
		IsTurnEnded: m.IsTurnEnded,
	}

	if m.WinnerPlayerID != nil {
		res.WinnerPlayerId = *m.WinnerPlayerID
	}
	if m.FinishedAt != nil {
		res.FinishedAt = timestamppb.New(*m.FinishedAt)
	}

	// キャラクター情報の詰め替え
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

	// 💡 グリッド情報の詰め替え（キャラクターの滞在判定は含めず、純粋な地形・デバフ情報のみ）
	for _, grid := range m.Grids {
		res.Grids = append(res.Grids, &pb.GridInfo{
			PositionX:  uint32(grid.PositionX),
			PositionY:  uint32(grid.PositionY),
			GridType:   grid.GridType,   // 変更のあった地形型
			DebuffType: grid.DebuffType, // 配置されたトラップやデバフ型
			// ※ IsCharacterStay は proto 定義から除外、またはフロント側で無視されるためここでの処理は不要です
		})
	}

	return res
}

func (h *BattleHandler) RegisterCharacters(ctx context.Context, req *pb.RegisterCharactersRequest) (*pb.RegisterCharactersResponse, error) {
	chars, err := h.repo.RegisterCharacters(ctx,req.RoomId, req.Is_1P, req.CharacterIds)
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

	// stream.Context() から context を取得してリポジトリに渡す
	ctx := stream.Context()

	// 初回接続時の情報送信
	gameData, err := h.repo.GetGameDataByRoomID(ctx, roomID)
	if err == nil {
		stream.Send(convertToResponse(gameData))
	}

	// 💡 stream.Context().Done() がチャネルを閉じるのを待つ
	<-ctx.Done()
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

	err := h.repo.ApplyMove(ctx, req.RoomId, req.PlayerId, req.CharacterId, req.ToX, req.ToY)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to apply move: %v", err)
	}

	gameData, _ := h.repo.GetGameDataByRoomID(ctx, req.RoomId)
	if gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{Success: true, Message: "move applied successfully"}, nil
} // 💡 閉じ括弧を修正

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

	err := h.repo.ApplyAttack(ctx, req.RoomId, req.PlayerId, req.AttackerCharacterId, req.AttackType, attackInfos)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to apply attack: %v", err)
	}

	gameData, _ := h.repo.GetGameDataByRoomID(ctx, req.RoomId)
	if gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{Success: true, Message: "attack applied successfully"}, nil
}

func (h *BattleHandler) EndTurn(ctx context.Context, req *pb.EndTurnRequest) (*pb.AcceptResponse, error) {
	err := h.repo.EndTurn(ctx, req.RoomId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to end turn: %v", err)
	}

	// 手動で終了されたので、走っているタイマーを安全にストップ
	h.timerMgr.StopTimer(req.RoomId)

	gameData, _ := h.repo.GetGameDataByRoomID(ctx, req.RoomId)
	if gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{Success: true, Message: "turn ended successfully"}, nil
}

func (h *BattleHandler) ApplyGridUpdate(ctx context.Context, req *pb.GridUpdateAction) (*pb.AcceptResponse, error) {
	// 1. バリデーション: 変更データが空の場合は何もしない
	if len(req.Grids) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "grids list cannot be empty")
	}

	// 2. proto のスライスをモデル（GORM）のスライスに変換
	var modelGrids []model.Grid
	for _, g := range req.Grids {
		modelGrids = append(modelGrids, model.Grid{
			RoomID:     req.RoomId,         // GORMの複合キー用に room_id を各マスにセット
			PositionX:  uint32(g.PositionX), // 必要に応じて uint にキャストしてください
			PositionY:  uint32(g.PositionY),
			GridType:   g.GridType,
			DebuffType: g.DebuffType,
		})
	}

	// 3. 💡 リポジトリの一括更新処理を呼び出す
	// 引数にスライス（modelGrids）を渡せるようにします
	err := h.repo.ApplyGridUpdate(ctx, req.RoomId, modelGrids)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to apply grid update: %v", err)
	}

	// 4. 最新のゲームデータを取得して、ストリーム中の全プレイヤーにブロードキャスト
	gameData, err := h.repo.GetGameDataByRoomID(ctx, req.RoomId)
	if err == nil && gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{
		Success: true, 
		Message: fmt.Sprintf("%d grids updated successfully", len(modelGrids)),
	}, nil
}

func (h *BattleHandler) FetchActionLog(ctx context.Context, req *pb.FetchActionLogRequest) (*pb.GameActionLog, error) {
	logData, err := h.repo.FetchActionLog(ctx, req.RoomId, req.Sequence)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "action log not found: %v", err)
	}

	// 1. 共通部分のベースを組み立てる
	logResp := &pb.GameActionLog{
		Id:                     uint32(logData.ID),
		RoomId:                 uint32(logData.RoomID),
		Sequence:               uint32(logData.Sequence),
		PlayerId:               logData.PlayerID,
		ActorCharacterUniqueId: uint32(logData.ActorCharacterUniqueID),
	}

	// 2. 文字列の action_type を proto の Enum 型にマッピング
	switch logData.ActionType {
	case "MOVE":
		logResp.ActionType = pb.ActionType_MOVE

		// 💡 oneof への代入処理 (MoveDetail 構造体を作ってラップして入れる)
		logResp.Detail = &pb.GameActionLog_MoveDetail{
			MoveDetail: &pb.MoveDetail{
				ToX: uint32(logData.ToX), // ⚠️ もしここでエラーが出る場合は「To_X」にしてください
				ToY: uint32(logData.ToY), // ⚠️ もしここでエラーが出る場合は「To_Y」にしてください
			},
		}

	case "ATTACK":
		logResp.ActionType = pb.ActionType_ATTACK

		// カンマ区切りの文字列を []uint32 に変換
		var targetIDs []uint32
		if logData.TargetCharacterIDs != "" {
			parts := strings.Split(logData.TargetCharacterIDs, ",")
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					if id, err := strconv.ParseUint(trimmed, 10, 32); err == nil {
						targetIDs = append(targetIDs, uint32(id))
					}
				}
			}
		}

		// 💡 oneof への代入処理 (AttackDetail 構造体を作ってラップして入れる)
		logResp.Detail = &pb.GameActionLog_AttackDetail{
			AttackDetail: &pb.AttackDetail{
				AttackType:               int32(logData.AttackType),
				TargetCharacterUniqueIds: targetIDs,
			},
		}

	default:
		logResp.ActionType = pb.ActionType_ACTION_TYPE_UNKNOWN
	}

	return logResp, nil
}

func (h *BattleHandler) ApplyEffect(ctx context.Context, req *pb.ApplyEffectRequest) (*pb.AcceptResponse, error) {
	err := h.repo.ApplyEffect(ctx, req.RoomId, req.PlayerId, req.CharacterId, req.EffectType, req.NewHp)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to apply effect: %v", err)
	}

	gameData, _ := h.repo.GetGameDataByRoomID(ctx, req.RoomId)
	if gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{Success: true, Message: "effect applied successfully"}, nil
}

func (h *BattleHandler) NewTurn(ctx context.Context, req *pb.NewTurnRequest) (*pb.AcceptResponse, error) {
	err := h.repo.NewTurn(ctx, req.RoomId, req.PlayerId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to start new turn: %v", err)
	}

	// 💡 新しいターンが開幕した瞬間に、次の30秒タイマーを始動！
	h.timerMgr.StartTimer(req.RoomId)

	gameData, _ := h.repo.GetGameDataByRoomID(ctx, req.RoomId)
	if gameData != nil {
		resp := convertToResponse(gameData)
		h.broadcastToGame(req.RoomId, resp)
	}

	return &pb.AcceptResponse{Success: true, Message: "new turn started successfully"}, nil
}