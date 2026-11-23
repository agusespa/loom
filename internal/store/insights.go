package store

import (
	"context"
	"database/sql"
	"time"
)

// Insights are the numbers the week grid can't show: how long each habit has
// kept going and how adherence moved over the last weeks. All are counted up
// to the week's "as of" day: today for the current week, otherwise its Sunday.
//
// Adherence here covers every habit. A plannable habit's chances are its
// sessions done or missed; any other habit's chances are the days since it was
// first done (today only once it's done: the day isn't over).
type Insights struct {
	AsOf   string     `json:"as_of"`
	Streak *Streak    `json:"streak"`
	Trend  []*float64 `json:"trend"` // adherence of the last TrendWeeks weeks, oldest first; nil = no chances
	// Adherence over the last TrendWeeks weeks up to AsOf, and over the same
	// stretch before that.
	Adherence     *float64       `json:"adherence"`
	PrevAdherence *float64       `json:"prev_adherence"`
	Habits        []HabitInsight `json:"habits"`
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

// HabitInsight is one habit's streak and adherence, for the per-habit table.
type HabitInsight struct {
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	Plannable     bool     `json:"plannable"`
	Streak        int      `json:"streak"`
	Unit          string   `json:"unit"`
	Adherence     *float64 `json:"adherence"`
	PrevAdherence *float64 `json:"prev_adherence"`
}

const TrendWeeks = 8

// tally is done out of chances.
type tally struct{ done, chances int }

func (t *tally) add(o tally) { t.done += o.done; t.chances += o.chances }

func (t tally) ratio() *float64 {
	if t.chances == 0 {
		return nil
	}
	a := float64(t.done) / float64(t.chances)
	return &a
}

func unit(h Habit) string {
	if h.Plannable {
		return "sessions"
	}
	return "days"
}

func (s *Store) insights(ctx context.Context, habits []Habit, monday, today time.Time) (Insights, error) {
	asOf := monday.AddDate(0, 0, 6)
	if today.Before(asOf) {
		asOf = today
	}
	in := Insights{AsOf: asOf.Format(time.DateOnly), Habits: make([]HabitInsight, len(habits))}

	// The first day each habit was done: daily habits have no chances before it.
	first := make([]*time.Time, len(habits))
	for i, h := range habits {
		var d sql.NullString
		if err := s.db.QueryRowContext(ctx,
			"SELECT min(date) FROM habit_days WHERE habit = ? AND status = 'done'", h.Key).Scan(&d); err != nil {
			return in, err
		}
		if d.Valid {
			t, err := time.Parse(time.DateOnly, d.String)
			if err != nil {
				return in, err
			}
			first[i] = &t
		}
	}
	all := func(from, to time.Time) ([]tally, tally, error) {
		per := make([]tally, len(habits))
		var sum tally
		for i, h := range habits {
			t, err := s.tally(ctx, h, first[i], from, to, today)
			if err != nil {
				return nil, sum, err
			}
			per[i] = t
			sum.add(t)
		}
		return per, sum, nil
	}

	for i := TrendWeeks - 1; i >= 0; i-- {
		start := monday.AddDate(0, 0, -7*i)
		end := start.AddDate(0, 0, 6)
		if end.After(asOf) {
			end = asOf
		}
		_, sum, err := all(start, end)
		if err != nil {
			return in, err
		}
		in.Trend = append(in.Trend, sum.ratio())
	}

	span := 7 * TrendWeeks
	cur, curSum, err := all(asOf.AddDate(0, 0, 1-span), asOf)
	if err != nil {
		return in, err
	}
	prev, prevSum, err := all(asOf.AddDate(0, 0, 1-2*span), asOf.AddDate(0, 0, -span))
	if err != nil {
		return in, err
	}
	in.Adherence, in.PrevAdherence = curSum.ratio(), prevSum.ratio()

	for i, h := range habits {
		n, err := s.streak(ctx, h, asOf, asOf.Equal(today))
		if err != nil {
			return in, err
		}
		in.Habits[i] = HabitInsight{
			Key: h.Key, Name: h.Name, Plannable: h.Plannable, Streak: n, Unit: unit(h),
			Adherence: cur[i].ratio(), PrevAdherence: prev[i].ratio(),
		}
		if n > 0 && (in.Streak == nil || n > in.Streak.Count) {
			in.Streak = &Streak{Habit: h.Key, Name: h.Name, Count: n, Unit: unit(h)}
		}
	}
	return in, nil
}

// tally counts one habit's done and chances from from to to, both included.
// first is the first day the habit was ever done (nil: never).
func (s *Store) tally(ctx context.Context, h Habit, first *time.Time, from, to, today time.Time) (tally, error) {
	var t tally
	var missed, doneToday int
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'done'),
			count(*) FILTER (WHERE status = 'missed'),
			count(*) FILTER (WHERE status = 'done' AND date = ?)
		FROM habit_days WHERE habit = ? AND date BETWEEN ? AND ?`,
		today.Format(time.DateOnly), h.Key, from.Format(time.DateOnly), to.Format(time.DateOnly),
	).Scan(&t.done, &missed, &doneToday); err != nil {
		return t, err
	}
	if h.Plannable {
		t.chances = t.done + missed
		return t, nil
	}
	if first == nil {
		return t, nil
	}
	if first.After(from) {
		from = *first
	}
	if days := int(to.Sub(from).Hours()/24) + 1; days > 0 {
		t.chances = days
		if !to.Before(today) && !from.After(today) && doneToday == 0 {
			t.chances-- // today isn't over
		}
	}
	return t, nil
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
