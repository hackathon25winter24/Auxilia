package game

func (s *State) pushBack(actor, target int, direction Position) {
	if abs(direction.X)+abs(direction.Y) != 1 {
		direction = Position{1, 0}
		if s.Characters[actor].OwnerID == s.Players[1].ID {
			direction = Position{-1, 0}
		}
	}
	c := &s.Characters[target]
	if s.immutableAt(c.Position) && !s.ignoresDebuffTiles(target) {
		return
	}
	for _, steps := range []int{2, 1} {
		p := Position{c.Position.X + direction.X*steps, c.Position.Y + direction.Y*steps}
		if !onBoard(p) || s.blocked(p) || s.immutableAt(p) || s.occupied(p, c.ID) || s.enemyBaseAt(c.OwnerID, p) {
			continue
		}
		c.Position = p
		s.triggerTile(target)
		return
	}
}
