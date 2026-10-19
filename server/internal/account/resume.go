package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

// ErrResumeFailed is returned for a resume token that is unknown, already
// used or expired.
var ErrResumeFailed = errors.New("resume token expired or unknown")

// IssueResumeToken lets a player back in as the named character without the
// password, once and within ttl: handed out when the server restarts for an
// update, so the page can reconnect by itself. Only a hash is stored.
func (s *Store) IssueResumeToken(name string, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := hex.EncodeToString(raw)
	now := time.Now()
	ctx := context.Background()
	// Expired tokens are cleared as new ones are issued.
	if _, err := s.db.ExecContext(ctx, `DELETE FROM resume_tokens WHERE expires < ?`, now.Unix()); err != nil {
		return "", err
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO resume_tokens (token_hash, name, expires) VALUES (?, ?, ?)`,
		hashToken(token), name, now.Add(ttl).Unix())
	if err != nil {
		return "", err
	}
	return token, nil
}

// Resume spends a resume token and returns its character.
func (s *Store) Resume(token string) (Character, error) {
	ctx := context.Background()
	var name string
	err := s.db.QueryRowContext(ctx, `DELETE FROM resume_tokens WHERE token_hash = ? AND expires >= ? RETURNING name`,
		hashToken(token), time.Now().Unix()).Scan(&name)
	if err != nil {
		return Character{}, ErrResumeFailed
	}
	id, c, err := s.load(ctx, name)
	if err != nil {
		return Character{}, err
	}
	if id == 0 {
		return Character{}, ErrResumeFailed
	}
	return c, nil
}

// Load reads a character by name as last saved (for a login that takes over
// a session: the old one saves first, then this picks up where it left off).
func (s *Store) Load(name string) (Character, error) {
	id, c, err := s.load(context.Background(), name)
	if err != nil {
		return Character{}, err
	}
	if id == 0 {
		return Character{}, ErrResumeFailed
	}
	return c, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
