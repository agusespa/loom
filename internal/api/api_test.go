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
	static := fstest.MapFS{
		"index.html":            {Data: []byte("<html>loom</html>")},
		"_app/immutable/app.js": {Data: []byte("app")},
		"service-worker.js":     {Data: []byte("sw")},
	}
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
		{"/api/days/2026-11-16/workout", `{"status":"done"}`},
		{"/api/days/2026-11-18/workout", `{"status":"missed"}`},
		{"/api/days/2026-11-21/writing", `{"status":"done"}`},
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
	if week.Summary.Done != 1 || week.Summary.Missed != 1 {
		t.Fatalf("counts: %+v %+v", week.Habits[0], week.Summary)
	}
	if len(week.Goals.ShortTerm) != 1 || week.Goals.ShortTerm[0].Name != "Bench 100 kg" {
		t.Fatalf("goals: %+v", week.Goals)
	}
}

func TestConditionalDayWrites(t *testing.T) {
	h := newServer(t, "2026-11-21 12:00")
	write := func(method, url, body string) (applied bool, status *string) {
		t.Helper()
		rec := do(t, h, method, url, body)
		if rec.Code != 200 {
			t.Fatalf("%s %s: %d %s", method, url, rec.Code, rec.Body)
		}
		var res struct {
			Applied bool
			Status  *string
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res.Applied, res.Status
	}

	// Done logged by hand survives n8n's planning and close-out.
	write("PUT", "/api/days/2026-11-20/dance", `{"status":"done"}`)
	if ok, st := write("PUT", "/api/days/2026-11-20/dance?if=blank", `{"status":"planned"}`); ok || *st != "done" {
		t.Fatalf("plan over done: %v %v", ok, st)
	}
	if ok, st := write("PUT", "/api/days/2026-11-20/dance?if=planned", `{"status":"missed"}`); ok || *st != "done" {
		t.Fatalf("close out done: %v %v", ok, st)
	}

	// The close-out finds what is still planned before today and resolves it.
	write("PUT", "/api/days/2026-11-19/workout?if=blank", `{"status":"planned"}`)
	write("PUT", "/api/days/2026-11-21/workout?if=blank", `{"status":"planned"}`)
	rec := do(t, h, "GET", "/api/days?status=planned&before=2026-11-21", "")
	var list struct {
		Days []struct{ Date, Habit, Status string }
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Days) != 1 || list.Days[0].Date != "2026-11-19" || list.Days[0].Habit != "workout" {
		t.Fatalf("planned: %s", rec.Body)
	}
	if ok, st := write("PUT", "/api/days/2026-11-19/workout?if=planned", `{"status":null}`); !ok || st != nil {
		t.Fatalf("clear planned: %v %v", ok, st)
	}

	// DELETE clears a day too.
	if ok, st := write("DELETE", "/api/days/2026-11-20/dance", ""); !ok || st != nil {
		t.Fatalf("delete: %v %v", ok, st)
	}
}

func TestWriteSource(t *testing.T) {
	h := newServer(t, "2026-11-21 12:00")
	do(t, h, "PUT", "/api/days/2026-11-20/workout", `{"status":"planned"}`) // n8n: no browser headers
	req := httptest.NewRequest("PUT", "/api/days/2026-11-21/workout", strings.NewReader(`{"status":"done"}`))
	req.Header.Set("Sec-Fetch-Site", "same-origin") // the PWA
	h.ServeHTTP(httptest.NewRecorder(), req)

	rec := do(t, h, "GET", "/api/week", "")
	var week struct {
		Habits []struct {
			Days []struct{ Source *string }
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &week); err != nil {
		t.Fatal(err)
	}
	days := week.Habits[0].Days
	if *days[4].Source != "n8n" || *days[5].Source != "app" || days[6].Source != nil {
		t.Fatalf("sources: %s", rec.Body)
	}
	// Clients can't claim a source.
	if rec := do(t, h, "PUT", "/api/days/2026-11-19/workout", `{"status":"done","source":"app"}`); rec.Code != 400 {
		t.Fatalf("source in body: %d", rec.Code)
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
		{"PUT", "/api/days/2026-11-16/workout", `{"status":"done","note":"Upper-body"}`, 400},
		{"PUT", "/api/days/2026-11-16/meditation", `{"status":"done"}`, 404},
		{"PUT", "/api/days/2026-11-16/workout?if=empty", `{"status":"done"}`, 400},
		{"GET", "/api/days", "", 400},
		{"GET", "/api/days?status=planned&before=today", "", 400},
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
	// Unknown paths get the SPA shell, so client routes work on reload.
	if rec := do(t, h, "GET", "/somewhere", ""); rec.Code != 200 || !strings.Contains(rec.Body.String(), "loom") {
		t.Fatalf("fallback: %d", rec.Code)
	}
	rec := do(t, h, "GET", "/_app/immutable/app.js", "")
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("immutable: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	rec = do(t, h, "GET", "/service-worker.js", "")
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("service worker: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := do(t, h, "POST", "/", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post: %d", rec.Code)
	}
}
