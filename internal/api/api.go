// Package api serves the write API (n8n and the app), the week endpoint the
// app reads and the embedded frontend.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/agusespa/loom/internal/store"
)

const maxBody = 1 << 20

// Clock says which tracker day it is. The tracker day starts at DayStart
// (05:00 by default), so a workout synced at 01:00 still counts for the day before.
type Clock struct {
	Loc      *time.Location
	DayStart time.Duration
	Now      func() time.Time // nil means time.Now
}

// Today returns the current tracker day as a UTC midnight.
func (c Clock) Today() time.Time {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	t := now().In(c.Loc).Add(-c.DayStart)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// New returns the HTTP handler. static is the built frontend (may be empty).
//
// The app has no login of its own. From outside it is reached only through
// Cloudflare Access, which is what protects edits made in the app; n8n calls
// it over the Docker network. Requests that
// change data are refused when a browser marks them as coming from another
// site, so a page you visit can't use your Access session to edit data.
func New(s *store.Store, static fs.FS, clock Clock) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /api/week", weekHandler(s, clock))
	mux.Handle("GET /api/habits", habitsHandler(s))
	mux.Handle("PUT /api/habits/{key}", putHabitHandler(s))
	mux.Handle("GET /api/days", daysHandler(s))
	mux.Handle("PUT /api/days/{date}/{habit}", putDayHandler(s))
	mux.Handle("DELETE /api/days/{date}/{habit}", deleteDayHandler(s))
	mux.Handle("GET /api/goals/{list}", goalsHandler(s))
	mux.Handle("PUT /api/goals/{list}", replaceGoalsHandler(s))
	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.Ready(ctx); err != nil {
			log.Printf("readiness: %v", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		jsonError(w, http.StatusNotFound, "not found")
	})
	mux.Handle("/", staticHandler(static))

	csrf := http.NewCrossOriginProtection()
	csrf.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		jsonError(w, http.StatusForbidden, "cross-origin request refused")
	}))
	return csrf.Handler(mux)
}

// GET /api/week?start=YYYY-MM-DD (a Monday). Without start: the current week.
func weekHandler(s *store.Store, clock Clock) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		today := clock.Today()
		var monday time.Time
		if q := r.URL.Query().Get("start"); q != "" {
			t, err := time.Parse(time.DateOnly, q)
			if err != nil || t.Weekday() != time.Monday {
				jsonError(w, http.StatusBadRequest, "start must be a Monday as YYYY-MM-DD")
				return
			}
			monday = t
		} else {
			monday = today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
		}
		week, err := s.Week(r.Context(), monday, today)
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			*store.Week
			Today string `json:"today"`
		}{week, today.Format(time.DateOnly)})
	}
}

func habitsHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		habits, err := s.Habits(r.Context())
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"habits": habits})
	}
}

// PUT /api/habits/{key} {"name": "Workout", "position": 1, "plannable": true, "archived": false}
func putHabitHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var h store.Habit
		if !decode(w, r, &h) {
			return
		}
		h.Key = r.PathValue("key")
		if err := s.PutHabit(r.Context(), h); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, h)
	}
}

// source tells corrections made in the PWA from n8n's writes. Browsers mark
// every fetch with Sec-Fetch-Site, and requests from outside come through
// Cloudflare Access, which adds the signed-in user; n8n's calls over the
// Docker network carry neither.
func source(r *http.Request) string {
	if r.Header.Get("Sec-Fetch-Site") != "" || r.Header.Get("Cf-Access-Authenticated-User-Email") != "" {
		return store.SourceApp
	}
	return store.SourceN8n
}

// GET /api/days?status=planned&before=YYYY-MM-DD lists logged days, oldest first.
func daysHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		days, err := s.Days(r.Context(), q.Get("status"), q.Get("before"))
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"days": days})
	}
}

// dayResult answers writes to a day. Applied is false when an ?if= condition
// didn't hold; Status is then the day's unchanged status.
type dayResult struct {
	Date    string  `json:"date"`
	Habit   string  `json:"habit"`
	Status  *string `json:"status"`
	Applied bool    `json:"applied"`
}

// PUT /api/days/{date}/{habit}[?if=blank,planned] {"status": "done"}
// A null status clears the day. With ?if= the write only happens when the
// day's current status is one of those listed (blank meaning none).
func putDayHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Status *string `json:"status"`
		}
		if !decode(w, r, &body) {
			return
		}
		d := store.Day{Date: r.PathValue("date"), Status: body.Status}
		var onlyIf []string
		if q := r.URL.Query().Get("if"); q != "" {
			onlyIf = strings.Split(q, ",")
		}
		writeDay(w, r, s, d, onlyIf)
	}
}

// DELETE /api/days/{date}/{habit} clears the day, like PUT with a null status.
func deleteDayHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeDay(w, r, s, store.Day{Date: r.PathValue("date")}, nil)
	}
}

func writeDay(w http.ResponseWriter, r *http.Request, s *store.Store, d store.Day, onlyIf []string) {
	habit := r.PathValue("habit")
	applied, current, err := s.PutDay(r.Context(), habit, d, source(r), onlyIf...)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dayResult{Date: d.Date, Habit: habit, Status: current, Applied: applied})
}

func goalsHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list := r.PathValue("list")
		if list != store.ListShortTerm && list != store.ListPerennial {
			jsonError(w, http.StatusNotFound, "list must be short_term or perennial")
			return
		}
		goals, err := s.Goals(r.Context(), list)
		if err != nil {
			serverError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"goals": goals})
	}
}

// PUT /api/goals/{list} {"goals": [...]} replaces that list.
func replaceGoalsHandler(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Goals *[]store.Goal `json:"goals"`
		}
		if !decode(w, r, &body) {
			return
		}
		if body.Goals == nil {
			jsonError(w, http.StatusBadRequest, "goals is required (send [] to empty the list)")
			return
		}
		if err := s.ReplaceGoals(r.Context(), r.PathValue("list"), *body.Goals); err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]int{"count": len(*body.Goals)})
	}
}

// ---------- helpers ----------

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func writeStoreError(w http.ResponseWriter, err error) {
	var verr *store.ValidationError
	switch {
	case errors.As(err, &verr):
		jsonError(w, http.StatusBadRequest, verr.Error())
	case errors.Is(err, store.ErrNotFound):
		jsonError(w, http.StatusNotFound, "unknown habit")
	default:
		serverError(w, err)
	}
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	jsonError(w, http.StatusInternalServerError, "internal error")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// staticHandler serves the SPA: existing files as-is, everything else index.html.
func staticHandler(static fs.FS) http.Handler {
	files := http.FileServerFS(static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p != "" {
			if st, err := fs.Stat(static, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(static, "index.html")
		if err != nil {
			http.Error(w, "frontend not built", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
