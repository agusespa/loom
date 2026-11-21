package store

import (
	"context"
	"database/sql"
	"time"
)

// HabitWeek is one habit with a slot for every day of the week (Monday first).
type HabitWeek struct {
	Habit
	Days []Day `json:"days"`
	Done int   `json:"done"`
}

// Summary counts the week. Adherence is done / (done + missed) over the
// plannable habits, so habits that are never planned can't inflate it; it
// is nil until something planned has been done or missed.
type Summary struct {
	Done      int      `json:"done"`
	Missed    int      `json:"missed"`
	Planned   int      `json:"planned"`
	Adherence *float64 `json:"adherence"`
}

type Week struct {
	Start         string      `json:"start"`
	End           string      `json:"end"`
	Habits        []HabitWeek `json:"habits"`
	Summary       Summary     `json:"summary"`
	PrevAdherence *float64    `json:"prev_adherence"`
	Goals         struct {
		ShortTerm []Goal `json:"short_term"`
		Perennial []Goal `json:"perennial"`
	} `json:"goals"`
	// UpdatedAt is the latest write from n8n (ms), so the app can say how fresh it is.
	UpdatedAt *int64 `json:"updated_at"`
}

// Week returns the 7 days starting at monday.
func (s *Store) Week(ctx context.Context, monday time.Time) (*Week, error) {
	habits, err := s.Habits(ctx)
	if err != nil {
		return nil, err
	}
	start := monday.Format(time.DateOnly)
	end := monday.AddDate(0, 0, 6).Format(time.DateOnly)
	w := &Week{Start: start, End: end, Habits: make([]HabitWeek, len(habits))}

	index := map[string]int{}
	for i, h := range habits {
		days := make([]Day, 7)
		for d := range days {
			days[d].Date = monday.AddDate(0, 0, d).Format(time.DateOnly)
		}
		w.Habits[i] = HabitWeek{Habit: h, Days: days}
		index[h.Key] = i
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT date, habit, status, value, note FROM habit_days WHERE date BETWEEN ? AND ?", start, end)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var d Day
		var habit string
		if err := rows.Scan(&d.Date, &habit, &d.Status, &d.Value, &d.Note); err != nil {
			return nil, err
		}
		i, ok := index[habit]
		if !ok {
			continue
		}
		t, err := time.Parse(time.DateOnly, d.Date)
		if err != nil {
			continue
		}
		slot := int(t.Sub(monday).Hours() / 24)
		if slot < 0 || slot > 6 {
			continue
		}
		w.Habits[i].Days[slot] = d
		if d.Status != nil && *d.Status == StatusDone {
			w.Habits[i].Done++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if w.Summary, err = s.summary(ctx, start, end); err != nil {
		return nil, err
	}
	prev, err := s.summary(ctx, monday.AddDate(0, 0, -7).Format(time.DateOnly), monday.AddDate(0, 0, -1).Format(time.DateOnly))
	if err != nil {
		return nil, err
	}
	w.PrevAdherence = prev.Adherence

	if w.Goals.ShortTerm, err = s.Goals(ctx, ListShortTerm); err != nil {
		return nil, err
	}
	if w.Goals.Perennial, err = s.Goals(ctx, ListPerennial); err != nil {
		return nil, err
	}

	var updated sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `
		SELECT max(t) FROM (SELECT max(updated_at) AS t FROM habit_days UNION ALL SELECT max(synced_at) FROM goals)`,
	).Scan(&updated); err != nil {
		return nil, err
	}
	if updated.Valid {
		w.UpdatedAt = &updated.Int64
	}
	return w, nil
}

func (s *Store) summary(ctx context.Context, start, end string) (Summary, error) {
	var sum Summary
	err := s.db.QueryRowContext(ctx, `
		SELECT
			count(*) FILTER (WHERE d.status = 'done'),
			count(*) FILTER (WHERE d.status = 'missed'),
			count(*) FILTER (WHERE d.status = 'planned')
		FROM habit_days d JOIN habits h ON h.key = d.habit
		WHERE h.plannable AND d.date BETWEEN ? AND ?`, start, end).Scan(&sum.Done, &sum.Missed, &sum.Planned)
	if err != nil {
		return sum, err
	}
	if resolved := sum.Done + sum.Missed; resolved > 0 {
		a := float64(sum.Done) / float64(resolved)
		sum.Adherence = &a
	}
	return sum, nil
}
