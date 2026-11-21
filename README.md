# Loom

A life dashboard you install on your phone as a PWA: this week's habits, how they compare with last week, and your goals.

Loom only displays data. **n8n is the brain**: it works out each habit's status (from the calendar, Apple Health shortcuts and git commits) and copies goals in from Notion. Loom stores what it is sent in SQLite and serves one read-only screen.

```
Calendar, Shortcuts, git ─▶ n8n ─▶ PUT /api/days/...   ─▶ SQLite ─▶ GET /api/week ─▶ PWA
Notion goal databases    ─▶ n8n ─▶ PUT /api/goals/...  ─┘
```

- Go server (`cmd/loom`) with SQLite (`modernc.org/sqlite`, no cgo), shipped as a distroless image.
- Plain HTML/CSS/JS frontend in `internal/ui/static`, embedded in the binary. There's no build step.
- No login of its own. From outside, it's reachable only through Cloudflare Access. n8n calls it over the Docker network.

## Run locally

```bash
make run     # :8080, database in data/loom.db
make seed    # sample data for this week, in another terminal
make test
```

Frontend edits need a server restart because the files are embedded.

## Data model

**Habits** have a key, a name, a position and `plannable`. Plannable habits (Workout, Dance) can be `planned`, `done` or `missed` and count toward adherence. The others (Meditation, Writing) are only ever `done`. The four current habits are created by the first migration.

**Habit days** are one row per habit per tracker day: `status`, an optional `value` (such as mindful minutes) and an optional `note` (such as the workout plan). The tracker day runs from 05:00 to 05:00 (`DAY_START_HOUR`). n8n decides which date something belongs to. Loom uses the boundary only to know which day is "today".

**Goals** are read-only copies of the Notion Short-term Goals and Perennial Goals databases. Progress is the `Progress` you set in Notion (0-1). Tapping a goal opens its Notion page.

**Adherence** is done / (done + missed) over the plannable habits for the week. It stays empty until something planned has been done or missed.

## API

Every request that changes data refuses browser requests from other sites (`Sec-Fetch-Site`). Unknown JSON fields are rejected.

| Method | Path | Body | Notes |
|---|---|---|---|
| `GET` | `/api/week?start=YYYY-MM-DD` | | `start` must be a Monday. Without it, you get the current week. |
| `PUT` | `/api/days/{date}/{habit}` | `{"status": "done", "value": 12, "note": "Upper-body"}` | Upsert. Each field can be `null`, and a row with all three empty is removed. |
| `DELETE` | `/api/days/{date}/{habit}` | | |
| `PUT` | `/api/goals/{short_term\|perennial}` | `{"goals": [{"id", "name", "type", "priority", "progress", "due", "url"}]}` | Replaces that list. `priority` is `high`, `mid` or `low`, and `due` is `YYYY-MM-DD`. |
| `GET` | `/api/goals/{list}` | | |
| `GET` / `PUT` | `/api/habits`, `/api/habits/{key}` | `{"name", "position", "plannable"}` | Add or rename a habit. |
| `GET` | `/api/healthz`, `/api/readyz` | | `loom healthcheck` probes readiness. |

### From n8n

Use an **HTTP Request** node against `http://loom:8080`:

- **Fitness Tracker**: `PUT /api/days/{{date}}/workout` with `{"status": "planned"}` at 05:00, `{"status": "done", "note": "<Workout plan>"}` when the watch sends a workout, and `{"status": "missed"}` (or `{"status": null}` when the event moved) at the next 05:00. Meditation sends `{"status": "done", "value": <mindful min>}` when there are at least 10 minutes, or `{"status": null, "value": <mindful min>}` otherwise.
- **Writing Tracker**: `PUT /api/days/{{date}}/writing` with `{"status": "done"}`.
- **Goals sync** (on a schedule): query each Notion goal database, map `Priority` "High priority" to `high` and so on, and `PUT /api/goals/short_term` and `/api/goals/perennial` with the whole list.

## Configuration

| Env | Default | |
|---|---|---|
| `PORT` | `8080` | |
| `DB_PATH` | `data/loom.db` | `/app/data/loom.db` in the image |
| `TIMEZONE` | `Europe/Stockholm` | Decides which day is "today" |
| `DAY_START_HOUR` | `5` | When the tracker day rolls over |

## Release

Push a `v*` tag to build `ghcr.io/agusespa/loom` for amd64 and arm64 (`:X.Y.Z`, `:X.Y`, `:latest`). The image runs as distroless `nonroot` (uid 65532), so the data directory must be owned by that user.
