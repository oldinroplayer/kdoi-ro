package account

import (
	"errors"
	"testing"
	"time"

	"kdoiro/server/internal/game"
)

func TestResumeToken(t *testing.T) {
	s, _ := openTemp(t)
	if _, _, err := s.Authenticate("Hero", "pass1234", game.NewChar("town", 3, 4)); err != nil {
		t.Fatal(err)
	}
	token, err := s.IssueResumeToken("Hero", time.Minute)
	if err != nil || len(token) != 64 {
		t.Fatalf("issue: %q %v", token, err)
	}
	c, err := s.Resume(token)
	if err != nil || c.Name != "Hero" || c.Map != "town" || c.X != 3 {
		t.Fatalf("resume: %+v %v", c, err)
	}
	if _, err := s.Resume(token); !errors.Is(err, ErrResumeFailed) {
		t.Fatalf("a token works once: %v", err)
	}
	if _, err := s.Resume("nonsense"); !errors.Is(err, ErrResumeFailed) {
		t.Fatalf("unknown token: %v", err)
	}
	old, err := s.IssueResumeToken("Hero", -time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resume(old); !errors.Is(err, ErrResumeFailed) {
		t.Fatalf("expired token: %v", err)
	}
}
