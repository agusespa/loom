package store

import (
	"context"
	"errors"
	"path/filepath"
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
		name  string
		habit string
		day   Day
	}{
		{"bad date", "workout", Day{Date: "2026-13-01", Status: ptr(StatusDone)}},
		{"bad format", "workout", Day{Date: "21/11/2026", Status: ptr(StatusDone)}},
		{"bad status", "workout", Day{Date: "2026-11-16", Status: ptr("Done")}},
		{"meditation cannot be missed", "meditation", Day{Date: "2026-11-16", Status: ptr(StatusMissed)}},
		{"writing cannot be planned", "writing", Day{Date: "2026-11-16", Status: ptr(StatusPlanned)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var verr *ValidationError
			if err := s.PutDay(ctx, tc.habit, tc.day); !errors.As(err, &verr) {
				t.Fatalf("want validation error, got %v", err)
			}
		})
	}
	if err := s.PutDay(ctx, "juggling", Day{Date: "2026-11-16", Status: ptr(StatusDone)}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown habit: want ErrNotFound, got %v", err)
	}
}

func TestWeek(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	put := func(habit, date string, d Day) {
		t.Helper()
		d.Date = date
		if err := s.PutDay(ctx, habit, d); err != nil {
			t.Fatal(err)
		}
	}
	put("workout", "2026-11-16", Day{Status: ptr(StatusDone), Note: ptr("Upper-body")})
	put("workout", "2026-11-18", Day{Status: ptr(StatusMissed)})
	put("workout", "2026-11-20", Day{Status: ptr(StatusDone)})
	put("dance", "2026-11-22", Day{Status: ptr(StatusPlanned)})
	put("meditation", "2026-11-16", Day{Status: ptr(StatusDone), Value: ptr(12.0)})
	put("meditation", "2026-11-17", Day{Value: ptr(4.0)}) // below the bar: value only
	put("writing", "2026-11-17", Day{Status: ptr(StatusDone)})
	// Outside the week, and last week for the comparison.
	put("workout", "2026-11-23", Day{Status: ptr(StatusDone)})
	put("workout", "2026-11-10", Day{Status: ptr(StatusMissed)})
	put("dance", "2026-11-12", Day{Status: ptr(StatusDone)})

	w, err := s.Week(ctx, monday(t))
	if err != nil {
		t.Fatal(err)
	}
	if w.Start != "2026-11-16" || w.End != "2026-11-22" {
		t.Fatalf("range %s..%s", w.Start, w.End)
	}
	if len(w.Habits) != 4 || w.Habits[0].Key != "workout" {
		t.Fatalf("habits: %+v", w.Habits)
	}
	workout := w.Habits[0]
	if workout.Done != 2 || *workout.Days[0].Note != "Upper-body" || *workout.Days[2].Status != StatusMissed || workout.Days[1].Status != nil {
		t.Fatalf("workout: %+v", workout)
	}
	med := w.Habits[2]
	if med.Done != 1 || med.Days[1].Status != nil || *med.Days[1].Value != 4 {
		t.Fatalf("meditation: %+v", med)
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

func TestEmptyDayIsDeleted(t *testing.T) {
	s := open(t)
	ctx := context.Background()
	if err := s.PutDay(ctx, "workout", Day{Date: "2026-11-16", Status: ptr(StatusPlanned)}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutDay(ctx, "workout", Day{Date: "2026-11-16"}); err != nil {
		t.Fatal(err)
	}
	w, err := s.Week(ctx, monday(t))
	if err != nil {
		t.Fatal(err)
	}
	if w.Habits[0].Days[0].Status != nil || w.Summary.Planned != 0 || w.Summary.Adherence != nil {
		t.Fatalf("row not removed: %+v %+v", w.Habits[0].Days[0], w.Summary)
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
	if len(habits) != 5 || habits[4].Key != "reading" || habits[4].Plannable {
		t.Fatalf("habits: %+v", habits)
	}
}
