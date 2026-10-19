package account

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"kdoiro/server/internal/game"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kdoiro.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

func TestCreateLoginAndPersist(t *testing.T) {
	s, path := openTemp(t)
	tmpl := game.NewChar("morocc", 156, 93)

	c, created, err := s.Authenticate("Poring_1", "secret", tmpl)
	if err != nil || !created || c.Name != "Poring_1" || c.Map != "morocc" {
		t.Fatalf("create: %+v created=%v err=%v", c, created, err)
	}
	if _, created, err := s.Authenticate("poring_1", "secret", tmpl); err != nil || created {
		t.Fatalf("login (case-insensitive): created=%v err=%v", created, err)
	}
	if _, _, err := s.Authenticate("Poring_1", "nope!", tmpl); !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("wrong password: err=%v", err)
	}
	c.Map, c.X, c.Y, c.Zeny, c.Job, c.JobLevel = "payon", 160, 100, 1234, game.JobThief, 7
	c.Stats[game.AGI] = 42
	c.Inventory = []game.InvItem{{Index: 0, ItemID: 1201, Amount: 1, Equip: game.SlotHandR}, {Index: 3, ItemID: 501, Amount: 12}}
	c.Skills = map[int]int{game.SkillFirstAid: 1, game.SkillDoubleAttack: 4}
	c.SkillPoints = 2
	c.Save = game.SavePoint{Map: "payon", X: 163, Y: 97}
	if err := s.Save("POring_1", c.CharData); err != nil {
		t.Fatal(err)
	}
	// Saving again replaces the inventory rather than adding to it, and keeps
	// cards compounded into equipment.
	c.Inventory = append(c.Inventory[1:], game.InvItem{Index: 5, ItemID: 1201, Amount: 1, Cards: []int{4001, 4002}},
		game.InvItem{Index: 6, ItemID: 1207, Amount: 1, Element: game.ElemFire})
	if err := s.Save("Poring_1", c.CharData); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, created, err := s2.Authenticate("Poring_1", "secret", tmpl)
	if err != nil || created {
		t.Fatalf("after reopen: created=%v err=%v", created, err)
	}
	if got.Map != "payon" || got.X != 160 || got.Zeny != 1234 || got.Job != game.JobThief || got.JobLevel != 7 ||
		got.Stats[game.AGI] != 42 || got.Stats[game.STR] != 1 || got.SkillPoints != 2 ||
		got.Save != c.Save || got.CreatedAt.IsZero() {
		t.Fatalf("after reopen: %+v", got)
	}
	want := []game.InvItem{{Index: 3, ItemID: 501, Amount: 12}, {Index: 5, ItemID: 1201, Amount: 1, Cards: []int{4001, 4002}},
		{Index: 6, ItemID: 1207, Amount: 1, Element: game.ElemFire}}
	if !reflect.DeepEqual(got.Inventory, want) {
		t.Fatalf("inventory: %+v", got.Inventory)
	}
	if len(got.Skills) != 2 || got.Skills[game.SkillDoubleAttack] != 4 {
		t.Fatalf("skills: %+v", got.Skills)
	}
}

func TestValidation(t *testing.T) {
	s, _ := openTemp(t)
	for _, name := range []string{"abc", "has space", "this_name_is_way_too_long", "名前です"} {
		if _, _, err := s.Authenticate(name, "secret", game.CharData{}); !errors.Is(err, ErrInvalidName) {
			t.Errorf("name %q: err=%v", name, err)
		}
	}
	if _, _, err := s.Authenticate("Valid", "abc", game.CharData{}); !errors.Is(err, ErrInvalidPassword) {
		t.Errorf("short password: err=%v", err)
	}
	if err := s.Save("Nobody", game.CharData{}); err == nil {
		t.Error("saving an unknown character should fail")
	}
}

func TestConcurrentCreateOfOneName(t *testing.T) {
	s, _ := openTemp(t)
	var wg sync.WaitGroup
	results := make([]bool, 4)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, created, err := s.Authenticate("Racer", "secret", game.NewChar("morocc", 1, 1))
			results[i] = err == nil && created
		}()
	}
	wg.Wait()
	n := 0
	for _, r := range results {
		if r {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d creations of one name succeeded", n)
	}
}

func TestImportJSON(t *testing.T) {
	s, _ := openTemp(t)
	// The JSON store kept bcrypt hashes (base64 in JSON); they carry over.
	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	legacy := `[{"name":"OldTimer","password_hash":"` + base64.StdEncoding.EncodeToString(hash) + `","job":0,"map":"morocc","x":1,"y":2},
		{"name":"Stocked","password_hash":"eA==","map":"moc_fild07","x":5,"y":6,"base_level":3,"job_level":2,
		 "stats":[2,3,4,5,6,7],"zeny":99,"skills":{"1":2,"142":1},"inventory":[{"index":0,"item_id":501,"amount":3}]}]`
	path := filepath.Join(t.TempDir(), "characters.json")
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	if n, err := s.ImportJSON(path); err != nil || n != 2 {
		t.Fatalf("import: n=%d err=%v", n, err)
	}
	c, created, err := s.Authenticate("oldtimer", "secret", game.CharData{})
	if err != nil || created {
		t.Fatalf("imported login: created=%v err=%v", created, err)
	}
	// Characters saved before stats existed get the defaults.
	if c.BaseLevel != 1 || c.Stats[game.DEX] != 1 || c.StatusPoints != game.StartPoints || c.Zeny != game.StartZeny || c.X != 1 {
		t.Fatalf("defaults not applied: %+v", c.CharData)
	}
	_, s2, err := s.load(t.Context(), "Stocked")
	if err != nil || s2.Zeny != 99 || s2.Stats[game.LUK] != 7 || s2.Skills[1] != 2 || len(s2.Inventory) != 1 || s2.Map != "moc_fild07" {
		t.Fatalf("stocked character: %+v err=%v", s2.CharData, err)
	}
	// A database that already has characters is never overwritten.
	if n, err := s.ImportJSON(path); err != nil || n != 0 {
		t.Fatalf("second import: n=%d err=%v", n, err)
	}
}

func TestHairIsSaved(t *testing.T) {
	s, path := openTemp(t)
	c, _, err := s.Authenticate("Barber", "secret", game.NewChar("morocc", 1, 1))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetAppearance(12, 4, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("Barber", c.CharData); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, _, err := s2.Authenticate("Barber", "secret", game.CharData{})
	if err != nil || got.Hair != 12 || got.HairColor != 4 || !got.Female {
		t.Fatalf("appearance after reopen: %d/%d female=%v err %v", got.Hair, got.HairColor, got.Female, err)
	}
}

func TestParties(t *testing.T) {
	s, path := openTemp(t)
	if err := s.SaveParty(Party{ID: 3, Name: "Porings", Leader: "Alice", Members: []string{"Alice", "Bob"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveParty(Party{ID: 3, Name: "Porings", Leader: "Bob", Members: []string{"Bob", "Carol"}, EvenEXP: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveParty(Party{ID: 4, Name: "Gone", Leader: "Dan", Members: []string{"Dan"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteParty(4); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.Parties()
	if err != nil || len(got) != 1 {
		t.Fatalf("parties: %+v err %v", got, err)
	}
	if p := got[0]; p.ID != 3 || p.Leader != "Bob" || !p.EvenEXP || len(p.Members) != 2 || p.Members[1] != "Carol" {
		t.Fatalf("party after reopen: %+v", p)
	}
}

func TestRelocateAll(t *testing.T) {
	s, _ := openTemp(t)
	for _, name := range []string{"Alpha", "Bravo"} {
		if _, _, err := s.Authenticate(name, "secret", game.NewChar("morocc", 1, 1)); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := s.RelocateAll("payon", 100, 100); err != nil || n != 2 {
		t.Fatalf("relocate: %d %v", n, err)
	}
	c, _, err := s.Authenticate("Bravo", "secret", game.CharData{})
	if err != nil || c.Map != "payon" || c.X != 100 || c.Y != 100 {
		t.Fatalf("after relocate: %s (%d, %d) %v", c.Map, c.X, c.Y, err)
	}
}

func TestSaveAllStored(t *testing.T) {
	s, _ := openTemp(t)
	if _, _, err := s.Authenticate("Respawner", "secret", game.NewChar("morocc", 1, 1)); err != nil {
		t.Fatal(err)
	}
	if n, err := s.SaveAll("pay_arche", 46, 131); err != nil || n != 1 {
		t.Fatalf("saveall: %d %v", n, err)
	}
	c, _, err := s.Authenticate("Respawner", "secret", game.CharData{})
	if err != nil || c.Save != (game.SavePoint{Map: "pay_arche", X: 46, Y: 131}) || c.Map != "morocc" {
		t.Fatalf("after saveall: save %+v, map %s, err %v", c.Save, c.Map, err)
	}
}
