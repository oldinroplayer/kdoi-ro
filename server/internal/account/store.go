// Package account stores characters (name + password) in a SQLite database.
//
// One character per name; logging in with an unknown name creates it. The
// schema has a table per kind of data (characters, inventory, skills) and is
// versioned with PRAGMA user_version, so later changes can migrate it.
package account

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite" // pure-Go SQLite driver, registered as "sqlite"

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
	// Embedded so job/map/x/y are top-level fields in the old JSON format.
	game.CharData
}

type Store struct {
	db *sql.DB
}

// migrations[i] upgrades the schema from user_version i to i+1.
var migrations = []string{
	`CREATE TABLE characters (
		id            INTEGER PRIMARY KEY,
		name          TEXT    NOT NULL UNIQUE COLLATE NOCASE,
		password_hash BLOB    NOT NULL,
		created_at    TEXT    NOT NULL,
		job           INTEGER NOT NULL DEFAULT 0,
		map           TEXT    NOT NULL,
		x             INTEGER NOT NULL,
		y             INTEGER NOT NULL,
		base_level    INTEGER NOT NULL,
		job_level     INTEGER NOT NULL,
		base_exp      INTEGER NOT NULL DEFAULT 0,
		job_exp       INTEGER NOT NULL DEFAULT 0,
		hp            INTEGER NOT NULL,
		sp            INTEGER NOT NULL,
		str INTEGER NOT NULL, agi INTEGER NOT NULL, vit INTEGER NOT NULL,
		int INTEGER NOT NULL, dex INTEGER NOT NULL, luk INTEGER NOT NULL,
		status_points INTEGER NOT NULL DEFAULT 0,
		skill_points  INTEGER NOT NULL DEFAULT 0,
		zeny          INTEGER NOT NULL DEFAULT 0,
		save_map      TEXT    NOT NULL DEFAULT '',
		save_x        INTEGER NOT NULL DEFAULT 0,
		save_y        INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE inventory (
		char_id INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
		slot    INTEGER NOT NULL, -- the inventory index the client sees
		item_id INTEGER NOT NULL,
		amount  INTEGER NOT NULL,
		equip   INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (char_id, slot)
	);
	CREATE TABLE skills (
		char_id  INTEGER NOT NULL REFERENCES characters(id) ON DELETE CASCADE,
		skill_id INTEGER NOT NULL,
		level    INTEGER NOT NULL,
		PRIMARY KEY (char_id, skill_id)
	);`,
	`ALTER TABLE characters ADD COLUMN hair INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE characters ADD COLUMN hair_color INTEGER NOT NULL DEFAULT 0;`,
	`ALTER TABLE characters ADD COLUMN female INTEGER NOT NULL DEFAULT 0;`,
	`CREATE TABLE parties (
		id       INTEGER PRIMARY KEY,
		name     TEXT    NOT NULL UNIQUE COLLATE NOCASE,
		leader   TEXT    NOT NULL,
		even_exp INTEGER NOT NULL DEFAULT 0
	);
	CREATE TABLE party_members (
		party_id INTEGER NOT NULL REFERENCES parties(id) ON DELETE CASCADE,
		position INTEGER NOT NULL, -- joining order
		name     TEXT    NOT NULL UNIQUE COLLATE NOCASE,
		PRIMARY KEY (party_id, position)
	);`,
}

// Open opens (creating if needed) the database at path and brings its schema
// up to date.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	// WAL lets reads continue during a save; foreign keys clean up inventory
	// and skills with their character.
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: SQLite has a single writer anyway, and this avoids
	// SQLITE_BUSY between our own connections.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		if head, _ := os.ReadFile(path); len(head) > 0 && (head[0] == '[' || head[0] == '{') {
			return nil, fmt.Errorf("%s is the old JSON character file: pass it with -import-json and use a new -db path", path)
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	for ; version < len(migrations); version++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[version]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", version+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
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
	ctx := context.Background()

	id, existing, err := s.load(ctx, name)
	if err != nil {
		return Character{}, false, err
	}
	if id != 0 {
		if bcrypt.CompareHashAndPassword(existing.PasswordHash, []byte(password)) != nil {
			return Character{}, false, ErrWrongPassword
		}
		return existing, false, nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Character{}, false, err
	}
	nc := Character{Name: name, PasswordHash: hash, CreatedAt: time.Now().UTC(), CharData: start.Clone()}
	if err := s.insert(ctx, nc); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			// Someone created the same name while we were hashing.
			return Character{}, false, ErrWrongPassword
		}
		return Character{}, false, err
	}
	return nc, true, nil
}

const charColumns = `job, map, x, y, base_level, job_level, base_exp, job_exp, hp, sp,
	str, agi, vit, int, dex, luk, status_points, skill_points, zeny, save_map, save_x, save_y, hair, hair_color, female`

// columnValues lists a character's values in charColumns order.
func columnValues(c *game.CharData) []any {
	return []any{c.Job, c.Map, c.X, c.Y, c.BaseLevel, c.JobLevel, c.BaseExp, c.JobExp, c.HP, c.SP,
		c.Stats[game.STR], c.Stats[game.AGI], c.Stats[game.VIT], c.Stats[game.INT], c.Stats[game.DEX], c.Stats[game.LUK],
		c.StatusPoints, c.SkillPoints, c.Zeny, c.Save.Map, c.Save.X, c.Save.Y, c.Hair, c.HairColor, c.Female}
}

// load reads a character by name (any case); id 0 means there is none.
func (s *Store) load(ctx context.Context, name string) (id int64, c Character, err error) {
	d := &c.CharData
	var created string
	err = s.db.QueryRowContext(ctx, `SELECT id, name, password_hash, created_at, `+charColumns+`
		FROM characters WHERE name = ?`, name).Scan(
		&id, &c.Name, &c.PasswordHash, &created,
		&d.Job, &d.Map, &d.X, &d.Y, &d.BaseLevel, &d.JobLevel, &d.BaseExp, &d.JobExp, &d.HP, &d.SP,
		&d.Stats[game.STR], &d.Stats[game.AGI], &d.Stats[game.VIT], &d.Stats[game.INT], &d.Stats[game.DEX], &d.Stats[game.LUK],
		&d.StatusPoints, &d.SkillPoints, &d.Zeny, &d.Save.Map, &d.Save.X, &d.Save.Y, &d.Hair, &d.HairColor, &d.Female)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, Character{}, nil
	}
	if err != nil {
		return 0, Character{}, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)

	rows, err := s.db.QueryContext(ctx, `SELECT slot, item_id, amount, equip FROM inventory WHERE char_id = ? ORDER BY slot`, id)
	if err != nil {
		return 0, Character{}, err
	}
	for rows.Next() {
		var it game.InvItem
		if err := rows.Scan(&it.Index, &it.ItemID, &it.Amount, &it.Equip); err != nil {
			rows.Close()
			return 0, Character{}, err
		}
		d.Inventory = append(d.Inventory, it)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, Character{}, err
	}

	rows, err = s.db.QueryContext(ctx, `SELECT skill_id, level FROM skills WHERE char_id = ?`, id)
	if err != nil {
		return 0, Character{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var skill, level int
		if err := rows.Scan(&skill, &level); err != nil {
			return 0, Character{}, err
		}
		if d.Skills == nil {
			d.Skills = map[int]int{}
		}
		d.Skills[skill] = level
	}
	d.FillDefaults() // e.g. skills for characters imported from before they existed
	return id, c, rows.Err()
}

func (s *Store) insert(ctx context.Context, c Character) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO characters (name, password_hash, created_at, `+charColumns+`)
		VALUES (?, ?, ?, `+strings.Repeat("?, ", strings.Count(charColumns, ","))+`?)`,
		append([]any{c.Name, c.PasswordHash, c.CreatedAt.Format(time.RFC3339Nano)}, columnValues(&c.CharData)...)...)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if err := writeChildren(ctx, tx, id, &c.CharData); err != nil {
		return err
	}
	return tx.Commit()
}

// writeChildren replaces a character's inventory and skill rows.
func writeChildren(ctx context.Context, tx *sql.Tx, id int64, c *game.CharData) error {
	for _, q := range []string{`DELETE FROM inventory WHERE char_id = ?`, `DELETE FROM skills WHERE char_id = ?`} {
		if _, err := tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	for _, it := range c.Inventory {
		if _, err := tx.ExecContext(ctx, `INSERT INTO inventory (char_id, slot, item_id, amount, equip) VALUES (?, ?, ?, ?, ?)`,
			id, it.Index, it.ItemID, it.Amount, it.Equip); err != nil {
			return err
		}
	}
	for skill, level := range c.Skills {
		if _, err := tx.ExecContext(ctx, `INSERT INTO skills (char_id, skill_id, level) VALUES (?, ?, ?)`, id, skill, level); err != nil {
			return err
		}
	}
	return nil
}

// Save records a character's state (position, stats, zeny, inventory, skills)
// in one transaction.
func (s *Store) Save(name string, data game.CharData) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM characters WHERE name = ?`, name).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("unknown character %q", name)
		}
		return err
	}
	sets := strings.Join(strings.Fields(strings.NewReplacer(",", " ").Replace(charColumns)), " = ?, ") + " = ?"
	if _, err := tx.ExecContext(ctx, `UPDATE characters SET `+sets+` WHERE id = ?`, append(columnValues(&data), id)...); err != nil {
		return err
	}
	if err := writeChildren(ctx, tx, id, &data); err != nil {
		return err
	}
	return tx.Commit()
}

// ImportJSON copies characters from the old JSON store into an empty
// database, returning how many it imported (0 if the database already has
// characters). Passwords keep their bcrypt hashes.
func (s *Store) ImportJSON(path string) (int, error) {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM characters`).Scan(&count); err != nil || count > 0 {
		return 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	var list []Character
	if err := json.Unmarshal(data, &list); err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	ctx := context.Background()
	for _, c := range list {
		c.FillDefaults() // characters saved before stats/zeny/inventory existed
		if c.CreatedAt.IsZero() {
			c.CreatedAt = time.Now().UTC()
		}
		if err := s.insert(ctx, c); err != nil {
			return 0, fmt.Errorf("importing %s: %w", c.Name, err)
		}
	}
	return len(list), nil
}

// Party is a party as stored: its members are character names.
type Party struct {
	ID      uint32
	Name    string
	Leader  string
	Members []string
	EvenEXP bool
}

// Parties loads every party.
func (s *Store) Parties() ([]Party, error) {
	rows, err := s.db.Query(`SELECT p.id, p.name, p.leader, p.even_exp, m.name
		FROM parties p JOIN party_members m ON m.party_id = p.id ORDER BY p.id, m.position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Party
	for rows.Next() {
		var pt Party
		var member string
		if err := rows.Scan(&pt.ID, &pt.Name, &pt.Leader, &pt.EvenEXP, &member); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].ID != pt.ID {
			out = append(out, pt)
		}
		last := &out[len(out)-1]
		last.Members = append(last.Members, member)
	}
	return out, rows.Err()
}

// SaveParty creates or replaces a party and its member list.
func (s *Store) SaveParty(pt Party) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO parties (id, name, leader, even_exp) VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, leader = excluded.leader, even_exp = excluded.even_exp`,
		pt.ID, pt.Name, pt.Leader, pt.EvenEXP); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM party_members WHERE party_id = ?`, pt.ID); err != nil {
		return err
	}
	for i, m := range pt.Members {
		if _, err := tx.Exec(`INSERT INTO party_members (party_id, position, name) VALUES (?, ?, ?)`, pt.ID, i, m); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteParty removes a disbanded party.
func (s *Store) DeleteParty(id uint32) error {
	_, err := s.db.Exec(`DELETE FROM parties WHERE id = ?`, id)
	return err
}

// RelocateAll moves every saved character to a map cell (they start there at
// their next login). It returns how many characters were updated.
func (s *Store) RelocateAll(mapName string, x, y int) (int64, error) {
	res, err := s.db.Exec(`UPDATE characters SET map = ?, x = ?, y = ?`, mapName, x, y)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SaveAll sets every saved character's respawn point.
func (s *Store) SaveAll(mapName string, x, y int) (int64, error) {
	res, err := s.db.Exec(`UPDATE characters SET save_map = ?, save_x = ?, save_y = ?`, mapName, x, y)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
