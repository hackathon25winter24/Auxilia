package game

import "testing"

func fixture() *State {
	s := NewState("m1", [2]Player{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}, [2][]string{{"zina", "jude", "dana"}, {"sophie", "chiyo", "aoi"}})
	s.TurnPlayerID = "a"
	return s
}

func TestPendingMatchStartsAfterBothPlayersAreReady(t *testing.T) {
	s := NewPendingState("m1", [2]Player{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}, [2][]string{{"zina", "jude", "dana"}, {"sophie", "chiyo", "aoi"}})
	if s.Started {
		t.Fatal("pending match started before either player was ready")
	}
	if err := s.Ready("a"); err != nil {
		t.Fatal(err)
	}
	if s.Started || len(s.ReadyPlayerIDs) != 1 {
		t.Fatalf("started=%v ready=%v", s.Started, s.ReadyPlayerIDs)
	}
	if err := s.Ready("b"); err != nil {
		t.Fatal(err)
	}
	if !s.Started || s.TurnDeadline.IsZero() {
		t.Fatalf("started=%v deadline=%v", s.Started, s.TurnDeadline)
	}
}

func TestPlayerCanCancelReadyBeforeOpponentStarts(t *testing.T) {
	s := NewPendingState("m1", [2]Player{{ID: "a", Name: "A"}, {ID: "b", Name: "B"}}, [2][]string{{"zina", "jude", "dana"}, {"sophie", "chiyo", "aoi"}})
	if err := s.Ready("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelReady("a"); err != nil {
		t.Fatal(err)
	}
	if s.Started || len(s.ReadyPlayerIDs) != 0 {
		t.Fatalf("started=%v ready=%v", s.Started, s.ReadyPlayerIDs)
	}
	if err := s.Ready("a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Ready("b"); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelReady("a"); err != ErrInvalidAction {
		t.Fatalf("開始後のキャンセル結果=%v", err)
	}
}

func TestInitialPositionsUseBottomLeftOriginLayout(t *testing.T) {
	s := fixture()
	want := []Position{{0, 4}, {1, 2}, {0, 0}, {7, 4}, {6, 2}, {7, 0}}
	for i, position := range want {
		if s.Characters[i].Position != position {
			t.Fatalf("character %d position=%v want=%v", i, s.Characters[i].Position, position)
		}
	}
}

func TestLowerMovementCostTotalGoesFirst(t *testing.T) {
	players := [2]Player{{ID: "slow"}, {ID: "fast"}}
	selections := [2][]string{{"shicho", "jude", "sophie"}, {"tsukiha", "chiyo", "zina"}}
	s := NewPendingState("movement-cost-match", players, selections)
	if s.TurnPlayerID != "fast" {
		t.Fatalf("先攻=%q, want fast (移動コスト合計が小さい側)", s.TurnPlayerID)
	}
}

func TestTiedMovementCostDoesNotDependOnQueueOrder(t *testing.T) {
	players := [2]Player{{ID: "alice"}, {ID: "bob"}}
	selections := [2][]string{{"sophie", "jude", "dana"}, {"sophie", "jude", "dana"}}
	first := NewPendingState("tied-movement-cost-match", players, selections).TurnPlayerID

	swappedPlayers := [2]Player{players[1], players[0]}
	swappedSelections := [2][]string{selections[1], selections[0]}
	second := NewPendingState("tied-movement-cost-match", swappedPlayers, swappedSelections).TurnPlayerID
	if first != second {
		t.Fatalf("待機順変更前の先攻=%q, 変更後=%q", first, second)
	}
}

func TestSophieNoLongerBoostsNearbyAttack(t *testing.T) {
	s := NewState("m1", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"sophie", "chiyo", "aoi"}, {"jude", "nadia", "dana"}})
	s.Characters[0].Position = Position{2, 2}
	s.Characters[1].Position = Position{3, 2}
	s.Characters[1].HP-- // 千代自身の最大HP時パッシブをこの検証から除外する。

	if got := s.attackPower(0, 10); got != 10 {
		t.Fatalf("ソフィー自身の攻撃力=%d, want=10", got)
	}
	if got := s.attackPower(1, 10); got != 10 {
		t.Fatalf("隣接する味方の攻撃力=%d, want=10", got)
	}
}

func TestTsukihaIgnoresPersistentDebuffTileEffects(t *testing.T) {
	s := NewState("m1", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"tsukiha", "jude", "dana"}, {"sophie", "chiyo", "aoi"}})
	s.TurnPlayerID = "a"
	tsukiha := &s.Characters[0]
	tsukiha.Position = Position{2, 2}
	s.TileEffects = []TileEffect{{Position: tsukiha.Position, Type: "まきびし", OwnerID: "b"}}
	beforeCost := s.Players[0].Cost
	if err := s.ApplyMove("a", Command{ExpectedRevision: s.Revision, CharacterID: tsukiha.ID, Target: Position{3, 2}}); err != nil {
		t.Fatal(err)
	}
	if spent := beforeCost - s.Players[0].Cost; spent != 3 {
		t.Fatalf("まきびし上からの移動コスト=%d, want=3", spent)
	}

	tsukiha.Position = Position{2, 2}
	tsukiha.Effects = nil
	s.TileEffects = []TileEffect{{Position: tsukiha.Position, Type: "毒ガス", OwnerID: "b"}}
	s.processTurnEnd("a")
	if s.hasEffect(0, "毒") {
		t.Fatal("月葉に毒ガスマスのターン終了時毒が付与されました")
	}
}

func TestDanaReceivesDebuffTileDamageAndMoveCost(t *testing.T) {
	s := NewState("m1", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"dana", "jude", "aoi"}, {"sophie", "chiyo", "zina"}})
	s.TurnPlayerID = "a"
	dana := &s.Characters[0]
	dana.Position = Position{2, 2}
	s.TileEffects = []TileEffect{{Position: dana.Position, Type: "まきびし", OwnerID: "b"}}
	beforeCost := s.Players[0].Cost
	if err := s.ApplyMove("a", Command{ExpectedRevision: s.Revision, CharacterID: dana.ID, Target: Position{3, 2}}); err != nil {
		t.Fatal(err)
	}
	if spent := beforeCost - s.Players[0].Cost; spent != 11 {
		t.Fatalf("まきびし上からの移動コスト=%d, want=11", spent)
	}

	dana.Position = Position{2, 2}
	dana.HP = dana.MaxHP
	s.TileEffects = []TileEffect{{Position: Position{3, 2}, Type: "地雷", OwnerID: "b"}}
	if err := s.ApplyMove("a", Command{ExpectedRevision: s.Revision, CharacterID: dana.ID, Target: Position{3, 2}}); err != nil {
		t.Fatal(err)
	}
	if dana.HP != dana.MaxHP-100 {
		t.Fatalf("地雷移動後のHP=%d, want=%d", dana.HP, dana.MaxHP-100)
	}
}

func TestBoardAndBaseInitialState(t *testing.T) {
	s := fixture()
	want := []Position{{1, 1}, {2, 3}, {5, 1}, {6, 3}}
	for _, position := range want {
		if !s.blocked(position) {
			t.Fatalf("missing blocked cell %v", position)
		}
	}
	for _, base := range s.Bases {
		if base.HP != 400 || base.MaxHP != 400 {
			t.Fatalf("base=%+v", base)
		}
	}
}

func TestExistingBaseStateMigratesFromThreeHundredToFourHundred(t *testing.T) {
	s := fixture()
	s.Bases[0].HP = 250
	s.Bases[0].MaxHP = 300
	s.EnsureBases()
	if s.Bases[0].HP != 350 || s.Bases[0].MaxHP != 400 {
		t.Fatalf("base=%+v", s.Bases[0])
	}
}

func TestAttackPatternRotatesWithDirection(t *testing.T) {
	s := NewState("m1", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"sophie", "dana", "aoi"}, {"jude", "chiyo", "zina"}})
	s.Characters[0].Position = Position{3, 3}
	a, _ := Definition("sophie")
	tests := []struct{ direction, want Position }{{Position{-1, 0}, Position{2, 3}}, {Position{0, 1}, Position{3, 4}}, {Position{1, 0}, Position{4, 3}}, {Position{0, -1}, Position{3, 2}}}
	for _, test := range tests {
		cells := s.attackCells(0, a.Attacks[0], test.direction)
		if !containsPosition(cells, test.want) {
			t.Fatalf("direction=%v cells=%v want=%v", test.direction, cells, test.want)
		}
	}
}
func TestRejectsOpponentTurn(t *testing.T) {
	s := fixture()
	err := s.ApplyMove("b", Command{ExpectedRevision: s.Revision, CharacterID: "p2-c1", Target: Position{6, 1}})
	if err != ErrNotYourTurn {
		t.Fatalf("got %v", err)
	}
}
func TestRejectsOutOfRangeMove(t *testing.T) {
	s := fixture()
	err := s.ApplyMove("a", Command{ExpectedRevision: s.Revision, CharacterID: "p1-c1", Target: Position{6, 4}})
	if err != ErrInvalidAction {
		t.Fatalf("got %v", err)
	}
}
func TestServerCalculatesDamage(t *testing.T) {
	s := fixture()
	s.Characters[0].Position = Position{0, 0}
	s.Characters[3].Position = Position{3, 0}
	err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: "p1-c1", AttackIndex: 0, Target: Position{3, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Characters[3].HP != 70 {
		t.Fatalf("hp=%d", s.Characters[3].HP)
	}
}
func TestRejectsStaleRevision(t *testing.T) {
	s := fixture()
	old := s.Revision
	_ = s.ApplyMove("a", Command{ExpectedRevision: old, CharacterID: "p1-c1", Target: Position{0, 3}})
	if err := s.ApplyMove("a", Command{ExpectedRevision: old, CharacterID: "p1-c1", Target: Position{2, 1}}); err != ErrStaleRevision {
		t.Fatalf("got %v", err)
	}
}

func TestServerDamagesBaseAndEndsMatch(t *testing.T) {
	s := fixture()
	s.Characters[0].Position = Position{4, 2}
	s.Characters[4].Position = Position{6, 3}
	s.Bases[1].HP = 20
	err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: "p1-c1", AttackIndex: 0, Target: Position{7, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Bases[1].HP != 0 {
		t.Fatalf("base hp=%d", s.Bases[1].HP)
	}
	if !s.Finished || s.WinnerID != "a" {
		t.Fatalf("finished=%v winner=%q", s.Finished, s.WinnerID)
	}
}

func TestShichoProgressAttackHitsSelfAndFriendlyBase(t *testing.T) {
	s := NewState("m1", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"shicho", "jude", "dana"}, {"sophie", "chiyo", "aoi"}})
	s.TurnPlayerID = "a"
	s.Characters[0].Position = s.Bases[0].Position
	beforeCharacter := s.Characters[0].HP
	beforeBase := s.Bases[0].HP
	if err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: "p1-c1", AttackIndex: 0, Target: s.Characters[0].Position}); err != nil {
		t.Fatal(err)
	}
	if s.Characters[0].HP >= beforeCharacter {
		t.Fatalf("新著自身にダメージが入っていません: hp=%d", s.Characters[0].HP)
	}
	if s.Bases[0].HP >= beforeBase {
		t.Fatalf("味方拠点にダメージが入っていません: hp=%d", s.Bases[0].HP)
	}
}

func TestRejectsMovingOntoEnemyBase(t *testing.T) {
	s := fixture()
	s.Characters[0].Position = Position{6, 2}
	err := s.ApplyMove("a", Command{ExpectedRevision: s.Revision, CharacterID: "p1-c1", Target: Position{7, 2}})
	if err != ErrInvalidAction {
		t.Fatalf("got %v", err)
	}
}

func TestSurrenderAwardsWinToOpponent(t *testing.T) {
	s := fixture()
	if err := s.Surrender("a", s.Revision); err != nil {
		t.Fatal(err)
	}
	if !s.Finished || s.WinnerID != "b" {
		t.Fatalf("finished=%v winner=%q", s.Finished, s.WinnerID)
	}
}

func TestRecoveryAttackProducesRecoveryEvent(t *testing.T) {
	s := fixture()
	jude := &s.Characters[1]
	jude.HP -= 30
	err := s.ApplyAttack("a", Command{
		ExpectedRevision: s.Revision,
		CharacterID:      jude.ID,
		AttackIndex:      2,
		Target:           jude.Position,
		Direction:        Position{1, 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.LastEvent.Type != "RECOVERED" {
		t.Fatalf("event type=%q, want RECOVERED", s.LastEvent.Type)
	}
}

func TestSurrenderIsAllowedOutsidePlayersTurn(t *testing.T) {
	s := fixture()
	s.TurnPlayerID = "a"
	if err := s.Surrender("b", s.Revision); err != nil {
		t.Fatal(err)
	}
	if !s.Finished || s.WinnerID != "a" {
		t.Fatalf("finished=%v winner=%q", s.Finished, s.WinnerID)
	}
}

func TestMineDealsDamageAndDisappears(t *testing.T) {
	s := NewState("m1", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"berenice", "jude", "dana"}, {"sophie", "chiyo", "aoi"}})
	s.TurnPlayerID = "a"
	s.Characters[0].Position = Position{2, 2}
	if err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: "p1-c1", AttackIndex: 0, Target: Position{3, 2}}); err != nil {
		t.Fatal(err)
	}
	s.advanceTurn("test")
	s.ExpireTurn(s.PhaseDeadline)
	s.Characters[3].Position = Position{4, 2}
	before := s.Characters[3].HP
	if err := s.ApplyMove("b", Command{ExpectedRevision: s.Revision, CharacterID: "p2-c1", Target: Position{3, 2}}); err != nil {
		t.Fatal(err)
	}
	if s.Characters[3].HP != before-100 || len(s.TileEffects) != 0 {
		t.Fatalf("hp=%d tiles=%v", s.Characters[3].HP, s.TileEffects)
	}
}

func TestPoisonDealsFortyDamageAtTurnEnd(t *testing.T) {
	s := fixture()
	s.Characters[0].Effects = []string{"毒"}
	before := s.Characters[0].HP
	s.advanceTurn("test")
	if s.Characters[0].HP != before-40 {
		t.Fatalf("hp=%d", s.Characters[0].HP)
	}
}

func TestTurnEndRecoveryIsAppliedBeforePoisonDamage(t *testing.T) {
	s := NewState("m1", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"shicho", "jude", "dana"}, {"sophie", "chiyo", "aoi"}})
	s.TurnPlayerID = "a"
	shicho := &s.Characters[0]
	shicho.HP = 35
	shicho.Effects = []string{"毒"}

	s.advanceTurn("test")

	// 先に10回復して45、その後に毒の40ダメージを受ける。
	if shicho.HP != 5 {
		t.Fatalf("hp=%d, want 5 (35 + 10 recovery - 40 poison)", shicho.HP)
	}
	if len(s.Events) < 3 || s.Events[len(s.Events)-3].Type != "TURN_END_RECOVERY" || s.Events[len(s.Events)-2].Type != "TURN_END_DAMAGE" {
		t.Fatalf("turn-end event order=%v", s.Events)
	}
}

func TestTurnEndProcessingPhasePrecedesNextTurn(t *testing.T) {
	s := fixture()
	s.Characters[0].Effects = []string{"毒"}
	beforeHP := s.Characters[0].HP
	beforeTurn := s.Turn

	if err := s.EndTurn("a", s.Revision); err != nil {
		t.Fatal(err)
	}
	if s.Phase != "turn_end" || s.LastEvent.Type != "TURN_END_PROCESSING" {
		t.Fatalf("phase=%q event=%q", s.Phase, s.LastEvent.Type)
	}
	if s.Turn != beforeTurn || s.TurnPlayerID != "a" {
		t.Fatalf("processing中に手番が変化: turn=%d player=%q", s.Turn, s.TurnPlayerID)
	}
	if s.Characters[0].HP != beforeHP-40 {
		t.Fatalf("processing中に毒が適用されていません: hp=%d", s.Characters[0].HP)
	}

	s.ExpireTurn(s.PhaseDeadline)
	if s.Phase != "action" || s.LastEvent.Type != "TURN_ENDED" {
		t.Fatalf("phase=%q event=%q", s.Phase, s.LastEvent.Type)
	}
	if s.Turn != beforeTurn+1 || s.TurnPlayerID != "b" {
		t.Fatalf("手番が交代していません: turn=%d player=%q", s.Turn, s.TurnPlayerID)
	}
}

func TestAoiPassiveHealsAllEightSurroundingCells(t *testing.T) {
	s := NewState("m1", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"aoi", "jude", "dana"}, {"sophie", "chiyo", "zina"}})
	s.Characters[0].Position = Position{3, 2}
	s.Characters[1].Position = Position{4, 3} // 斜め隣接
	s.Characters[2].Position = Position{5, 3} // 範囲外
	s.Characters[1].HP = 100
	s.Characters[2].HP = 100
	s.processTurnEnd("a")
	if s.Characters[1].HP != 130 {
		t.Fatalf("斜め隣接する味方のHP=%d, want=130", s.Characters[1].HP)
	}
	if s.Characters[2].HP != 100 {
		t.Fatalf("範囲外の味方のHP=%d, want=100", s.Characters[2].HP)
	}
}

func TestJudeReducesDamage(t *testing.T) {
	s := NewState("m1", [2]Player{{ID: "a"}, {ID: "b"}}, [2][]string{{"zina", "dana", "aoi"}, {"jude", "chiyo", "sophie"}})
	s.Characters[0].Position = Position{0, 0}
	s.Characters[3].Position = Position{3, 0}
	before := s.Characters[3].HP
	if err := s.ApplyAttack("a", Command{ExpectedRevision: s.Revision, CharacterID: "p1-c1", AttackIndex: 0, Target: Position{3, 0}}); err != nil {
		t.Fatal(err)
	}
	if s.Characters[3].HP != before-10 {
		t.Fatalf("hp=%d", s.Characters[3].HP)
	}
}
