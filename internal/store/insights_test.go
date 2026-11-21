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

func TestTrendAndLast30(t *testing.T) {
	s := open(t)
	put(t, s, "workout", "2026-11-16", ptr(StatusDone))   // this week: 100%
	put(t, s, "workout", "2026-11-10", ptr(StatusMissed)) // last week: 50%
	put(t, s, "dance", "2026-11-12", ptr(StatusDone))
	put(t, s, "workout", "2026-10-20", ptr(StatusDone))   // 4 weeks back
	put(t, s, "writing", "2026-10-19", ptr(StatusDone))   // 31 days before the 18th: outside last 30
	put(t, s, "writing", "2026-10-20", ptr(StatusDone))   // 30th day back: inside
	put(t, s, "workout", "2026-11-19", ptr(StatusDone))   // after today: outside
	put(t, s, "workout", "2026-09-21", ptr(StatusMissed)) // 9 weeks back: outside the trend

	in := insightsFor(t, s, "2026-11-18")
	if in.AsOf != "2026-11-18" || len(in.Trend) != TrendWeeks {
		t.Fatalf("as of %s, trend %v", in.AsOf, in.Trend)
	}
	// Oldest first: the weeks of 28 Sep ... 16 Nov (21 Sep is one too far back).
	if in.Trend[0] != nil || *in.Trend[3] != 1 || in.Trend[4] != nil || *in.Trend[6] != 0.5 || *in.Trend[7] != 1 {
		t.Fatalf("trend: %v", in.Trend)
	}
	if in.Last30.From != "2026-10-20" || in.Last30.To != "2026-11-18" {
		t.Fatalf("range %s..%s", in.Last30.From, in.Last30.To)
	}
	got := map[string]int{}
	for _, h := range in.Last30.Habits {
		got[h.Key] = h.Done
	}
	// The trend counts the whole week, but the 30 days stop at today.
	if got["workout"] != 2 || got["dance"] != 1 || got["writing"] != 1 {
		t.Fatalf("last 30: %v", got)
	}

	// A past week counts up to its Sunday.
	if in := insightsFor(t, s, "2026-12-30"); in.AsOf != "2026-11-22" {
		t.Fatalf("past week as of %s", in.AsOf)
	}
}
