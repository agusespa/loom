// Package store keeps habit days and the goals copied from Notion in SQLite.
//
// n8n is the only writer: it decides each habit's status and copies goals in.
// The store validates and records what it is sent; it holds no habit rules.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// ValidationError means the caller sent something malformed.
type ValidationError struct{ msg string }

func (e *ValidationError) Error() string { return e.msg }

func invalid(format string, args ...any) error {
	return &ValidationError{fmt.Sprintf(format, args...)}
}

// ErrNotFound means the named habit does not exist.
var ErrNotFound = errors.New("not found")

const (
	StatusPlanned = "planned"
	StatusDone    = "done"
	StatusMissed  = "missed"

	ListShortTerm = "short_term"
	ListPerennial = "perennial"

	maxText = 500
)

var (
	dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	keyRe  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
)

type Store struct {
	db  *sql.DB
	now func() time.Time // for tests; nil means time.Now
}

// Open opens (creating if needed) the database at path and applies migrations.
func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Single user and a single writer (n8n): one connection avoids SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) nowMs() int64 {
	if s.now != nil {
		return s.now().UnixMilli()
	}
	return time.Now().UnixMilli()
}

// Ready checks that the schema is there and the database accepts writes,
// without committing anything.
func (s *Store) Ready(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, "UPDATE habits SET position = position WHERE 0")
	return err
}

var migrations = []string{
	`CREATE TABLE habits (
		key       TEXT PRIMARY KEY,
		name      TEXT NOT NULL,
		position  INTEGER NOT NULL,
		plannable INTEGER NOT NULL
	);
	INSERT INTO habits (key, name, position, plannable) VALUES
		('workout',    'Workout',    1, 1),
		('dance',      'Dance',      2, 1),
		('meditation', 'Meditation', 3, 0),
		('writing',    'Writing',    4, 0);
	CREATE TABLE habit_days (
		date       TEXT NOT NULL,
		habit      TEXT NOT NULL REFERENCES habits(key) ON DELETE CASCADE,
		status     TEXT CHECK (status IN ('planned', 'done', 'missed')),
		value      REAL,
		note       TEXT,
		updated_at INTEGER NOT NULL,
		PRIMARY KEY (date, habit)
	);
	CREATE TABLE goals (
		id        TEXT PRIMARY KEY,
		list      TEXT NOT NULL CHECK (list IN ('short_term', 'perennial')),
		name      TEXT NOT NULL,
		type      TEXT,
		priority  INTEGER,
		progress  REAL,
		due       TEXT,
		url       TEXT NOT NULL,
		synced_at INTEGER NOT NULL
	);`,
}

func (s *Store) migrate() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("database schema version %d is newer than this build supports (%d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// ---------- habits ----------

type Habit struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Position  int    `json:"position"`
	Plannable bool   `json:"plannable"`
}

func (s *Store) Habits(ctx context.Context) ([]Habit, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, name, position, plannable FROM habits ORDER BY position, key")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	habits := []Habit{}
	for rows.Next() {
		var h Habit
		if err := rows.Scan(&h.Key, &h.Name, &h.Position, &h.Plannable); err != nil {
			return nil, err
		}
		habits = append(habits, h)
	}
	return habits, rows.Err()
}

// PutHabit creates or updates a habit. Plannable habits can be planned and
// missed and count toward adherence; the others only ever get done.
func (s *Store) PutHabit(ctx context.Context, h Habit) error {
	if !keyRe.MatchString(h.Key) {
		return invalid("habit key must be lowercase letters, digits or _, starting with a letter")
	}
	h.Name = strings.TrimSpace(h.Name)
	if h.Name == "" || utf8.RuneCountInString(h.Name) > 40 {
		return invalid("habit name must be 1-40 characters")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO habits (key, name, position, plannable) VALUES (?, ?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET name = excluded.name, position = excluded.position, plannable = excluded.plannable`,
		h.Key, h.Name, h.Position, h.Plannable)
	return err
}

// ---------- habit days ----------

// Day is one habit on one tracker day. A nil Status means nothing was planned
// or done; the row may still carry a value (e.g. mindful minutes below the bar).
type Day struct {
	Date   string   `json:"date"`
	Status *string  `json:"status"`
	Value  *float64 `json:"value"`
	Note   *string  `json:"note"`
}

// PutDay records one habit's state for one tracker day. When status, value
// and note are all empty the row is removed.
func (s *Store) PutDay(ctx context.Context, habit string, d Day) error {
	if err := checkDate(d.Date); err != nil {
		return err
	}
	if d.Status != nil {
		switch *d.Status {
		case StatusPlanned, StatusDone, StatusMissed:
		default:
			return invalid("status must be planned, done, missed or null")
		}
	}
	if d.Value != nil && (math.IsNaN(*d.Value) || math.IsInf(*d.Value, 0)) {
		return invalid("value must be a finite number")
	}
	if d.Note != nil {
		if utf8.RuneCountInString(*d.Note) > maxText {
			return invalid("note is longer than %d characters", maxText)
		}
		if *d.Note == "" {
			d.Note = nil
		}
	}

	var plannable bool
	err := s.db.QueryRowContext(ctx, "SELECT plannable FROM habits WHERE key = ?", habit).Scan(&plannable)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !plannable && d.Status != nil && *d.Status != StatusDone {
		return invalid("%s is not plannable: status must be done or null", habit)
	}

	if d.Status == nil && d.Value == nil && d.Note == nil {
		_, err = s.db.ExecContext(ctx, "DELETE FROM habit_days WHERE date = ? AND habit = ?", d.Date, habit)
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO habit_days (date, habit, status, value, note, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (date, habit) DO UPDATE SET
			status = excluded.status, value = excluded.value, note = excluded.note, updated_at = excluded.updated_at`,
		d.Date, habit, d.Status, d.Value, d.Note, s.nowMs())
	return err
}

// DeleteDay removes one habit's state for one tracker day.
func (s *Store) DeleteDay(ctx context.Context, habit, date string) error {
	if err := checkDate(date); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "DELETE FROM habit_days WHERE date = ? AND habit = ?", date, habit)
	return err
}

func checkDate(date string) error {
	if !dateRe.MatchString(date) {
		return invalid("date must be YYYY-MM-DD")
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return invalid("date %q is not a real date", date)
	}
	return nil
}

// ---------- goals ----------

// Goal is a read-only copy of a goal page in Notion. ID is the Notion page ID.
type Goal struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Type     *string  `json:"type"`
	Priority *string  `json:"priority"` // high, mid or low
	Progress *float64 `json:"progress"` // 0-1, as entered in Notion
	Due      *string  `json:"due"`
	URL      string   `json:"url"`
}

var priorities = map[string]int{"high": 1, "mid": 2, "low": 3}

// ReplaceGoals makes list hold exactly goals. Each list is replaced on its
// own, so a failed copy of one Notion database never empties the other.
func (s *Store) ReplaceGoals(ctx context.Context, list string, goals []Goal) error {
	if list != ListShortTerm && list != ListPerennial {
		return invalid("list must be short_term or perennial")
	}
	if len(goals) > 500 {
		return invalid("too many goals")
	}
	seen := map[string]bool{}
	for i, g := range goals {
		if g.ID == "" || len(g.ID) > 64 {
			return invalid("goal %d: id is required", i)
		}
		if seen[g.ID] {
			return invalid("goal %d: duplicate id %s", i, g.ID)
		}
		seen[g.ID] = true
		if strings.TrimSpace(g.Name) == "" || utf8.RuneCountInString(g.Name) > maxText {
			return invalid("goal %d: name must be 1-%d characters", i, maxText)
		}
		if !strings.HasPrefix(g.URL, "https://") || len(g.URL) > 1000 {
			return invalid("goal %d: url must be an https link", i)
		}
		if g.Priority != nil {
			if _, ok := priorities[*g.Priority]; !ok {
				return invalid("goal %d: priority must be high, mid, low or null", i)
			}
		}
		if g.Progress != nil && (math.IsNaN(*g.Progress) || *g.Progress < 0 || *g.Progress > 1) {
			return invalid("goal %d: progress must be between 0 and 1", i)
		}
		if g.Due != nil {
			if err := checkDate(*g.Due); err != nil {
				return invalid("goal %d: due must be YYYY-MM-DD", i)
			}
		}
		if g.Type != nil && utf8.RuneCountInString(*g.Type) > 40 {
			return invalid("goal %d: type is too long", i)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DELETE FROM goals WHERE list = ?", list); err != nil {
		return err
	}
	now := s.nowMs()
	for _, g := range goals {
		var prio *int
		if g.Priority != nil {
			p := priorities[*g.Priority]
			prio = &p
		}
		// ON CONFLICT: a goal moved between Notion databases keeps its page ID.
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO goals (id, list, name, type, priority, progress, due, url, synced_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET list = excluded.list, name = excluded.name, type = excluded.type,
				priority = excluded.priority, progress = excluded.progress, due = excluded.due,
				url = excluded.url, synced_at = excluded.synced_at`,
			g.ID, list, strings.TrimSpace(g.Name), g.Type, prio, g.Progress, g.Due, g.URL, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Goals returns one list ordered like the Notion views: priority, then
// due date, then least progress first.
func (s *Store) Goals(ctx context.Context, list string) ([]Goal, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, type, priority, progress, due, url FROM goals WHERE list = ?
		ORDER BY priority IS NULL, priority, due IS NULL, due, progress, name`, list)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	names := map[int]string{1: "high", 2: "mid", 3: "low"}
	goals := []Goal{}
	for rows.Next() {
		var g Goal
		var prio *int
		if err := rows.Scan(&g.ID, &g.Name, &g.Type, &prio, &g.Progress, &g.Due, &g.URL); err != nil {
			return nil, err
		}
		if prio != nil {
			p := names[*prio]
			g.Priority = &p
		}
		goals = append(goals, g)
	}
	return goals, rows.Err()
}
