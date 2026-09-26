package game

// CanonicalCharacterID keeps saved selections, matches and usage history readable.
func CanonicalCharacterID(id string) string {
	switch id {
	case "wellbulus":
		return "verbulus"
	case "shincho":
		return "shicho"
	default:
		return id
	}
}
