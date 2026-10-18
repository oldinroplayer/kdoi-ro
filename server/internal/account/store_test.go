package account

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestCreateLoginAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chars.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := Character{Job: 0, Map: "morocc", X: 156, Y: 93}

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
	if err := s.SavePosition("Poring_1", "morocc", 160, 100); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c, created, err = s2.Authenticate("Poring_1", "secret", tmpl)
	if err != nil || created || c.X != 160 || c.Y != 100 {
		t.Fatalf("after reopen: %+v created=%v err=%v", c, created, err)
	}
}

func TestValidation(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "chars.json"))
	for _, name := range []string{"abc", "has space", "this_name_is_way_too_long", "名前です"} {
		if _, _, err := s.Authenticate(name, "secret", Character{}); !errors.Is(err, ErrInvalidName) {
			t.Errorf("name %q: err=%v", name, err)
		}
	}
	if _, _, err := s.Authenticate("Valid", "abc", Character{}); !errors.Is(err, ErrInvalidPassword) {
		t.Errorf("short password: err=%v", err)
	}
}
