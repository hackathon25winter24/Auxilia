package gormv2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	game "auxilia/domain/gamev2"
	"auxilia/domain/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const RulesVersion = "web-2026-09-26-v2"

const PresentationWindow = 32

var ErrForbidden = errors.New("not a participant or room member")
var ErrPrecondition = errors.New("invalid match lifecycle")
var ErrCommandConflict = errors.New("command ID reused with different payload")

type Match struct {
	PresentationJSON                         string `gorm:"type:longtext"`
	ID                                       string `gorm:"type:varchar(36);primaryKey"`
	RoomID                                   uint32 `gorm:"index"`
	StateJSON                                string `gorm:"type:longtext;not null"`
	SelectionsJSON                           string `gorm:"type:longtext;not null"`
	Finished                                 bool   `gorm:"index"`
	Started                                  bool   `gorm:"index"`
	LogSequence                              uint64
	P1Rate, P2Rate, P1RateDelta, P2RateDelta int
	CreatedAt, UpdatedAt                     time.Time
}

func (Match) TableName() string { return "battle_v2_matches" }

type Transition struct {
	PresentationJSON string `gorm:"type:longtext"`
	MatchID          string `gorm:"type:varchar(36);primaryKey"`
	Sequence         uint64 `gorm:"primaryKey;autoIncrement:false"`
	PlayerID         string `gorm:"type:varchar(36)"`
	ActionType       string
	CommandJSON      string `gorm:"type:longtext"`
	BeforeJSON       string `gorm:"type:longtext"`
	AfterJSON        string `gorm:"type:longtext"`
	CreatedAt        time.Time
}

func (Transition) TableName() string { return "battle_v2_transitions" }

type Receipt struct {
	MatchID   string `gorm:"type:varchar(36);primaryKey"`
	PlayerID  string `gorm:"type:varchar(36);primaryKey"`
	CommandID string `gorm:"type:varchar(128);primaryKey"`
	Payload   string `gorm:"type:longtext"`
}

func (Receipt) TableName() string { return "battle_v2_commands" }

type View struct {
	PresentationEvents []game.PresentationEvent
	Match              Match
	State              *game.State
	Selections         [2][]string
}
type Store struct{ DB *gorm.DB }

func New(db *gorm.DB) *Store { return &Store{DB: db} }
func (s *Store) Migrate() error {
	return s.DB.AutoMigrate(&Match{}, &Transition{}, &Receipt{}, &Session{})
}
func decode(m Match) (*View, error) {
	v := &View{Match: m, State: &game.State{}}
	if err := json.Unmarshal([]byte(m.StateJSON), v.State); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(m.SelectionsJSON), &v.Selections); err != nil {
		return nil, err
	}
	return v, nil
}
func side(st *game.State, player string) int {
	for i, p := range st.Players {
		if p.ID == player {
			return i
		}
	}
	return -1
}

// Room lock precedes match lock in every mutating path, including the scheduler.
func (s *Store) locked(ctx context.Context, id string, fn func(*gorm.DB, *View) error) error {
	// Lookup outside the transaction: an ordinary read inside would establish an
	// InnoDB REPEATABLE READ snapshot before waiting for the room lock, hiding
	// a concurrently committed command receipt from the subsequent dedup check.
	var hint Match
	if err := s.DB.WithContext(ctx).First(&hint, "id = ?", id).Error; err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var room model.RoomMatch
		roomErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&room, "id = ?", hint.RoomID).Error
		if roomErr != nil && !errors.Is(roomErr, gorm.ErrRecordNotFound) {
			return roomErr
		}
		var m Match
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&m, "id = ?", id).Error; err != nil {
			return err
		}
		// Completed matches retain receipts even after everybody leaves the room.
		if roomErr != nil && !m.Finished {
			return roomErr
		}
		v, err := decode(m)
		if err != nil {
			return err
		}
		return fn(tx, v)
	})
}
func (s *Store) Create(ctx context.Context, roomID uint32, player string) (*View, error) {
	var result *View
	if roomID == 0 || roomID > math.MaxInt32 {
		return nil, game.ErrInvalidAction
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var room model.RoomMatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&room, "id = ?", roomID).Error; err != nil {
			return err
		}
		var existing Match
		err := tx.Where("room_id = ? AND finished = ?", roomID, false).First(&existing).Error
		if err == nil {
			v, e := decode(existing)
			if e != nil {
				return e
			}
			if side(v.State, player) < 0 {
				return ErrForbidden
			}
			result = v
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if room.IsGaming {
			return ErrPrecondition
		}
		var members []model.Room
		if err := tx.Where("room_id = ? AND state IN ?", roomID, []int{1, 2}).Find(&members).Error; err != nil {
			return err
		}
		var players [2]game.Player
		for _, member := range members {
			i := int(member.State) - 1
			if !member.IsReady || players[i].ID != "" {
				return ErrPrecondition
			}
			var user model.User
			if err := tx.First(&user, "id = ?", member.UserID).Error; err != nil {
				return err
			}
			players[i] = game.Player{ID: member.UserID, Name: user.Name}
		}
		if players[0].ID == "" || players[1].ID == "" || players[0].ID == players[1].ID {
			return ErrPrecondition
		}
		if player != players[0].ID && player != players[1].ID {
			return ErrForbidden
		}
		id := uuid.NewString()
		st := game.NewPendingState(id, players, [2][]string{})
		v := &View{Match: Match{ID: id, RoomID: roomID, SelectionsJSON: "[null,null]"}, State: st}
		raw, e := json.Marshal(st)
		if e != nil {
			return e
		}
		v.Match.StateJSON = string(raw)
		if err := tx.Create(&v.Match).Error; err != nil {
			return err
		}
		if err := tx.Model(&room).Update("is_gaming", true).Error; err != nil {
			return err
		}
		if err := s.save(tx, v, "", "CREATED", "", ""); err != nil {
			return err
		}
		result = v
		return nil
	})
	return result, err
}
func (s *Store) Read(ctx context.Context, id, player string) (*View, error) {
	var m Match
	if err := s.DB.WithContext(ctx).First(&m, "id = ?", id).Error; err != nil {
		return nil, err
	}
	v, err := decode(m)
	if err != nil {
		return nil, err
	}
	if side(v.State, player) < 0 {
		var n int64
		if err := s.DB.WithContext(ctx).Model(&model.Room{}).Where("room_id = ? AND user_id = ?", m.RoomID, player).Count(&n).Error; err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, ErrForbidden
		}
	}
	v.State.ServerTime = time.Now().UTC()
	return v, nil
}

func (s *Store) RoomGame(ctx context.Context, roomID uint32, player string) (*View, error) {
	var m Match
	if err := s.DB.WithContext(ctx).Where("room_id = ?", roomID).Order("finished ASC, created_at DESC, id DESC").First(&m).Error; err != nil {
		return nil, err
	}
	return s.Read(ctx, m.ID, player)
}
func (s *Store) Select(ctx context.Context, id, player string, ids []string) (*View, error) {
	if len(ids) != 3 {
		return nil, game.ErrInvalidAction
	}
	seen := map[string]bool{}
	for i, id := range ids {
		id = game.CanonicalCharacterID(id)
		if _, ok := game.Definition(id); !ok || seen[id] {
			return nil, game.ErrInvalidAction
		}
		ids[i] = id
		seen[id] = true
	}
	return s.change(ctx, id, player, "SELECT", func(v *View) error {
		if v.State.Started || v.State.Finished {
			return ErrPrecondition
		}
		i := side(v.State, player)
		v.Selections[i] = append([]string(nil), ids...)
		rev := v.State.Revision
		events := v.State.Events
		v.State = game.NewPendingState(id, v.State.Players, v.Selections)
		v.State.Revision = rev + 1
		v.State.LastEvent = game.Event{Sequence: rev + 1, Type: "SELECTION_CHANGED", Text: "編成を登録しました"}
		v.State.Events = append(events, v.State.LastEvent)
		return nil
	})
}
func (s *Store) Ready(ctx context.Context, id, player string) (*View, error) {
	return s.change(ctx, id, player, "READY", func(v *View) error {
		if v.State.Finished {
			return ErrPrecondition
		}
		if len(v.Selections[0]) != 3 || len(v.Selections[1]) != 3 {
			return ErrPrecondition
		}
		return v.State.Ready(player)
	})
}
func (s *Store) Cancel(ctx context.Context, id, player string) (*View, error) {
	return s.change(ctx, id, player, "CANCEL", func(v *View) error {
		if v.State.Started {
			return ErrPrecondition
		}
		if v.State.Finished {
			return nil
		}
		v.State.Finished = true
		v.State.Revision++
		v.State.LastEvent = game.Event{Sequence: v.State.Revision, Type: "CANCELLED", Text: "対戦準備を中止しました"}
		return nil
	})
}
func (s *Store) change(ctx context.Context, id, player, kind string, fn func(*View) error) (*View, error) {
	var result *View
	err := s.locked(ctx, id, func(tx *gorm.DB, v *View) error {
		if side(v.State, player) < 0 {
			return ErrForbidden
		}
		before := v.Match.StateJSON
		trace := game.StartPresentation(v.State, player, kind, nil)
		if err := fn(v); err != nil {
			return err
		}
		v.PresentationEvents = trace.Finish(v.State)
		if err := s.save(tx, v, player, kind, "", before); err != nil {
			return err
		}
		result = v
		return nil
	})
	return result, err
}
func (s *Store) Apply(ctx context.Context, id, player, kind string, c game.Command) (*View, error) {
	if len(c.ID) == 0 || len(c.ID) > 128 {
		return nil, game.ErrInvalidAction
	}
	payloadBytes, _ := json.Marshal(struct {
		Kind    string
		Command game.Command
	}{kind, c})
	payload := string(payloadBytes)
	var result *View
	var actionErr error
	err := s.locked(ctx, id, func(tx *gorm.DB, v *View) error {
		if side(v.State, player) < 0 {
			return ErrForbidden
		}
		// Expiry is committed even when the submitted action is stale or invalid.
		if err := s.expire(tx, v, time.Now()); err != nil {
			return err
		}
		var receipt Receipt
		err := tx.First(&receipt, "match_id = ? AND player_id = ? AND command_id = ?", id, player, c.ID).Error
		if err == nil {
			if receipt.Payload != payload {
				actionErr = ErrCommandConflict
			}
			result = v
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		before := v.Match.StateJSON
		trace := game.StartPresentation(v.State, player, kind, &c)
		switch kind {
		case "MOVE":
			actionErr = v.State.ApplyMove(player, c)
		case "ATTACK":
			actionErr = v.State.ApplyAttack(player, c)
		case "END_TURN":
			actionErr = v.State.EndTurn(player, c.ExpectedRevision)
		case "SURRENDER":
			actionErr = v.State.Surrender(player, c.ExpectedRevision)
		default:
			actionErr = game.ErrInvalidAction
		}
		if actionErr != nil {
			return nil
		}
		v.PresentationEvents = trace.Finish(v.State)
		raw, _ := json.Marshal(c)
		if err := s.save(tx, v, player, kind, string(raw), before); err != nil {
			return err
		}
		if err := tx.Create(&Receipt{MatchID: id, PlayerID: player, CommandID: c.ID, Payload: payload}).Error; err != nil {
			return err
		}
		result = v
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, actionErr
}
func (s *Store) save(tx *gorm.DB, v *View, player, kind, command, before string) error {
	if v.State.Finished && !v.Match.Finished {
		if v.State.Started {
			if err := settle(tx, v); err != nil {
				return err
			}
		}
		if err := tx.Model(&model.RoomMatch{}).Where("id = ?", v.Match.RoomID).Update("is_gaming", false).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.Room{}).Where("room_id = ?", v.Match.RoomID).Update("is_ready", false).Error; err != nil {
			return err
		}
	}
	raw, err := json.Marshal(v.State)
	if err != nil {
		return err
	}
	picks, err := json.Marshal(v.Selections)
	if err != nil {
		return err
	}
	v.Match.StateJSON = string(raw)
	v.Match.SelectionsJSON = string(picks)
	v.Match.Finished = v.State.Finished
	v.Match.Started = v.State.Started
	v.Match.LogSequence++
	batch := game.PresentationBatch{Version: 1, Sequence: v.Match.LogSequence, AfterRevision: v.State.Revision, ActionType: kind, Events: v.PresentationEvents}
	if before != "" {
		var prior game.State
		if err := json.Unmarshal([]byte(before), &prior); err != nil {
			return err
		}
		batch.BeforeRevision = prior.Revision
	}
	if command != "" {
		var c game.Command
		if err := json.Unmarshal([]byte(command), &c); err != nil {
			return err
		}
		batch.CommandID = c.ID
	}
	if kind == "CREATED" {
		batch.Events = []game.PresentationEvent{{Type: "MATCH_CREATED", Cause: kind, TargetKind: "MATCH"}}
	}
	presentation, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	var window []game.PresentationBatch
	if v.Match.PresentationJSON != "" {
		if err := json.Unmarshal([]byte(v.Match.PresentationJSON), &window); err != nil {
			return err
		}
	}
	window = append(window, batch)
	if len(window) > PresentationWindow {
		window = window[len(window)-PresentationWindow:]
	}
	windowJSON, err := json.Marshal(window)
	if err != nil {
		return err
	}
	v.Match.PresentationJSON = string(windowJSON)
	v.PresentationEvents = nil
	log := Transition{MatchID: v.Match.ID, Sequence: v.Match.LogSequence, PlayerID: player, ActionType: kind, CommandJSON: command, BeforeJSON: before, AfterJSON: string(raw)}
	log.PresentationJSON = string(presentation)
	if err := tx.Create(&log).Error; err != nil {
		return err
	}
	return tx.Save(&v.Match).Error
}
func settle(tx *gorm.DB, v *View) error {
	var users []model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", []string{v.State.Players[0].ID, v.State.Players[1].ID}).Order("id").Find(&users).Error; err != nil {
		return err
	}
	if len(users) != 2 {
		return ErrPrecondition
	}
	var players [2]*model.User
	for i := range users {
		players[side(v.State, users[i].ID.String())] = &users[i]
	}
	rates := [2]int{players[0].Rate, players[1].Rate}
	for i := range rates {
		if rates[i] <= 0 {
			rates[i] = 1500
		}
	}
	scores := [2]float64{0.5, 0.5}
	if v.State.WinnerID == players[0].ID.String() {
		scores = [2]float64{1, 0}
	} else if v.State.WinnerID == players[1].ID.String() {
		scores = [2]float64{0, 1}
	}
	var next [2]int
	for i, p := range players {
		expected := 1 / (1 + math.Pow(10, float64(rates[1-i]-rates[i])/400))
		next[i] = int(math.Round(float64(rates[i]) + 32*(scores[i]-expected)))
		p.Rate = next[i]
		p.NumBattles++
		if scores[i] == 1 {
			p.NumWins++
		}
		if err := tx.Save(p).Error; err != nil {
			return err
		}
	}
	v.Match.P1Rate = next[0]
	v.Match.P2Rate = next[1]
	v.Match.P1RateDelta = next[0] - rates[0]
	v.Match.P2RateDelta = next[1] - rates[1]
	return nil
}
func (s *Store) expire(tx *gorm.DB, v *View, now time.Time) error {
	before := v.Match.StateJSON
	rev := v.State.Revision
	trace := game.StartPresentation(v.State, "", "TIMER", nil)
	v.State.ExpireTurn(now)
	v.PresentationEvents = trace.Finish(v.State)
	if v.State.Revision != rev {
		return s.save(tx, v, "", "TIMER", "", before)
	}
	return nil
}
func (s *Store) Tick(ctx context.Context, now time.Time) error {
	var ids []string
	if err := s.DB.WithContext(ctx).Model(&Match{}).Where("started = ? AND finished = ?", true, false).Pluck("id", &ids).Error; err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		if err := s.locked(ctx, id, func(tx *gorm.DB, v *View) error { return s.expire(tx, v, now) }); err != nil {
			failures = append(failures, fmt.Errorf("match %s: %w", id, err))
		}
	}
	return errors.Join(failures...)
}
func (s *Store) Logs(ctx context.Context, id, player string, after uint64, limit uint32) ([]Transition, error) {
	if _, err := s.Read(ctx, id, player); err != nil {
		return nil, err
	}
	if limit == 0 || limit > 100 {
		limit = 100
	}
	var rows []Transition
	err := s.DB.WithContext(ctx).Where("match_id = ? AND sequence > ?", id, after).Order("sequence").Limit(int(limit)).Find(&rows).Error
	return rows, err
}
