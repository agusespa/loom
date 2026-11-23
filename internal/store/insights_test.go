package store

import (
	"context"
	"testing"
	"time"
)

func date(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func insightsFor(t *testing.T, s *Store, today string) Insights {
	t.Helper()
	w, err := s.Week(context.Background(), monday(t), date(t, today))
	if err != nil {
		t.Fatal(err)
	}
	return w.Insights
}

func TestStreakDays(t *testing.T) {
	s := open(t)
	for _, d := range []string{"2026-11-14", "2026-11-15", "2026-11-16", "2026-11-17", "2026-11-18"} {
		put(t, s, "writing", d, ptr(StatusDone))
	}
	put(t, s, "writing", "2026-11-12", ptr(StatusDone)) // before the gap on the 13th

	// Today (19th) not written yet: the run up to yesterday still counts.
	in := insightsFor(t, s, "2026-11-19")
	if in.Streak == nil || in.Streak.Habit != "writing" || in.Streak.Count != 5 || in.Streak.Unit != "days" {
		t.Fatalf("streak: %+v", in.Streak)
	}
	// A day later, with yesterday missed, it's gone.
	if in := insightsFor(t, s, "2026-11-20"); in.Streak != nil {
		t.Fatalf("broken streak: %+v", in.Streak)
	}
	// Writing today extends it.
	put(t, s, "writing", "2026-11-19", ptr(StatusDone))
	if in := insightsFor(t, s, "2026-11-19"); in.Streak.Count != 6 {
		t.Fatalf("with today: %+v", in.Streak)
	}
}

func TestStreakSessions(t *testing.T) {
	s := open(t)
	put(t, s, "workout", "2026-11-09", ptr(StatusMissed))
	put(t, s, "workout", "2026-11-11", ptr(StatusDone))
	put(t, s, "workout", "2026-11-14", ptr(StatusDone))
	put(t, s, "workout", "2026-11-16", ptr(StatusDone))
	put(t, s, "workout", "2026-11-18", ptr(StatusPlanned)) // still open: neither counts nor breaks
	put(t, s, "dance", "2026-11-17", ptr(StatusDone))

	in := insightsFor(t, s, "2026-11-18")
	if in.Streak == nil || in.Streak.Habit != "workout" || in.Streak.Count != 3 || in.Streak.Unit != "sessions" {
		t.Fatalf("streak: %+v", in.Streak)
	}
	put(t, s, "workout", "2026-11-18", ptr(StatusMissed))
	if in := insightsFor(t, s, "2026-11-18"); in.Streak.Habit != "dance" || in.Streak.Count != 1 {
		t.Fatalf("after a miss: %+v", in.Streak)
	}
}

func eq(a *float64, want float64) bool { return a != nil && *a == want }

func TestTrendAndAdherence(t *testing.T) {
	s := open(t)
	put(t, s, "workout", "2026-11-16", ptr(StatusDone))
	put(t, s, "workout", "2026-11-10", ptr(StatusMissed))
	put(t, s, "workout", "2026-10-20", ptr(StatusDone))
	put(t, s, "workout", "2026-11-19", ptr(StatusDone))   // after today: outside
	put(t, s, "workout", "2026-09-21", ptr(StatusMissed)) // 57 days before the 18th: the previous stretch
	put(t, s, "dance", "2026-11-12", ptr(StatusDone))
	// Writing starts on the 14th: no chances before it. Today (18th) isn't done yet.
	for _, d := range []string{"2026-11-14", "2026-11-15", "2026-11-17"} {
		put(t, s, "writing", d, ptr(StatusDone))
	}

	in := insightsFor(t, s, "2026-11-18")
	if in.AsOf != "2026-11-18" || len(in.Trend) != TrendWeeks {
		t.Fatalf("as of %s, trend %v", in.AsOf, in.Trend)
	}
	// Oldest first: the weeks of 28 Sep ... 16 Nov. Week of 9 Nov: workout 0/1, dance 1/1,
	// writing 2/2. This week, up to today: workout 1/1, writing 1/2.
	if in.Trend[0] != nil || !eq(in.Trend[3], 1) || in.Trend[4] != nil || !eq(in.Trend[6], 0.75) || !eq(in.Trend[7], 2.0/3) {
		t.Fatalf("trend: %v", in.Trend)
	}
	// Workout 2/3, dance 1/1, writing 3/4; before: workout 0/1.
	if !eq(in.Adherence, 0.75) || !eq(in.PrevAdherence, 0) {
		t.Fatalf("adherence %v, prev %v", in.Adherence, in.PrevAdherence)
	}
	got := map[string]HabitInsight{}
	for _, h := range in.Habits {
		got[h.Key] = h
	}
	if w := got["workout"]; !eq(w.Adherence, 2.0/3) || !eq(w.PrevAdherence, 0) || w.Streak != 1 || w.Unit != "sessions" {
		t.Fatalf("workout: %+v", w)
	}
	if d := got["dance"]; !eq(d.Adherence, 1) || d.PrevAdherence != nil {
		t.Fatalf("dance: %+v", d)
	}
	if w := got["writing"]; !eq(w.Adherence, 0.75) || w.PrevAdherence != nil || w.Streak != 1 || w.Unit != "days" {
		t.Fatalf("writing: %+v", w)
	}

	// Writing today makes it 4 of 5.
	put(t, s, "writing", "2026-11-18", ptr(StatusDone))
	for _, h := range insightsFor(t, s, "2026-11-18").Habits {
		if h.Key == "writing" && !eq(h.Adherence, 0.8) {
			t.Fatalf("writing with today: %+v", h)
		}
	}

	// A past week counts up to its Sunday.
	if in := insightsFor(t, s, "2026-12-30"); in.AsOf != "2026-11-22" {
		t.Fatalf("past week as of %s", in.AsOf)
	}
}
