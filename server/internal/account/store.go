// Package account stores characters (name + password) in a JSON file.
//
// One character per name; logging in with an unknown name creates it.
package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"kdoiro/server/internal/game"
)

var (
	ErrInvalidName     = errors.New("name must be 4-23 letters, digits or underscores")
	ErrInvalidPassword = errors.New("password must be 4-72 bytes")
	ErrWrongPassword   = errors.New("wrong password")
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_]{4,23}$`)

type Character struct {
	Name         string    `json:"name"`
	PasswordHash []byte    `json:"password_hash"`
	CreatedAt    time.Time `json:"created_at"`
	// Embedded so job/map/x/y stay top-level fields in the saved JSON.
	game.CharData
}

type Store struct {
	path  string
	mu    sync.Mutex
	chars map[string]*Character // keyed by lowercase name
}

// Open loads the store from path, starting empty if the file doesn't exist.
func Open(path string) (*Store, error) {
	s := &Store{path: path, chars: map[string]*Character{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*Character
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for _, c := range list {
		c.FillDefaults() // characters saved before stats/zeny/inventory existed
		s.chars[strings.ToLower(c.Name)] = c
	}
	return s, nil
}

// Authenticate logs into the named character, creating it with the given
// starting data if the name is free.
func (s *Store) Authenticate(name, password string, start game.CharData) (c Character, created bool, err error) {
	if !namePattern.MatchString(name) {
		return Character{}, false, ErrInvalidName
	}
	if len(password) < 4 || len(password) > 72 {
		return Character{}, false, ErrInvalidPassword
	}
	key := strings.ToLower(name)

	s.mu.Lock()
	existing, ok := s.chars[key]
	var snapshot Character
	if ok {
		snapshot = *existing
		snapshot.CharData = existing.Clone()
	}
	s.mu.Unlock()

	if ok {
		// bcrypt is deliberately slow; compare outside the lock.
		if bcrypt.CompareHashAndPassword(snapshot.PasswordHash, []byte(password)) != nil {
			return Character{}, false, ErrWrongPassword
		}
		return snapshot, false, nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Character{}, false, err
	}
	nc := Character{Name: name, PasswordHash: hash, CreatedAt: time.Now().UTC(), CharData: start}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, taken := s.chars[key]; taken {
		// Someone created the same name while we were hashing.
		return Character{}, false, ErrWrongPassword
	}
	s.chars[key] = &nc
	if err := s.saveLocked(); err != nil {
		delete(s.chars, key)
		return Character{}, false, err
	}
	return nc, true, nil
}

// Save records a character's state (position, stats, zeny, inventory).
func (s *Store) Save(name string, data game.CharData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.chars[strings.ToLower(name)]
	if !ok {
		return fmt.Errorf("unknown character %q", name)
	}
	c.CharData = data.Clone()
	return s.saveLocked()
}

// saveLocked writes the file atomically via a temp file + rename.
func (s *Store) saveLocked() error {
	list := make([]*Character, 0, len(s.chars))
	for _, c := range s.chars {
		list = append(list, c)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".characters-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}
