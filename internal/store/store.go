// Package store keeps habit days and the goals copied from Notion in SQLite.
//
// n8n logs habit days and copies goals in; the app corrects days by hand.
// The store validates and records what it is sent; it holds no habit rules.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
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
	// Single user, light writes: one connection avoids SQLITE_BUSY.
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

// The database enforces what it can on its own (types, formats, ranges), so a
// bad write is refused even if it gets past the Go checks. Rules that need
// another table, such as "only plannable habits can be planned or missed",
// live in PutDay.
var migrations = []string{
	`CREATE TABLE habits (
		key       TEXT PRIMARY KEY
		          CHECK (length(key) BETWEEN 1 AND 32 AND key GLOB '[a-z]*' AND key NOT GLOB '*[^a-z0-9_]*'),
		name      TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 40),
		position  INTEGER NOT NULL,
		plannable INTEGER NOT NULL CHECK (plannable IN (0, 1)),
		-- Archived habits leave the screen but keep their history.
		archived  INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1))
	) STRICT;
	INSERT INTO habits (key, name, position, plannable) VALUES
		('workout', 'Workout', 1, 1),
		('dance',   'Dance',   2, 1),
		('writing', 'Writing', 3, 0);

	-- One row per habit per tracker day that has a status; a blank day has no row.
	-- A habit with history can't be deleted (archive it instead).
	CREATE TABLE habit_days (
		date       TEXT NOT NULL CHECK (date(date) IS date),
		habit      TEXT NOT NULL REFERENCES habits(key) ON DELETE RESTRICT,
		status     TEXT NOT NULL CHECK (status IN ('planned', 'done', 'missed')),
		-- Who set it: 'app' (a correction made in the PWA) or 'n8n' (an automation).
		source     TEXT NOT NULL CHECK (source IN ('app', 'n8n')),
		updated_at INTEGER NOT NULL CHECK (updated_at > 0), -- Unix ms
		PRIMARY KEY (date, habit)
	) STRICT;

	-- A copy of the Notion goal databases, replaced on every sync.
	CREATE TABLE goals (
		id        TEXT PRIMARY KEY CHECK (length(id) BETWEEN 1 AND 64), -- Notion page ID
		list      TEXT NOT NULL CHECK (list IN ('short_term', 'perennial')),
		name      TEXT NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 500),
		type      TEXT CHECK (length(type) <= 40),
		priority  INTEGER CHECK (priority IN (1, 2, 3)), -- 1 high, 2 mid, 3 low
		progress  REAL CHECK (progress BETWEEN 0 AND 1),
		due       TEXT CHECK (date(due) IS due),
		url       TEXT NOT NULL CHECK (url LIKE 'https://%' AND length(url) <= 1000),
		synced_at INTEGER NOT NULL CHECK (synced_at > 0) -- Unix ms
	) STRICT;`,
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
	Archived  bool   `json:"archived"` // hidden from the dashboard, history kept
}

func (s *Store) Habits(ctx context.Context) ([]Habit, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT key, name, position, plannable, archived FROM habits ORDER BY position, key")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	habits := []Habit{}
	for rows.Next() {
		var h Habit
		if err := rows.Scan(&h.Key, &h.Name, &h.Position, &h.Plannable, &h.Archived); err != nil {
			return nil, err
		}
		habits = append(habits, h)
	}
	return habits, rows.Err()
}

// PutHabit creates or updates a habit. Plannable habits can be planned and
// missed and count toward adherence; the others only ever get done. Habits
// are never deleted: archive one to take it off the dashboard.
func (s *Store) PutHabit(ctx context.Context, h Habit) error {
	if !keyRe.MatchString(h.Key) {
		return invalid("habit key must be lowercase letters, digits or _, starting with a letter")
	}
	h.Name = strings.TrimSpace(h.Name)
	if h.Name == "" || utf8.RuneCountInString(h.Name) > 40 {
		return invalid("habit name must be 1-40 characters")
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO habits (key, name, position, plannable, archived) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET name = excluded.name, position = excluded.position,
			plannable = excluded.plannable, archived = excluded.archived`,
		h.Key, h.Name, h.Position, h.Plannable, h.Archived)
	return err
}

// ---------- habit days ----------

// Day is one habit on one tracker day. A nil Status means nothing is logged.
type Day struct {
	Date   string  `json:"date"`
	Status *string `json:"status"`
	Source *string `json:"source"` // who set the status: SourceApp or SourceN8n
}

// Who set a day's status.
const (
	SourceApp = "app" // a correction made in the PWA
	SourceN8n = "n8n" // an automation
)

// Blank stands for "no status" in PutDay conditions.
const Blank = "blank"

// PutDay sets one habit's status for one tracker day, recording who set it;
// a nil status clears it.
//
// When onlyIf is not empty the write happens only if the current status is
// one of them (Blank for none), checked and written atomically. n8n uses it so
// automations never undo what was logged by hand: plan only blank days, close
// out only planned ones. It reports whether the write happened and the status
// the day has afterwards.
func (s *Store) PutDay(ctx context.Context, habit string, d Day, source string, onlyIf ...string) (applied bool, current *string, err error) {
	if err := checkDate(d.Date); err != nil {
		return false, nil, err
	}
	if source != SourceApp && source != SourceN8n {
		return false, nil, invalid("source must be app or n8n")
	}
	if d.Status != nil && !isStatus(*d.Status) {
		return false, nil, invalid("status must be planned, done, missed or null")
	}
	for _, c := range onlyIf {
		if c != Blank && !isStatus(c) {
			return false, nil, invalid("condition must be blank, planned, done or missed")
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var plannable bool
	err = tx.QueryRowContext(ctx, "SELECT plannable FROM habits WHERE key = ?", habit).Scan(&plannable)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil, ErrNotFound
	}
	if err != nil {
		return false, nil, err
	}
	if !plannable && d.Status != nil && *d.Status != StatusDone {
		return false, nil, invalid("%s is not plannable: status must be done or null", habit)
	}

	err = tx.QueryRowContext(ctx, "SELECT status FROM habit_days WHERE date = ? AND habit = ?", d.Date, habit).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, nil, err
	}
	if len(onlyIf) > 0 {
		have := Blank
		if current != nil {
			have = *current
		}
		if !slices.Contains(onlyIf, have) {
			return false, current, nil
		}
	}

	if d.Status == nil {
		_, err = tx.ExecContext(ctx, "DELETE FROM habit_days WHERE date = ? AND habit = ?", d.Date, habit)
	} else {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO habit_days (date, habit, status, source, updated_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (date, habit) DO UPDATE SET
				status = excluded.status, source = excluded.source, updated_at = excluded.updated_at`,
			d.Date, habit, *d.Status, source, s.nowMs())
	}
	if err != nil {
		return false, nil, err
	}
	return true, d.Status, tx.Commit()
}

// DayEntry is a logged habit day, for listing across habits.
type DayEntry struct {
	Date   string `json:"date"`
	Habit  string `json:"habit"`
	Status string `json:"status"`
}

// Days lists logged days with the given status, oldest first. A non-empty
// before keeps only days strictly before it (n8n's 05:00 close-out asks for
// days still planned before today).
func (s *Store) Days(ctx context.Context, status, before string) ([]DayEntry, error) {
	if !isStatus(status) {
		return nil, invalid("status must be planned, done or missed")
	}
	if before == "" {
		before = "9999-12-31"
	} else if err := checkDate(before); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.date, d.habit, d.status FROM habit_days d JOIN habits h ON h.key = d.habit
		WHERE d.status = ? AND d.date < ? ORDER BY d.date, h.position, d.habit`, status, before)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	days := []DayEntry{}
	for rows.Next() {
		var e DayEntry
		if err := rows.Scan(&e.Date, &e.Habit, &e.Status); err != nil {
			return nil, err
		}
		days = append(days, e)
	}
	return days, rows.Err()
}

func isStatus(s string) bool {
	return s == StatusPlanned || s == StatusDone || s == StatusMissed
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
