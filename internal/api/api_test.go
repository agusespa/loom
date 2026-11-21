package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/agusespa/loom/internal/store"
)

func newServer(t *testing.T, now string) http.Handler {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "loom.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	loc, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Fatal(err)
	}
	at, err := time.ParseInLocation("2006-01-02 15:04", now, loc)
	if err != nil {
		t.Fatal(err)
	}
	static := fstest.MapFS{"index.html": {Data: []byte("<html>loom</html>")}}
	return New(s, static, Clock{Loc: loc, DayStart: 5 * time.Hour, Now: func() time.Time { return at }})
}

func do(t *testing.T, h http.Handler, method, url, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestWriteThenReadWeek(t *testing.T) {
	h := newServer(t, "2026-11-21 12:00")
	for _, w := range []struct{ url, body string }{
		{"/api/days/2026-11-16/workout", `{"status":"done","note":"Upper-body"}`},
		{"/api/days/2026-11-18/workout", `{"status":"missed"}`},
		{"/api/days/2026-11-21/meditation", `{"status":"done","value":14}`},
		{"/api/goals/short_term", `{"goals":[{"id":"g1","name":"Bench 100 kg","priority":"high","progress":0.92,"due":"2027-03-31","url":"https://app.notion.com/p/g1"}]}`},
	} {
		if rec := do(t, h, "PUT", w.url, w.body); rec.Code != 200 {
			t.Fatalf("PUT %s: %d %s", w.url, rec.Code, rec.Body)
		}
	}

	rec := do(t, h, "GET", "/api/week", "")
	if rec.Code != 200 {
		t.Fatalf("week: %d %s", rec.Code, rec.Body)
	}
	var week struct {
		Start, End, Today string
		Habits            []struct {
			Key  string
			Done int
			Days []struct{ Status *string }
		}
		Summary struct{ Done, Missed int }
		Goals   struct {
			ShortTerm []struct{ Name string } `json:"short_term"`
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &week); err != nil {
		t.Fatal(err)
	}
	if week.Start != "2026-11-16" || week.End != "2026-11-22" || week.Today != "2026-11-21" {
		t.Fatalf("dates: %s..%s today %s", week.Start, week.End, week.Today)
	}
	if week.Habits[0].Done != 1 || week.Summary.Done != 1 || week.Summary.Missed != 1 {
		t.Fatalf("counts: %+v %+v", week.Habits[0], week.Summary)
	}
	if len(week.Goals.ShortTerm) != 1 || week.Goals.ShortTerm[0].Name != "Bench 100 kg" {
		t.Fatalf("goals: %+v", week.Goals)
	}
}

func TestTrackerDayStartsAtFive(t *testing.T) {
	// 03:00 on Monday is still Sunday's tracker day, so the week is the previous one.
	h := newServer(t, "2026-11-23 03:00")
	rec := do(t, h, "GET", "/api/week", "")
	var week struct{ Start, Today string }
	_ = json.Unmarshal(rec.Body.Bytes(), &week)
	if week.Today != "2026-11-22" || week.Start != "2026-11-16" {
		t.Fatalf("got today %s start %s", week.Today, week.Start)
	}
}

func TestErrors(t *testing.T) {
	h := newServer(t, "2026-11-21 12:00")
	for _, tc := range []struct {
		method, url, body string
		want              int
	}{
		{"PUT", "/api/days/2026-11-16/juggling", `{"status":"done"}`, 404},
		{"PUT", "/api/days/2026-11-16/workout", `{"status":"Done"}`, 400},
		{"PUT", "/api/days/2026-11-16/workout", `{"stauts":"done"}`, 400},
		{"PUT", "/api/goals/short_term", `{}`, 400},
		{"PUT", "/api/goals/someday", `{"goals":[]}`, 400},
		{"GET", "/api/week?start=2026-11-17", "", 400},
		{"GET", "/api/nope", "", 404},
	} {
		if rec := do(t, h, tc.method, tc.url, tc.body); rec.Code != tc.want {
			t.Errorf("%s %s: got %d want %d (%s)", tc.method, tc.url, rec.Code, tc.want, rec.Body)
		}
	}
}

func TestCrossSiteWriteRefused(t *testing.T) {
	h := newServer(t, "2026-11-21 12:00")
	req := httptest.NewRequest("PUT", "/api/days/2026-11-16/workout", strings.NewReader(`{"status":"done"}`))
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestStatic(t *testing.T) {
	h := newServer(t, "2026-11-21 12:00")
	if rec := do(t, h, "GET", "/", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), "loom") {
		t.Fatalf("index: %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/missing.js", ""); rec.Code != 404 {
		t.Fatalf("missing: %d", rec.Code)
	}
}
