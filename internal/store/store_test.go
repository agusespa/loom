package store

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.UnixMilli(1_000) }
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func ptr[T any](v T) *T { return &v }

func monday(t *testing.T) time.Time {
	t.Helper()
	m, err := time.Parse(time.DateOnly, "2026-11-16")
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestReopenKeepsSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "loom.db")
	for range 2 {
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Ready(context.Background()); err != nil {
			t.Fatal(err)
		}
		_ = s.Close()
	}
}

func TestPutDayValidation(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		habit  string
		day    Day
		onlyIf []string
	}{
		{"bad date", "workout", Day{Date: "2026-13-01", Status: ptr(StatusDone)}, nil},
		{"bad format", "workout", Day{Date: "21/11/2026", Status: ptr(StatusDone)}, nil},
		{"bad status", "workout", Day{Date: "2026-11-16", Status: ptr("Done")}, nil},
		{"writing cannot be planned", "writing", Day{Date: "2026-11-16", Status: ptr(StatusPlanned)}, nil},
		{"writing cannot be missed", "writing", Day{Date: "2026-11-16", Status: ptr(StatusMissed)}, nil},
		{"bad condition", "workout", Day{Date: "2026-11-16", Status: ptr(StatusDone)}, []string{"empty"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var verr *ValidationError
			if _, _, err := s.PutDay(ctx, tc.habit, tc.day, SourceN8n, tc.onlyIf...); !errors.As(err, &verr) {
				t.Fatalf("want validation error, got %v", err)
			}
		})
	}
	if _, _, err := s.PutDay(ctx, "meditation", Day{Date: "2026-11-16", Status: ptr(StatusDone)}, SourceN8n); !errors.Is(err, ErrNotFound) {
		t.Fatalf("meditation: want ErrNotFound, got %v", err)
	}
}

func put(t *testing.T, s *Store, habit, date string, status *string, onlyIf ...string) (bool, *string) {
	t.Helper()
	applied, current, err := s.PutDay(context.Background(), habit, Day{Date: date, Status: status}, SourceN8n, onlyIf...)
	if err != nil {
		t.Fatal(err)
	}
	return applied, current
}

func TestWeek(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	put(t, s, "workout", "2026-11-16", ptr(StatusDone))
	put(t, s, "workout", "2026-11-18", ptr(StatusMissed))
	put(t, s, "workout", "2026-11-20", ptr(StatusDone))
	put(t, s, "dance", "2026-11-22", ptr(StatusPlanned))
	put(t, s, "writing", "2026-11-17", ptr(StatusDone))
	// Outside the week, and last week for the comparison.
	put(t, s, "workout", "2026-11-23", ptr(StatusDone))
	put(t, s, "workout", "2026-11-10", ptr(StatusMissed))
	put(t, s, "dance", "2026-11-12", ptr(StatusDone))

	w, err := s.Week(ctx, monday(t), monday(t).AddDate(0, 0, 6))
	if err != nil {
		t.Fatal(err)
	}
	if w.Start != "2026-11-16" || w.End != "2026-11-22" {
		t.Fatalf("range %s..%s", w.Start, w.End)
	}
	if len(w.Habits) != 3 || w.Habits[0].Key != "workout" {
		t.Fatalf("habits: %+v", w.Habits)
	}
	workout := w.Habits[0]
	if *workout.Days[2].Status != StatusMissed || workout.Days[1].Status != nil {
		t.Fatalf("workout: %+v", workout)
	}
	if writing := w.Habits[2]; *writing.Days[1].Status != StatusDone {
		t.Fatalf("writing: %+v", writing)
	}
	// Only plannable habits count: 2 done, 1 missed, 1 planned.
	if w.Summary.Done != 2 || w.Summary.Missed != 1 || w.Summary.Planned != 1 {
		t.Fatalf("summary: %+v", w.Summary)
	}
	if a := *w.Summary.Adherence; a < 0.66 || a > 0.67 {
		t.Fatalf("adherence %v", a)
	}
	if w.PrevAdherence == nil || *w.PrevAdherence != 0.5 {
		t.Fatalf("prev adherence %v", w.PrevAdherence)
	}
	if w.UpdatedAt == nil || *w.UpdatedAt != 1_000 {
		t.Fatalf("updated_at %v", w.UpdatedAt)
	}
}

func TestNullStatusClearsDay(t *testing.T) {
	s := open(t)
	put(t, s, "workout", "2026-11-16", ptr(StatusPlanned))
	put(t, s, "workout", "2026-11-16", nil)
	w, err := s.Week(context.Background(), monday(t), monday(t).AddDate(0, 0, 6))
	if err != nil {
		t.Fatal(err)
	}
	if w.Habits[0].Days[0].Status != nil || w.Summary.Planned != 0 || w.Summary.Adherence != nil {
		t.Fatalf("row not removed: %+v %+v", w.Habits[0].Days[0], w.Summary)
	}
}

func TestConditionalPut(t *testing.T) {
	s := open(t)
	day := "2026-11-16"

	// 05:00 planning only fills a blank day.
	if ok, cur := put(t, s, "workout", day, ptr(StatusPlanned), Blank); !ok || *cur != StatusPlanned {
		t.Fatalf("plan blank: %v %v", ok, cur)
	}
	// Logged done by hand; a later plan or close-out leaves it alone.
	put(t, s, "workout", day, ptr(StatusDone))
	if ok, cur := put(t, s, "workout", day, ptr(StatusPlanned), Blank); ok || *cur != StatusDone {
		t.Fatalf("plan over done: %v %v", ok, cur)
	}
	if ok, cur := put(t, s, "workout", day, ptr(StatusMissed), StatusPlanned); ok || *cur != StatusDone {
		t.Fatalf("close out done: %v %v", ok, cur)
	}
	// Close-out of a planned day, either way.
	put(t, s, "dance", day, ptr(StatusPlanned))
	if ok, cur := put(t, s, "dance", day, nil, StatusPlanned); !ok || cur != nil {
		t.Fatalf("clear planned: %v %v", ok, cur)
	}
	// A blank day isn't closed out.
	if ok, cur := put(t, s, "dance", day, ptr(StatusMissed), StatusPlanned); ok || cur != nil {
		t.Fatalf("close out blank: %v %v", ok, cur)
	}
	// Several allowed statuses.
	put(t, s, "dance", day, ptr(StatusMissed))
	if ok, _ := put(t, s, "dance", day, ptr(StatusDone), Blank, StatusPlanned, StatusMissed); !ok {
		t.Fatal("done over missed refused")
	}
}

func TestDays(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	put(t, s, "dance", "2026-11-18", ptr(StatusPlanned))
	put(t, s, "workout", "2026-11-18", ptr(StatusPlanned))
	put(t, s, "workout", "2026-11-17", ptr(StatusPlanned))
	put(t, s, "workout", "2026-11-19", ptr(StatusPlanned)) // today: not before
	put(t, s, "workout", "2026-11-16", ptr(StatusDone))

	days, err := s.Days(ctx, StatusPlanned, "2026-11-19")
	if err != nil {
		t.Fatal(err)
	}
	want := []DayEntry{
		{"2026-11-17", "workout", StatusPlanned},
		{"2026-11-18", "workout", StatusPlanned},
		{"2026-11-18", "dance", StatusPlanned},
	}
	if !slices.Equal(days, want) {
		t.Fatalf("got %+v", days)
	}
	if all, _ := s.Days(ctx, StatusPlanned, ""); len(all) != 4 {
		t.Fatalf("without before: %+v", all)
	}
	if _, err := s.Days(ctx, "", ""); err == nil {
		t.Fatal("missing status accepted")
	}
	if _, err := s.Days(ctx, StatusPlanned, "tomorrow"); err == nil {
		t.Fatal("bad before accepted")
	}
}

func TestReplaceGoals(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	short := []Goal{
		{ID: "a", Name: "Low one", Priority: ptr("low"), URL: "https://app.notion.com/p/a"},
		{ID: "b", Name: "High later", Priority: ptr("high"), Due: ptr("2027-03-31"), Progress: ptr(0.9), URL: "https://app.notion.com/p/b"},
		{ID: "c", Name: "High sooner", Priority: ptr("high"), Due: ptr("2026-12-31"), URL: "https://app.notion.com/p/c"},
	}
	if err := s.ReplaceGoals(ctx, ListShortTerm, short); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceGoals(ctx, ListPerennial, []Goal{{ID: "p", Name: "Stay fit", URL: "https://app.notion.com/p/p"}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Goals(ctx, ListShortTerm)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "c" || got[1].ID != "b" || got[2].ID != "a" || *got[0].Priority != "high" {
		t.Fatalf("order: %+v", got)
	}

	// Replacing one list leaves the other alone and drops goals that are gone.
	if err := s.ReplaceGoals(ctx, ListShortTerm, short[:1]); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Goals(ctx, ListShortTerm); len(got) != 1 {
		t.Fatalf("short-term after replace: %+v", got)
	}
	if got, _ := s.Goals(ctx, ListPerennial); len(got) != 1 {
		t.Fatalf("perennial touched: %+v", got)
	}

	// A goal moved to the other database keeps its page ID.
	if err := s.ReplaceGoals(ctx, ListPerennial, []Goal{{ID: "a", Name: "Low one", URL: "https://app.notion.com/p/a"}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Goals(ctx, ListShortTerm); len(got) != 0 {
		t.Fatalf("moved goal still short-term: %+v", got)
	}
}

func TestReplaceGoalsValidation(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	ok := Goal{ID: "a", Name: "Goal", URL: "https://app.notion.com/p/a"}
	for _, tc := range []struct {
		name string
		list string
		goal func(Goal) Goal
	}{
		{"bad list", "long_term", func(g Goal) Goal { return g }},
		{"no id", ListShortTerm, func(g Goal) Goal { g.ID = ""; return g }},
		{"no name", ListShortTerm, func(g Goal) Goal { g.Name = " "; return g }},
		{"http url", ListShortTerm, func(g Goal) Goal { g.URL = "javascript:alert(1)"; return g }},
		{"bad priority", ListShortTerm, func(g Goal) Goal { g.Priority = ptr("High priority"); return g }},
		{"progress over 1", ListShortTerm, func(g Goal) Goal { g.Progress = ptr(92.0); return g }},
		{"bad due", ListShortTerm, func(g Goal) Goal { g.Due = ptr("31/03/2027"); return g }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var verr *ValidationError
			if err := s.ReplaceGoals(ctx, tc.list, []Goal{tc.goal(ok)}); !errors.As(err, &verr) {
				t.Fatalf("want validation error, got %v", err)
			}
		})
	}
	if err := s.ReplaceGoals(ctx, ListShortTerm, []Goal{ok, ok}); err == nil {
		t.Fatal("duplicate ids accepted")
	}
}

func TestPutHabit(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	if err := s.PutHabit(ctx, Habit{Key: "reading", Name: "Reading", Position: 5}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutHabit(ctx, Habit{Key: "Bad Key", Name: "x"}); err == nil {
		t.Fatal("bad key accepted")
	}
	habits, err := s.Habits(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(habits) != 4 || habits[3].Key != "reading" || habits[3].Plannable {
		t.Fatalf("habits: %+v", habits)
	}
}

func TestArchivedHabitKeepsHistoryOffTheDashboard(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	put(t, s, "dance", "2026-11-16", ptr(StatusMissed))
	put(t, s, "workout", "2026-11-16", ptr(StatusDone))
	if err := s.PutHabit(ctx, Habit{Key: "dance", Name: "Dance", Position: 2, Plannable: true, Archived: true}); err != nil {
		t.Fatal(err)
	}
	w, err := s.Week(ctx, monday(t), monday(t).AddDate(0, 0, 6))
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Habits) != 2 || w.Habits[1].Key != "writing" {
		t.Fatalf("habits: %+v", w.Habits)
	}
	// The missed dance no longer counts against adherence.
	if w.Summary.Missed != 0 || *w.Summary.Adherence != 1 {
		t.Fatalf("summary: %+v", w.Summary)
	}
	var n int
	if err := s.db.QueryRow("SELECT count(*) FROM habit_days WHERE habit = 'dance'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("history: %d %v", n, err)
	}
	// A habit with history can't be deleted.
	if _, err := s.db.Exec("DELETE FROM habits WHERE key = 'dance'"); err == nil {
		t.Fatal("habit with history deleted")
	}
}

func TestSourceIsRecorded(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	day := Day{Date: "2026-11-16", Status: ptr(StatusPlanned)}
	if _, _, err := s.PutDay(ctx, "workout", day, SourceN8n); err != nil {
		t.Fatal(err)
	}
	day.Status = ptr(StatusDone)
	if _, _, err := s.PutDay(ctx, "workout", day, SourceApp); err != nil {
		t.Fatal(err)
	}
	w, err := s.Week(ctx, monday(t), monday(t))
	if err != nil {
		t.Fatal(err)
	}
	if d := w.Habits[0].Days[0]; *d.Status != StatusDone || *d.Source != SourceApp {
		t.Fatalf("day: %+v", d)
	}
	var verr *ValidationError
	if _, _, err := s.PutDay(ctx, "workout", day, "cron"); !errors.As(err, &verr) {
		t.Fatalf("bad source: %v", err)
	}
}

// The schema refuses bad rows on its own, without the Go checks.
func TestSchemaConstraints(t *testing.T) {
	s := open(t)
	for _, q := range []string{
		`INSERT INTO habits (key, name, position, plannable) VALUES ('Bad Key', 'X', 9, 0)`,
		`INSERT INTO habits (key, name, position, plannable) VALUES ('9lives', 'X', 9, 0)`,
		`INSERT INTO habits (key, name, position, plannable) VALUES ('reading', '  ', 9, 0)`,
		`INSERT INTO habits (key, name, position, plannable) VALUES ('reading', 'Reading', 9, 2)`,
		`INSERT INTO habits (key, name, position, plannable) VALUES ('reading', 'Reading', 'first', 0)`,
		`INSERT INTO habit_days VALUES ('2026-13-01', 'workout', 'done', 'n8n', 1)`,
		`INSERT INTO habit_days VALUES ('2026-11-16', 'workout', 'Done', 'n8n', 1)`,
		`INSERT INTO habit_days VALUES ('2026-11-16', 'workout', 'done', 'cron', 1)`,
		`INSERT INTO habit_days VALUES ('2026-11-16', 'juggling', 'done', 'n8n', 1)`,
		`INSERT INTO habit_days VALUES ('2026-11-16', 'workout', 'done', 'n8n', 'now')`,
		`INSERT INTO goals (id, list, name, url, synced_at) VALUES ('g', 'someday', 'G', 'https://x', 1)`,
		`INSERT INTO goals (id, list, name, url, synced_at) VALUES ('g', 'perennial', 'G', 'javascript:x', 1)`,
		`INSERT INTO goals (id, list, name, priority, url, synced_at) VALUES ('g', 'perennial', 'G', 4, 'https://x', 1)`,
		`INSERT INTO goals (id, list, name, progress, url, synced_at) VALUES ('g', 'perennial', 'G', 92, 'https://x', 1)`,
		`INSERT INTO goals (id, list, name, due, url, synced_at) VALUES ('g', 'perennial', 'G', '31/03/2027', 'https://x', 1)`,
	} {
		if _, err := s.db.Exec(q); err == nil {
			t.Errorf("accepted: %s", q)
		}
	}
}
