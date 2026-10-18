package account

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"kdoiro/server/internal/game"
)

func TestCreateLoginAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chars.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
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
	c.X, c.Y, c.Zeny = 160, 100, 1234
	c.Inventory = []game.InvItem{{Index: 0, ItemID: 1201, Amount: 1, Equip: game.SlotHandR}}
	if err := s.Save("Poring_1", c.CharData); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c, created, err = s2.Authenticate("Poring_1", "secret", tmpl)
	if err != nil || created || c.X != 160 || c.Y != 100 || c.Zeny != 1234 ||
		len(c.Inventory) != 1 || c.Inventory[0].Equip != game.SlotHandR || c.Stats[game.STR] != 1 {
		t.Fatalf("after reopen: %+v created=%v err=%v", c, created, err)
	}
}

func TestValidation(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "chars.json"))
	for _, name := range []string{"abc", "has space", "this_name_is_way_too_long", "名前です"} {
		if _, _, err := s.Authenticate(name, "secret", game.CharData{}); !errors.Is(err, ErrInvalidName) {
			t.Errorf("name %q: err=%v", name, err)
		}
	}
	if _, _, err := s.Authenticate("Valid", "abc", game.CharData{}); !errors.Is(err, ErrInvalidPassword) {
		t.Errorf("short password: err=%v", err)
	}
}

func TestLegacyCharacterGetsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chars.json")
	legacy := `[{"name":"OldTimer","password_hash":"eA==","job":0,"map":"morocc","x":1,"y":2}]`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := s.chars["oldtimer"]
	if c.BaseLevel != 1 || c.Stats[game.DEX] != 1 || c.StatusPoints != game.StartPoints || c.Zeny != game.StartZeny || c.X != 1 {
		t.Fatalf("defaults not applied: %+v", c.CharData)
	}
}
