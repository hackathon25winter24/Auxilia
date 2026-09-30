package game

import (
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"time"
)

const (
	Width           = 8
	Height          = 5
	MaxCost         = 50
	MaxBaseHP       = 400
	TurnDuration    = 120 * time.Second
	TurnEndDuration = 2 * time.Second
)

var (
	ErrNotYourTurn   = errors.New("相手のターンです")
	ErrInvalidAction = errors.New("実行できない操作です")
	ErrStaleRevision = errors.New("古いゲーム状態からの操作です")
)

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}
type Character struct {
	HangoverUntil  int            `json:"hangoverUntil,omitempty"`
	UsedSkills     map[string]int `json:"usedSkills,omitempty"`
	CombatStance   bool           `json:"combatStance,omitempty"`
	BarrierTurn    int            `json:"barrierTurn,omitempty"`
	Wriggling      bool           `json:"wriggling,omitempty"`
	TemporaryBuffs []string       `json:"temporaryBuffs,omitempty"`
	ID             string         `json:"id"`
	DefinitionID   string         `json:"definitionId"`
	OwnerID        string         `json:"ownerId"`
	Name           string         `json:"name"`
	HP             int            `json:"hp"`
	MaxHP          int            `json:"maxHP"`
	Position       Position       `json:"position"`
	Effects        []string       `json:"effects"`
	ReviveUsed     bool           `json:"reviveUsed,omitempty"`
	DepartureUsed  bool           `json:"departureUsed,omitempty"`
	DrankTurn      int            `json:"drankTurn,omitempty"`
	HangoverTurn   int            `json:"hangoverTurn,omitempty"`
}
type Player struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Cost int    `json:"cost"`
}
type Base struct {
	OwnerID  string   `json:"ownerId"`
	HP       int      `json:"hp"`
	MaxHP    int      `json:"maxHP"`
	Position Position `json:"position"`
}
type TileEffect struct {
	Position Position `json:"position"`
	Type     string   `json:"type"`
	OwnerID  string   `json:"ownerId"`
	HP       int      `json:"hp,omitempty"`
}
type Event struct {
	Sequence uint64 `json:"sequence"`
	Type     string `json:"type"`
	Text     string `json:"text"`
}
type State struct {
	presentation   *PresentationTrace
	TestOwnerID    string       `json:"testOwnerId,omitempty"`
	MatchID        string       `json:"matchId"`
	Revision       uint64       `json:"revision"`
	Started        bool         `json:"started"`
	ReadyPlayerIDs []string     `json:"readyPlayerIds"`
	Players        [2]Player    `json:"players"`
	Bases          [2]Base      `json:"bases"`
	Characters     []Character  `json:"characters"`
	TileEffects    []TileEffect `json:"tileEffects"`
	BlockedCells   []Position   `json:"blockedCells"`
	TurnPlayerID   string       `json:"turnPlayerId"`
	Turn           int          `json:"turn"`
	Phase          string       `json:"phase"`
	PhaseDeadline  time.Time    `json:"phaseDeadline,omitempty"`
	TurnDeadline   time.Time    `json:"turnDeadline"`
	ServerTime     time.Time    `json:"serverTime,omitempty"`
	WinnerID       string       `json:"winnerId,omitempty"`
	Finished       bool         `json:"finished"`
	LastEvent      Event        `json:"lastEvent"`
	Events         []Event      `json:"events"`
}
type Command struct {
	ID               string   `json:"commandId"`
	ExpectedRevision uint64   `json:"expectedRevision"`
	CharacterID      string   `json:"characterId"`
	AttackIndex      int      `json:"attackIndex"`
	Target           Position `json:"target"`
	Direction        Position `json:"direction"`
}

func NewState(id string, players [2]Player, selections [2][]string) *State {
	return newState(id, players, selections, true)
}

func NewTestState(id, ownerID, name string, selection []string) *State {
	players := [2]Player{{ID: name + "1", Name: name + "1"}, {ID: name + "2", Name: name + "2"}}
	s := NewState(id, players, [2][]string{selection, selection})
	s.TestOwnerID = ownerID
	return s
}

// Only the authenticated owner can control the current test side.
func (s *State) ControlledPlayer(guestID string) string {
	if s.TestOwnerID != "" && s.TestOwnerID == guestID {
		return s.TurnPlayerID
	}
	return guestID
}

func (s *State) EndTest(guestID string, expected uint64) error {
	if s.TestOwnerID == "" || s.TestOwnerID != guestID {
		return ErrInvalidAction
	}
	if s.Finished {
		return nil
	}
	if s.Revision != expected {
		return ErrStaleRevision
	}
	s.Finished = true
	s.WinnerID = ""
	s.commit("TEST_FINISHED", "テストモードを終了しました")
	return nil
}

func NewPendingState(id string, players [2]Player, selections [2][]string) *State {
	return newState(id, players, selections, false)
}

func newState(id string, players [2]Player, selections [2][]string, started bool) *State {
	lastEvent := Event{Sequence: 1, Type: "MATCH_FOUND", Text: "対戦相手が見つかりました"}
	var deadline time.Time
	if started {
		lastEvent = Event{Sequence: 1, Type: "MATCH_STARTED", Text: "対戦開始"}
		deadline = time.Now().Add(TurnDuration)
	}
	phase := "waiting"
	if started {
		phase = "action"
	}
	s := &State{MatchID: id, Revision: 1, Started: started, Players: players, TurnPlayerID: firstPlayerID(id, players, selections), Turn: 1, Phase: phase, TurnDeadline: deadline, LastEvent: lastEvent}
	s.Players[0].Cost, s.Players[1].Cost = MaxCost, MaxCost
	s.EnsureBases()
	// 編成画面の1・2・3番目を、戦闘画面でも上・中・下の順に並べる。
	starts := [2][]Position{{{0, 4}, {1, 2}, {0, 0}}, {{7, 4}, {6, 2}, {7, 0}}}
	for side := range 2 {
		for i, defID := range selections[side] {
			if i >= 3 {
				break
			}
			if def, ok := Definition(defID); ok {
				s.Characters = append(s.Characters, Character{ID: fmt.Sprintf("p%d-c%d", side+1, i+1), DefinitionID: def.ID, OwnerID: players[side].ID, Name: def.Name, HP: def.MaxHP, MaxHP: def.MaxHP, Position: starts[side][i], Effects: []string{}})
			}
		}
	}
	if started {
		s.applyStartPassives()
	}
	s.record(s.LastEvent)
	return s
}

func (s *State) applyStartPassives() {
	defer s.applyTurnStartPassives()
	for sourceIndex, source := range s.Characters {
		if source.DefinitionID != "sophie" {
			continue
		}
		endPresentation := s.presentationScope("PASSIVE", sourceIndex)
		for j := range s.Characters {
			if s.Characters[j].OwnerID == source.OwnerID {
				s.addEffect(j, "俊足")
			}
		}
		endPresentation()
	}
}

func firstPlayerID(matchID string, players [2]Player, selections [2][]string) string {
	totals := [2]int{movementCostTotal(selections[0]), movementCostTotal(selections[1])}
	if totals[0] < totals[1] {
		return players[0].ID
	}
	if totals[1] < totals[0] {
		return players[1].ID
	}

	// 同点時はマッチIDとプレイヤーIDから決める。待機列へ入った順番には
	// 依存せず、保存・再読込後にも同じ結果になる。
	scores := [2]uint32{firstPlayerTieScore(matchID, players[0].ID), firstPlayerTieScore(matchID, players[1].ID)}
	if scores[0] < scores[1] || (scores[0] == scores[1] && players[0].ID < players[1].ID) {
		return players[0].ID
	}
	return players[1].ID
}

func movementCostTotal(selection []string) int {
	total := 0
	for _, id := range selection {
		if definition, ok := Definition(id); ok {
			total += definition.MoveCost
		}
	}
	return total
}

func firstPlayerTieScore(matchID, playerID string) uint32 {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s:%s", matchID, playerID)
	return h.Sum32()
}

func (s *State) Ready(playerID string) error {
	if s.Started {
		return nil
	}
	if playerID != s.Players[0].ID && playerID != s.Players[1].ID {
		return ErrInvalidAction
	}
	for _, id := range s.ReadyPlayerIDs {
		if id == playerID {
			return nil
		}
	}
	s.ReadyPlayerIDs = append(s.ReadyPlayerIDs, playerID)
	s.Revision++
	s.LastEvent = Event{Sequence: s.Revision, Type: "PLAYER_READY", Text: "相手を待っています"}
	if len(s.ReadyPlayerIDs) == 2 {
		s.Started = true
		s.applyStartPassives()
		s.Phase = "action"
		s.TurnDeadline = time.Now().Add(TurnDuration)
		s.LastEvent = Event{Sequence: s.Revision, Type: "MATCH_STARTED", Text: "対戦開始"}
		s.Events = append(s.Events, s.LastEvent)
	}
	return nil
}

func (s *State) CancelReady(playerID string) error {
	if s.Started || s.Finished {
		return ErrInvalidAction
	}
	if playerID != s.Players[0].ID && playerID != s.Players[1].ID {
		return ErrInvalidAction
	}
	for i, id := range s.ReadyPlayerIDs {
		if id != playerID {
			continue
		}
		s.ReadyPlayerIDs = append(s.ReadyPlayerIDs[:i], s.ReadyPlayerIDs[i+1:]...)
		s.Revision++
		s.LastEvent = Event{Sequence: s.Revision, Type: "PLAYER_READY_CANCELLED", Text: "対戦開始をキャンセルしました"}
		s.record(s.LastEvent)
		break
	}
	return nil
}
func (s *State) EnsureBases() {
	positions := [2]Position{{0, 2}, {7, 2}}
	for i := range s.Bases {
		if s.Bases[i].MaxHP == 0 {
			s.Bases[i] = Base{OwnerID: s.Players[i].ID, HP: MaxBaseHP, MaxHP: MaxBaseHP, Position: positions[i]}
		} else if s.Bases[i].MaxHP != MaxBaseHP {
			s.Bases[i].HP = clamp(s.Bases[i].HP+(MaxBaseHP-s.Bases[i].MaxHP), 0, MaxBaseHP)
			s.Bases[i].MaxHP = MaxBaseHP
		}
	}
	if len(s.BlockedCells) == 0 {
		s.BlockedCells = []Position{{1, 1}, {2, 3}, {5, 1}, {6, 3}}
	}
}
func (s *State) ApplyMove(playerID string, c Command) error {
	if err := s.validate(playerID, c); err != nil {
		return err
	}
	i, d, err := s.actor(playerID, c.CharacterID)
	if err != nil {
		return err
	}
	if s.hasEffect(i, "麻痺") {
		return ErrInvalidAction
	}
	if s.immutableAt(c.Target) || (s.immutableAt(s.Characters[i].Position) && !s.ignoresDebuffTiles(i)) {
		return ErrInvalidAction
	}
	moveCost := d.MoveCost
	if s.hasEffect(i, "二日酔い") {
		moveCost += 5
	}
	if s.hasEffect(i, "俊足") {
		moveCost -= 2
	}
	if s.hasEffect(i, "鈍足") {
		moveCost += 2
	}
	if tile := s.tileAt(s.Characters[i].Position); tile >= 0 && s.TileEffects[tile].Type == "まきびし" && !s.ignoresDebuffTiles(i) {
		moveCost += 2
	}
	moveCost = max(0, moveCost)
	if !onBoard(c.Target) || s.blocked(c.Target) || distance(s.Characters[i].Position, c.Target) > d.MoveRange || s.occupied(c.Target, c.CharacterID) || s.enemyBaseAt(playerID, c.Target) || s.cost(playerID) < moveCost {
		return ErrInvalidAction
	}
	s.Characters[i].Position = c.Target
	s.observePresentation()
	s.spend(playerID, moveCost)
	s.triggerTile(i)
	s.commit("MOVED", fmt.Sprintf("%sが移動（コスト%d）", s.Characters[i].Name, moveCost))
	s.checkWinner()
	return nil
}
func (s *State) ApplyAttack(playerID string, c Command) error {
	if err := s.validate(playerID, c); err != nil {
		return err
	}
	i, d, err := s.actor(playerID, c.CharacterID)
	if err != nil || c.AttackIndex < 0 || c.AttackIndex >= len(d.Attacks) {
		return ErrInvalidAction
	}
	a := d.Attacks[c.AttackIndex]
	if a.OncePerTurn && s.Characters[i].UsedSkills[a.Name] == s.Turn {
		return fmt.Errorf("%w: この技はこのターン使用済みです", ErrInvalidAction)
	}
	if s.hasEffect(i, "麻痺") {
		return ErrInvalidAction
	}
	attackCost := a.Cost
	if s.hasEffect(i, "二日酔い") {
		attackCost += 5
	}
	if s.hasEffect(i, "俊敏化") {
		attackCost -= 2
	}
	if s.hasEffect(i, "鈍化") {
		attackCost += 2
	}
	attackCost = max(0, attackCost)
	cells := s.attackCells(i, a, c.Direction)
	if s.cost(playerID) < attackCost || !containsPosition(cells, c.Target) {
		return ErrInvalidAction
	}
	if a.Tile != "" {
		if s.baseAt(c.Target) >= 0 || s.blocked(c.Target) || s.tileAt(c.Target) >= 0 || (a.Tile != "不変" && s.occupied(c.Target, "")) {
			return ErrInvalidAction
		}
		s.setTile(c.Target, a.Tile, playerID)
		s.spend(playerID, attackCost)
		s.commit("TILE_PLACED", fmt.Sprintf("%sが%sマスを設置", s.Characters[i].Name, a.Tile))
		return nil
	}
	if d.ID == "louise" && (!s.Characters[i].CombatStance || c.AttackIndex == 2) {
		s.applyLouiseSkill(i, c.AttackIndex)
		s.spend(playerID, attackCost)
		s.commit("SKILL_USED", s.Characters[i].Name+"の"+a.Name)
		return nil
	}
	if d.ID == "liberette" && c.AttackIndex == 1 {
		s.addEffect(i, s.randomEffect(i, false))
	}
	if d.ID == "liberette" && c.AttackIndex == 2 && s.hasBuff(i) {
		a.Power += 60
	}
	if d.ID == "kasuima" && c.AttackIndex > 0 {
		seen := map[string]bool{}
		for _, effect := range s.Characters[i].Effects {
			if isDebuff(effect) {
				seen[effect] = true
			}
		}
		a.Power += len(seen) * 10
	}
	affected := 0
	if d.ID == "suima" && !s.Characters[i].Wriggling && c.AttackIndex == 1 {
		for _, effect := range []string{"俊足", "威力上昇"} {
			if !s.hasEffect(i, effect) {
				s.addEffect(i, effect)
				s.Characters[i].TemporaryBuffs = append(s.Characters[i].TemporaryBuffs, effect)
			}
		}
	}
	if d.ID == "kasuima" && c.AttackIndex == 0 {
		if !s.hasEffect(i, "威力上昇") {
			s.Characters[i].Effects = append(s.Characters[i].Effects, "威力上昇")
		}
		s.Characters[i].DrankTurn = s.Turn + 2
		s.Characters[i].HangoverTurn = s.Turn + 4
		s.Characters[i].HangoverUntil = s.Turn + 6
		s.observePresentation()
	}
	var pushed []int
	// 地雷はダメージ計算前にまとめて処理し、全対象に同じ加算値を使う。
	if d.ID == "berenice" && a.Power > 0 {
		for j := len(s.TileEffects) - 1; j >= 0; j-- {
			if s.TileEffects[j].Type == "地雷" && containsPosition(cells, s.TileEffects[j].Position) {
				a.Power += 10 + s.passiveBoost(i)
				s.TileEffects = append(s.TileEffects[:j], s.TileEffects[j+1:]...)
				s.observePresentation()
				affected++
			}
		}
	}
	for j := range s.Characters {
		if s.Characters[j].HP <= 0 || !containsPosition(cells, s.Characters[j].Position) {
			continue
		}
		same := s.Characters[j].OwnerID == playerID
		if same && a.AllyEffect != "" {
			if i != j {
				s.addEffect(j, a.AllyEffect)
				affected++
			}
			continue
		}
		if !(a.Target == "any" || a.Target == "ally" && same || a.Target == "enemy" && !same) {
			continue
		}
		s.presentationSignal("TARGETED", "CHARACTER", s.Characters[j].ID, "")
		if !same && s.consumeBarrier(j) {
			affected++
			continue
		}
		power := a.Power
		if power > 0 {
			power = s.attackPower(i, power)
			if !passiveFor(d.ID).IgnorePassiveReduce {
				power = max(0, power-s.damageReduction(j))
			}
			if multiplier := passiveFor(d.ID).DebuffedDamageMultiplier; multiplier > 1 && s.hasDebuff(j) {
				power *= multiplier
			}
		}
		s.Characters[j].HP = clamp(s.Characters[j].HP-power, 0, s.Characters[j].MaxHP)
		s.observePresentation()
		if a.ClearDebuffs {
			s.clearDebuffs(j)
		}
		if a.ClearBuffs {
			s.clearBuffs(j)
		}
		if a.Effect != "" && (!same || a.Target == "any") && s.roll(i, j, a.Effect, a.EffectChance) {
			s.addEffect(j, a.Effect)
		}
		if chance := passiveFor(d.ID).ExtraAttackChance; chance > 0 && !same && s.Characters[j].HP > 0 && s.roll(i, j, "過量使用", chance+s.passiveBoost(i)) {
			// 追撃から追撃は発動しない。追加攻撃のデバフは独立判定。
			extraPower := power * passiveFor(d.ID).ExtraAttackDamagePercent / 100
			endPresentation := s.presentationScope("FOLLOW_UP", i)
			s.presentationSignal("TARGETED", "CHARACTER", s.Characters[j].ID, "")
			s.Characters[j].HP = clamp(s.Characters[j].HP-extraPower, 0, s.Characters[j].MaxHP)
			s.observePresentation()
			if a.Effect != "" && s.roll(i, j, a.Effect+"追撃", a.EffectChance) {
				s.addEffect(j, a.Effect)
			}
			endPresentation()
		}
		affected++
		if d.ID == "kasuima" && c.AttackIndex == 2 && s.Characters[j].HP > 0 {
			pushed = append(pushed, j)
		}
	}
	for _, target := range pushed {
		s.pushBack(i, target, c.Direction)
	}
	if d.ID == "suima" && !s.Characters[i].Wriggling {
		if c.AttackIndex == 0 {
			for j := range s.Characters {
				if s.Characters[j].DefinitionID == "shicho" && s.Characters[j].HP > 0 {
					s.Characters[j].HP = max(0, s.Characters[j].HP-40)
					s.observePresentation()
					affected++
				}
			}
		}
		if c.AttackIndex == 2 {
			for j := len(s.TileEffects) - 1; j >= 0; j-- {
				if s.TileEffects[j].OwnerID != playerID && containsPosition(cells, s.TileEffects[j].Position) {
					s.TileEffects = append(s.TileEffects[:j], s.TileEffects[j+1:]...)
					s.observePresentation()
					affected++
				}
			}
		}
	}
	if a.Power > 0 && (a.Target == "enemy" || a.Target == "any") {
		for j := range s.Bases {
			if (a.Target == "any" || s.Bases[j].OwnerID != playerID) && containsPosition(cells, s.Bases[j].Position) {
				s.Bases[j].HP = clamp(s.Bases[j].HP-s.attackPower(i, a.Power), 0, s.Bases[j].MaxHP)
				s.observePresentation()
				affected++
			}
		}
	}
	// 不変マスは所有者によらず攻撃で破壊できる。足元のキャラも通常の対象判定に従う。
	if a.Power > 0 && (a.Target == "enemy" || a.Target == "any") {
		for j := len(s.TileEffects) - 1; j >= 0; j-- {
			if s.TileEffects[j].Type == "不変" && containsPosition(cells, s.TileEffects[j].Position) {
				s.damageImmutable(j, s.attackPower(i, a.Power))
				affected++
			}
		}
	}
	if affected == 0 {
		return ErrInvalidAction
	}
	if a.OncePerTurn {
		if s.Characters[i].UsedSkills == nil {
			s.Characters[i].UsedSkills = map[string]int{}
		}
		s.Characters[i].UsedSkills[a.Name] = s.Turn
	}
	if d.ID == "liberette" && c.AttackIndex == 2 {
		s.clearBuffs(i)
	}
	s.spend(playerID, attackCost)
	eventType := "ATTACKED"
	if a.Power < 0 {
		eventType = "RECOVERED"
	}
	if a.ClearBuffs && a.Power == 0 {
		eventType = "BUFFS_CLEARED"
	}
	message := fmt.Sprintf("%sの%s：%d対象に効果", s.Characters[i].Name, a.Name, affected)
	if a.Power == 0 {
		eventType = "SKILL_USED"
	}
	s.commit(eventType, message)
	s.checkWinner()
	return nil
}
func (s *State) EndTurn(playerID string, expected uint64) error {
	if !s.Started || s.Finished {
		return ErrInvalidAction
	}
	if s.Revision != expected {
		return ErrStaleRevision
	}
	if s.Phase == "turn_end" {
		return ErrInvalidAction
	}
	if s.TurnPlayerID != playerID {
		return ErrNotYourTurn
	}
	s.advanceTurn("ターン終了")
	return nil
}
func (s *State) Surrender(playerID string, expected uint64) error {
	if !s.Started || s.Finished {
		return ErrInvalidAction
	}
	if s.Revision != expected {
		return ErrStaleRevision
	}
	found := false
	for _, p := range s.Players {
		if p.ID == playerID {
			found = true
		} else {
			s.WinnerID = p.ID
		}
	}
	if !found {
		return ErrInvalidAction
	}
	s.Finished = true
	s.commit("MATCH_FINISHED", "投降により勝敗が決定")
	return nil
}
func (s *State) ExpireTurn(now time.Time) {
	if !s.Started || s.Finished {
		return
	}
	if s.Phase == "turn_end" {
		if !now.Before(s.PhaseDeadline) {
			s.completeTurnChange()
		}
		return
	}
	if now.After(s.TurnDeadline) {
		if s.TestOwnerID != "" {
			_ = s.EndTest(s.TestOwnerID, s.Revision)
			return
		}
		s.beginTurnEnd("120秒経過によりターン終了後処理")
	}
}
func (s *State) advanceTurn(reason string) {
	s.beginTurnEnd(reason)
}
func (s *State) beginTurnEnd(reason string) {
	s.processTurnEnd(s.TurnPlayerID)
	s.checkWinner()
	if s.Finished {
		return
	}
	s.Phase = "turn_end"
	s.PhaseDeadline = time.Now().Add(TurnEndDuration)
	s.TurnDeadline = time.Time{}
	s.commit("TURN_END_PROCESSING", reason)
}
func (s *State) completeTurnChange() {
	if s.Players[0].ID == s.TurnPlayerID {
		s.TurnPlayerID = s.Players[1].ID
	} else {
		s.TurnPlayerID = s.Players[0].ID
	}
	s.Turn++
	s.applyTurnStartPassives()
	for i := range s.Characters {
		c := &s.Characters[i]
		if c.OwnerID == s.TurnPlayerID && c.HangoverTurn == s.Turn && c.HP > 0 {
			s.addEffect(i, "二日酔い")
			c.HangoverTurn = 0
		}
	}
	for i := range s.Players {
		s.Players[i].Cost = MaxCost
	}
	s.Phase = "action"
	s.PhaseDeadline = time.Time{}
	s.TurnDeadline = time.Now().Add(TurnDuration)
	s.commit("TURN_ENDED", "ターン終了後処理が完了し、手番が交代しました")
}
func (s *State) validate(playerID string, c Command) error {
	if !s.Started || s.Finished {
		return ErrInvalidAction
	}
	if s.Revision != c.ExpectedRevision {
		return ErrStaleRevision
	}
	if s.Phase == "turn_end" {
		return ErrInvalidAction
	}
	if s.TurnPlayerID != playerID {
		return ErrNotYourTurn
	}
	return nil
}
func (s *State) actor(playerID, id string) (int, CharacterDefinition, error) {
	for i := range s.Characters {
		if s.Characters[i].ID == id && s.Characters[i].OwnerID == playerID && s.Characters[i].HP > 0 {
			d, _ := Definition(s.Characters[i].DefinitionID)
			if (s.Characters[i].Wriggling || s.Characters[i].CombatStance) && d.AlternateAttacks != nil {
				d.Attacks = *d.AlternateAttacks
			}
			return i, d, nil
		}
	}
	return 0, CharacterDefinition{}, ErrInvalidAction
}
func (s *State) occupied(p Position, except string) bool {
	for _, c := range s.Characters {
		if c.ID != except && c.HP > 0 && c.Position == p {
			return true
		}
	}
	return false
}
func (s *State) attackCells(actor int, a AttackDefinition, facing Position) []Position {
	if abs(facing.X)+abs(facing.Y) != 1 {
		facing = Position{1, 0}
		if s.Characters[actor].OwnerID == s.Players[1].ID {
			facing = Position{-1, 0}
		}
	}
	result := make([]Position, 0, len(a.Pattern))
	for _, offset := range a.Pattern {
		rotated := Position{offset.X*facing.X - offset.Y*facing.Y, offset.X*facing.Y + offset.Y*facing.X}
		cell := Position{s.Characters[actor].Position.X + rotated.X, s.Characters[actor].Position.Y + rotated.Y}
		if onBoard(cell) {
			result = append(result, cell)
		}
	}
	return result
}
func (s *State) blocked(position Position) bool { return containsPosition(s.BlockedCells, position) }
func containsPosition(cells []Position, p Position) bool {
	for _, cell := range cells {
		if cell == p {
			return true
		}
	}
	return false
}
func (s *State) cost(id string) int {
	for _, p := range s.Players {
		if p.ID == id {
			return p.Cost
		}
	}
	return 0
}
func (s *State) spend(id string, n int) {
	defer s.observePresentation()
	for i := range s.Players {
		if s.Players[i].ID == id {
			s.Players[i].Cost -= n
		}
	}
}
func (s *State) record(e Event) {
	s.Events = append(s.Events, e)
	if len(s.Events) > 30 {
		s.Events = s.Events[len(s.Events)-30:]
	}
}
func (s *State) commit(t, text string) {
	s.observePresentation()
	s.Revision++
	s.LastEvent = Event{Sequence: s.Revision, Type: t, Text: text}
	s.record(s.LastEvent)
}
func (s *State) checkWinner() {
	for i := range s.Characters {
		c := &s.Characters[i]
		if c.DefinitionID == "sophie" && c.HP <= 0 && !c.DepartureUsed {
			endPresentation := s.presentationScope("PASSIVE", i)
			c.DepartureUsed = true
			for j := range s.Characters {
				if s.Characters[j].OwnerID != c.OwnerID && s.Characters[j].HP > 0 {
					s.addEffect(j, "鈍足")
				}
			}
			s.commit("PASSIVE_ACTIVATED", "ソフィーの播種：敵全体に鈍足")
			endPresentation()
		}
	}
	// 撃破・毒・地雷などの解決後、勝敗を決める前に一度だけ復活する。
	for i := range s.Characters {
		c := &s.Characters[i]
		if c.DefinitionID == "verbulus" && c.HP <= 0 && !c.ReviveUsed {
			endPresentation := s.presentationScope("REVIVE", i)
			c.ReviveUsed = true
			c.Effects = []string{}
			s.observePresentation()
			c.HP = min(c.MaxHP, 50+s.passiveBoost(i))
			s.commit("REVIVED", fmt.Sprintf("%sが輪廻転生により全バフ・デバフを解除しHP%dで復活", c.Name, c.HP))
			endPresentation()
		}
	}
	alive := [2]int{}
	for _, c := range s.Characters {
		for i, p := range s.Players {
			if c.OwnerID == p.ID && c.HP > 0 {
				alive[i]++
			}
		}
	}
	winner := -1
	reason := ""
	if alive[0] == 0 && alive[1] == 0 {
		if s.Bases[0].HP > s.Bases[1].HP {
			winner = 0
		} else {
			winner = 1
		}
		reason = "両軍全滅のため拠点耐久力で決着"
	} else if alive[0] == 0 {
		winner = 1
		reason = "相手キャラクター3体を戦闘不能にして勝利"
	} else if alive[1] == 0 {
		winner = 0
		reason = "相手キャラクター3体を戦闘不能にして勝利"
	} else if s.Bases[0].HP == 0 && s.Bases[1].HP == 0 {
		if alive[0] > alive[1] {
			winner = 0
		} else {
			winner = 1
		}
		reason = "両拠点陥落のため生存キャラクター数で決着"
	} else if s.Bases[0].HP == 0 {
		winner = 1
		reason = "相手拠点の耐久力を0にして勝利"
	} else if s.Bases[1].HP == 0 {
		winner = 0
		reason = "相手拠点の耐久力を0にして勝利"
	}
	if winner >= 0 && !s.Finished {
		s.Finished = true
		s.WinnerID = s.Players[winner].ID
		s.commit("MATCH_FINISHED", reason)
	}
}
func (s *State) enemyBaseAt(playerID string, p Position) bool { return s.enemyBase(playerID, p) >= 0 }
func (s *State) enemyBase(playerID string, p Position) int {
	for i, b := range s.Bases {
		if b.OwnerID != playerID && b.Position == p && b.HP > 0 {
			return i
		}
	}
	return -1
}
func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
func onBoard(p Position) bool    { return p.X >= 0 && p.X < Width && p.Y >= 0 && p.Y < Height }
func distance(a, b Position) int { return int(math.Abs(float64(a.X-b.X)) + math.Abs(float64(a.Y-b.Y))) }
func inSurroundingArea(a, b Position) bool {
	return abs(a.X-b.X) <= 1 && abs(a.Y-b.Y) <= 1
}
func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
