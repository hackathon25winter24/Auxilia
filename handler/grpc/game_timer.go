package handlergrpc

import (
	repository "auxilia/domain/interface"
	"auxilia/pb"
	"context"
	"log"
	"sync"
	"time"
)

type TurnTimerManager struct {
	repo           repository.BattleRepository
	broadcastFunc  func(roomID uint32, response *pb.GameDataResponse)
	convertToResp  func(roomID uint32) *pb.GameDataResponse
	timers         map[uint32]context.CancelFunc
	mu             sync.Mutex
	duration       time.Duration // ターンの制限時間（例: 30 * time.Second）
}

func NewTurnTimerManager(
	repo repository.BattleRepository,
	broadcastFunc func(roomID uint32, response *pb.GameDataResponse),
	convertToResp func(roomID uint32) *pb.GameDataResponse,
	duration time.Duration,
) *TurnTimerManager {
	return &TurnTimerManager{
		repo:          repo,
		broadcastFunc: broadcastFunc,
		convertToResp: convertToResp,
		timers:        make(map[uint32]context.CancelFunc),
		duration:      duration,
	}
}

// StartTimer: 新しいターンが始まった時（NewTurn呼出時）にタイマーを開始する
func (m *TurnTimerManager) StartTimer(roomID uint32) {
	m.mu.Lock()
	// 既に古いタイマーが走っていればキャンセルして上書き破棄
	if cancel, exists := m.timers[roomID]; exists {
		cancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.timers[roomID] = cancel
	m.mu.Unlock()

	// 非同期でカウントダウンを開始
	go func() {
		select {
		case <-time.After(m.duration):
			// 💡 時間切れ（タイムアウト）時の処理
			log.Printf("[Timer] Room %d timed out! Executing auto EndTurn.", roomID)
			
			// 1. リポジトリのEndTurnを叩いて IsTurnEnded = true に倒す
			if err := m.repo.EndTurn(ctx, roomID); err != nil {
				log.Printf("[Timer] Error executing auto EndTurn for room %d: %v", roomID, err)
				return
			}

			// 2. 最新の状態を取得してブロードキャスト（Unity側に通知）
			if resp := m.convertToResp(roomID); resp != nil {
				m.broadcastFunc(roomID, resp)
			}

			m.StopTimer(roomID)

		case <-ctx.Done():
			return
		}
	}()
}

// StopTimer: プレイヤーが手動でEndTurnを叩いた時などにタイマーを止める
func (m *TurnTimerManager) StopTimer(roomID uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cancel, exists := m.timers[roomID]; exists {
		cancel()
		delete(m.timers, roomID)
	}
}