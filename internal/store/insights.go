package store

import (
	"context"
	"time"
)

// Insights are the numbers the week grid can't show: how long a habit has
// kept going, how adherence moved over the last weeks and the last 30 days.
// All are counted up to the week's "as of" day: today for the current week,
// otherwise its Sunday.
type Insights struct {
	AsOf   string     `json:"as_of"`
	Streak *Streak    `json:"streak"`
	Trend  []*float64 `json:"trend"` // adherence of the last TrendWeeks weeks, oldest first; nil = nothing resolved
	Last30 Last30     `json:"last_30"`
}

// Streak is the longest current run among the habits. Plannable habits count
// planned sessions done in a row (a missed one breaks it; unplanned days don't);
// the others count days in a row.
type Streak struct {
	Habit string `json:"habit"`
	Name  string `json:"name"`
	Count int    `json:"count"`
	Unit  string `json:"unit"` // "sessions" or "days"
}

type Last30 struct {
	From   string       `json:"from"`
	To     string       `json:"to"`
	Habits []HabitCount `json:"habits"`
}

type HabitCount struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Plannable bool   `json:"plannable"`
	Done      int    `json:"done"`
}

const TrendWeeks = 8

func (s *Store) insights(ctx context.Context, habits []Habit, monday, today time.Time) (Insights, error) {
	asOf := monday.AddDate(0, 0, 6)
	if today.Before(asOf) {
		asOf = today
	}
	in := Insights{AsOf: asOf.Format(time.DateOnly)}

	for i := TrendWeeks - 1; i >= 0; i-- {
		start := monday.AddDate(0, 0, -7*i)
		sum, err := s.summary(ctx, start.Format(time.DateOnly), start.AddDate(0, 0, 6).Format(time.DateOnly))
		if err != nil {
			return in, err
		}
		in.Trend = append(in.Trend, sum.Adherence)
	}

	from := asOf.AddDate(0, 0, -29)
	in.Last30 = Last30{From: from.Format(time.DateOnly), To: in.AsOf, Habits: make([]HabitCount, len(habits))}
	for i, h := range habits {
		hc := HabitCount{Key: h.Key, Name: h.Name, Plannable: h.Plannable}
		if err := s.db.QueryRowContext(ctx,
			"SELECT count(*) FROM habit_days WHERE habit = ? AND status = 'done' AND date BETWEEN ? AND ?",
			h.Key, in.Last30.From, in.AsOf).Scan(&hc.Done); err != nil {
			return in, err
		}
		in.Last30.Habits[i] = hc
	}

	for _, h := range habits {
		n, err := s.streak(ctx, h, asOf, asOf.Equal(today))
		if err != nil {
			return in, err
		}
		if n > 0 && (in.Streak == nil || n > in.Streak.Count) {
			unit := "days"
			if h.Plannable {
				unit = "sessions"
			}
			in.Streak = &Streak{Habit: h.Key, Name: h.Name, Count: n, Unit: unit}
		}
	}
	return in, nil
}

// streak counts the habit's current run ending at asOf. When asOf is today a
// day habit not done yet still keeps yesterday's run: the day isn't over.
func (s *Store) streak(ctx context.Context, h Habit, asOf time.Time, isToday bool) (int, error) {
	q := "SELECT date, status FROM habit_days WHERE habit = ? AND date <= ? AND status IN ('done', 'missed') ORDER BY date DESC"
	rows, err := s.db.QueryContext(ctx, q, h.Key, asOf.Format(time.DateOnly))
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	n := 0
	expect := asOf // day habits: the next date the run needs
	for rows.Next() {
		var date, status string
		if err := rows.Scan(&date, &status); err != nil {
			return 0, err
		}
		if h.Plannable {
			if status != StatusDone {
				break
			}
			n++
			continue
		}
		want := expect.Format(time.DateOnly)
		if n == 0 && isToday && date != want {
			expect = expect.AddDate(0, 0, -1) // today not done yet
			want = expect.Format(time.DateOnly)
		}
		if date != want {
			break
		}
		n++
		expect = expect.AddDate(0, 0, -1)
	}
	return n, rows.Err()
}
