package game

import "testing"

func TestCanonicalCharacterSpellings(t *testing.T) {
	for old, id := range map[string]string{"wellbulus": "verbulus", "shincho": "shicho"} {
		d, ok := Definition(old)
		if !ok || d.ID != id {
			t.Fatalf("legacy %s did not resolve", old)
		}
		count := 0
		for _, d := range Definitions {
			if d.ID == old {
				t.Fatal("legacy ID exposed")
			}
			if d.ID == id {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("%s occurs %d times", id, count)
		}
	}
}
