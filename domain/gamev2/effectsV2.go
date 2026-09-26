package game

import (
	"fmt"
	"hash/fnv"
)

func (s *State) hasEffect(character int, effect string) bool {
	for _, v := range s.Characters[character].Effects {
		if v == effect {
			return true
		}
	}
	return false
}
func (s *State) addEffect(character int, effect string) {
	if s.Characters[character].DefinitionID == "kasuima" && isBuff(effect) {
		return
	}
	if s.Characters[character].DefinitionID == "dana" && !isBuff(effect) {
		return
	}
	if !isBuff(effect) && s.hasEffect(character, "免疫") {
		s.removeEffect(character, "免疫")
		return
	}
	if effect == "結界" {
		s.Characters[character].BarrierTurn = s.Turn + 1
	}
	if !s.hasEffect(character, effect) {
		s.Characters[character].Effects = append(s.Characters[character].Effects, effect)
	}
}
func isDebuff(effect string) bool {
	switch effect {
	case "毒", "麻痺", "鈍足", "鈍化", "出血", "二日酔い":
		return true
	}
	return false
}
func (s *State) hasDebuff(character int) bool {
	for _, effect := range s.Characters[character].Effects {
		if isDebuff(effect) {
			return true
		}
	}
	return false
}
func (s *State) clearDebuffs(character int) {
	kept := s.Characters[character].Effects[:0]
	for _, effect := range s.Characters[character].Effects {
		if !isDebuff(effect) {
			kept = append(kept, effect)
		}
	}
	s.Characters[character].Effects = kept
}

func (s *State) clearBuffs(character int) {
	kept := s.Characters[character].Effects[:0]
	for _, effect := range s.Characters[character].Effects {
		if !isBuff(effect) {
			kept = append(kept, effect)
		}
	}
	s.Characters[character].Effects = kept
}
func (s *State) roll(actor, target int, effect string, chance int) bool {
	h := fnv.New32a()
	fmt.Fprintf(h, "%s:%d:%s:%s:%s", s.MatchID, s.Revision, s.Characters[actor].ID, s.Characters[target].ID, effect)
	return int(h.Sum32()%100) < chance
}
func (s *State) passiveBoost(character int) int {
	boost := 0
	for i, c := range s.Characters {
		if c.HP > 0 && c.OwnerID == s.Characters[character].OwnerID && passiveFor(c.DefinitionID).PassiveValueBoost > 0 && inSurroundingArea(c.Position, s.Characters[character].Position) && i != character {
			boost = max(boost, passiveFor(c.DefinitionID).PassiveValueBoost)
		}
	}
	return boost
}
func (s *State) attackPower(actor, power int) int {
	if s.hasEffect(actor, "二日酔い") {
		power = power * 80 / 100
	}
	if s.hasEffect(actor, "威力上昇") {
		power = power * 125 / 100
	}
	if s.hasEffect(actor, "出血") {
		power = power * 75 / 100
	}
	boost := s.passiveBoost(actor)
	if fullHPBoost := passiveFor(s.Characters[actor].DefinitionID).FullHPAttackBoost; fullHPBoost > 0 && s.Characters[actor].HP == s.Characters[actor].MaxHP {
		power += fullHPBoost + boost
	}
	for i, c := range s.Characters {
		if c.HP <= 0 || c.OwnerID != s.Characters[actor].OwnerID || !inSurroundingArea(c.Position, s.Characters[actor].Position) {
			continue
		}
		passive := passiveFor(c.DefinitionID)
		if attackBoost := passive.AttackBoost; attackBoost > 0 && !(passive.ExcludeSelf && i == actor) {
			power += attackBoost + s.passiveBoost(i)
		}
	}
	return power
}
func (s *State) damageReduction(character int) int {
	if reduction := passiveFor(s.Characters[character].DefinitionID).DamageReduction; reduction > 0 {
		return reduction + s.passiveBoost(character)
	}
	return 0
}
func (s *State) tileAt(position Position) int {
	for i, tile := range s.TileEffects {
		if tile.Position == position {
			return i
		}
	}
	return -1
}
func (s *State) baseAt(position Position) int {
	for i, base := range s.Bases {
		if base.Position == position {
			return i
		}
	}
	return -1
}
func (s *State) setTile(position Position, tileType, ownerID string) {
	tile := TileEffect{Position: position, Type: tileType, OwnerID: ownerID}
	if tileType == "不変" {
		tile.HP = 170
	}
	if i := s.tileAt(position); i >= 0 {
		return
	}
	s.TileEffects = append(s.TileEffects, tile)
}

func (s *State) immutableAt(position Position) bool {
	i := s.tileAt(position)
	return i >= 0 && s.TileEffects[i].Type == "不変"
}

func (s *State) damageImmutable(index, amount int) {
	tile := &s.TileEffects[index]
	damage := min(tile.HP, max(0, amount))
	tile.HP -= damage
	text := fmt.Sprintf("不変マス(%d,%d)に%dダメージ（残りHP%d）", tile.Position.X, tile.Position.Y, damage, tile.HP)
	if tile.HP <= 0 {
		s.TileEffects = append(s.TileEffects[:index], s.TileEffects[index+1:]...)
		s.commit("TILE_DESTROYED", text+"：消滅")
	} else {
		s.commit("TILE_DAMAGED", text)
	}
}
func (s *State) triggerTile(character int) {
	index := s.tileAt(s.Characters[character].Position)
	if index < 0 {
		return
	}
	tile := s.TileEffects[index]
	id := s.Characters[character].DefinitionID
	if id == "tsukiha" || id == "berenice" && tile.Type == "地雷" {
		return
	}
	switch tile.Type {
	case "地雷":
		s.Characters[character].HP = clamp(s.Characters[character].HP-100, 0, s.Characters[character].MaxHP)
		s.TileEffects = append(s.TileEffects[:index], s.TileEffects[index+1:]...)
	case "まきびし":
		s.Characters[character].HP = clamp(s.Characters[character].HP-10, 0, s.Characters[character].MaxHP)
	case "毒ガス":
		if s.roll(character, character, "毒ガスマス", 50) {
			s.addEffect(character, "毒")
		}
	}
}

func (s *State) ignoresDebuffTiles(character int) bool {
	return s.Characters[character].DefinitionID == "tsukiha"
}

func (s *State) processTurnEnd(playerID string) {
	// 同時に解決されるターン終了時効果は、回復をすべて適用してから
	// ダメージを適用する。途中のHPで勝敗判定は行わない。
	healedTargets := 0
	healedAmount := 0
	for i, c := range s.Characters {
		if c.HP <= 0 || c.OwnerID != playerID {
			continue
		}
		boost := s.passiveBoost(i)
		if turnHeal := passiveFor(c.DefinitionID).TurnHeal; turnHeal > 0 {
			targets, amount := s.healNearby(i, turnHeal+boost)
			healedTargets += targets
			healedAmount += amount
		}
	}
	if healedAmount > 0 {
		s.commit("TURN_END_RECOVERY", fmt.Sprintf("パッシブにより%d体を合計%d回復", healedTargets, healedAmount))
	}

	poisonedTargets := 0
	poisonDamage := 0
	gasPoisonedTargets := 0
	for i := range s.Characters {
		c := &s.Characters[i]
		if c.OwnerID != playerID || c.HP <= 0 {
			continue
		}
		if s.hasEffect(i, "毒") {
			before := c.HP
			c.HP = clamp(c.HP-40, 0, c.MaxHP)
			poisonedTargets++
			poisonDamage += before - c.HP
		}
		if tile := s.tileAt(c.Position); tile >= 0 && s.TileEffects[tile].Type == "毒ガス" && !s.ignoresDebuffTiles(i) {
			alreadyPoisoned := s.hasEffect(i, "毒")
			s.addEffect(i, "毒")
			if !alreadyPoisoned && s.hasEffect(i, "毒") {
				gasPoisonedTargets++
			}
		}
		effects := c.Effects[:0]
		for _, effect := range c.Effects {
			temporary := false
			for _, buff := range c.TemporaryBuffs {
				if effect == buff {
					temporary = true
				}
			}
			if !temporary && effect != "麻痺" && !(effect == "二日酔い" && s.Turn >= c.HangoverUntil) && !(effect == "威力上昇" && c.DrankTurn > 0 && s.Turn >= c.DrankTurn) {
				effects = append(effects, effect)
			}
		}
		c.Effects = effects
		if s.Turn >= c.DrankTurn {
			c.DrankTurn = 0
		}
		c.TemporaryBuffs = nil
		if c.DefinitionID == "suima" {
			c.Wriggling = !c.Wriggling
		}
	}
	if poisonedTargets > 0 {
		s.commit("TURN_END_DAMAGE", fmt.Sprintf("%d体が毒により合計%dダメージ", poisonedTargets, poisonDamage))
	}
	if gasPoisonedTargets > 0 {
		s.commit("TURN_END_EFFECT", fmt.Sprintf("毒ガスマスにより%d体に毒を付与", gasPoisonedTargets))
	}
	for i := len(s.TileEffects) - 1; i >= 0; i-- {
		if s.TileEffects[i].Type == "不変" {
			s.damageImmutable(i, 50)
		}
	}
}
func (s *State) healNearby(source, amount int) (int, int) {
	targets := 0
	total := 0
	for i := range s.Characters {
		if !(i == source && passiveFor(s.Characters[source].DefinitionID).ExcludeSelf) && s.Characters[i].HP > 0 && s.Characters[i].OwnerID == s.Characters[source].OwnerID && inSurroundingArea(s.Characters[i].Position, s.Characters[source].Position) {
			before := s.Characters[i].HP
			s.Characters[i].HP = clamp(s.Characters[i].HP+amount, 0, s.Characters[i].MaxHP)
			if healed := s.Characters[i].HP - before; healed > 0 {
				targets++
				total += healed
			}
		}
	}
	return targets, total
}

var randomBuffs = []string{"威力上昇", "俊足", "俊敏化", "免疫", "結界"}
var randomDebuffs = []string{"毒", "麻痺", "鈍足", "鈍化", "出血"}

func isBuff(effect string) bool {
	for _, e := range randomBuffs {
		if e == effect {
			return true
		}
	}
	return false
}
func (s *State) hasBuff(i int) bool {
	for _, e := range s.Characters[i].Effects {
		if isBuff(e) {
			return true
		}
	}
	return false
}
func (s *State) removeEffect(i int, effect string) {
	kept := s.Characters[i].Effects[:0]
	for _, e := range s.Characters[i].Effects {
		if e != effect {
			kept = append(kept, e)
		}
	}
	s.Characters[i].Effects = kept
}
func (s *State) randomIndex(actor int, salt string, n int) int {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s:%d:%d:%s:%s", s.MatchID, s.Revision, s.Turn, s.Characters[actor].ID, salt)
	return int(h.Sum64() % uint64(n))
}
func (s *State) randomEffect(actor int, includeDebuffs bool) string {
	effects := append([]string{}, randomBuffs...)
	if includeDebuffs {
		effects = append(effects, randomDebuffs...)
	}
	return effects[s.randomIndex(actor, "effect", len(effects))]
}
func (s *State) applyTurnStartPassives() {
	for i, c := range s.Characters {
		if c.BarrierTurn < s.Turn {
			s.removeEffect(i, "結界")
		}
	}
	for i, c := range s.Characters {
		if c.HP <= 0 || c.OwnerID != s.TurnPlayerID || c.DefinitionID != "liberette" {
			continue
		}
		var targets []int
		for j, t := range s.Characters {
			if t.HP > 0 {
				targets = append(targets, j)
			}
		}
		if len(targets) == 0 {
			continue
		}
		j := targets[s.randomIndex(i, "target", len(targets))]
		effect := s.randomEffect(i, true)
		s.addEffect(j, effect)
	}
}
func (s *State) applyLouiseSkill(actor, attack int) {
	if attack == 2 {
		s.Characters[actor].CombatStance = !s.Characters[actor].CombatStance
		return
	}
	for j, c := range s.Characters {
		if c.HP <= 0 || c.OwnerID != s.Characters[actor].OwnerID {
			continue
		}
		if attack == 0 {
			s.clearDebuffs(j)
			s.addEffect(j, "免疫")
		} else {
			if j != actor {
				s.Characters[j].HP = min(c.MaxHP, c.HP+50)
			}
			s.addEffect(j, "結界")
		}
	}
}
func (s *State) consumeBarrier(i int) bool {
	if s.hasEffect(i, "結界") && s.Characters[i].BarrierTurn == s.Turn {
		s.removeEffect(i, "結界")
		return true
	}
	return false
}
