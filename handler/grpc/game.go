package handlergrpc

import (
	repository "auxilia/domain/interface"
	"auxilia/domain/model" // プロジェクト構造に合わせて調整してください
	"auxilia/pb"
	"context"
	"io"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"
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
			PositionX: uint32(grid.PositionX),
			PositionY: uint32(grid.PositionY),
			GridType:  grid.GridType,
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

func (h *BattleHandler) StreamGame(stream pb.BattleService_StreamGameServer) error {
	// 最初のアクションでroomIDを取得しストリーム登録
	action, err := stream.Recv()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	roomID := action.RoomId

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

	for {
		// 2回目以降のアクション受信
		if action == nil {
			action, err = stream.Recv()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
		}

		var gameData *model.GameData
		var attackInfo *model.AttackInfo
		var sendErr error

		switch a := action.GetAction().(type) {
		case *pb.PlayerAction_Move:
			if a.Move == nil {
				action = nil
				continue
			}
			gameData, sendErr = h.repo.ApplyMove(action.RoomId, action.PlayerId, a.Move.CharacterUniqueId, a.Move.ToX, a.Move.ToY)
		case *pb.PlayerAction_Attack:
			if a.Attack == nil {
				action = nil
				continue
			}
			gameData, attackInfo, sendErr = h.repo.ApplyAttack(action.RoomId, action.PlayerId, a.Attack.AttackerCharacterUniqueId, a.Attack.AttackType, a.Attack.IsStarted, a.Attack.BaseHp1, a.Attack.BaseHp2, a.Attack.AttackedCharacterUniqueId, a.Attack.NewHp)
		case *pb.PlayerAction_EndTurn:
			if !a.EndTurn {
				action = nil
				continue
			}
			gameData, sendErr = h.repo.EndTurn(action.RoomId)
		default:
			gameData, sendErr = h.repo.GetGameDataByRoomID(action.RoomId)
		}

		if sendErr != nil {
			// エラーは呼び出し元ストリームだけに返す
			return sendErr
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

		// 部屋内全ストリームに送信
		roomStreamsMu.Lock()
		streams := roomStreams[roomID]
		roomStreamsMu.Unlock()
		for _, s := range streams {
			s.Send(resp)
		}

		action = nil
	}
}
