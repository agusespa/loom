<p align="center"> <img src="web/static/icon-192.png" alt="Loom icon: five coloured threads merging into one trunk, like a tree" width="128"> </p>

# Loom

A life dashboard you install on your phone as a PWA: this week's habits, how they compare with last week, and your goals.

Loom is where habit days live. **n8n does the tracking**: it works out each habit's status (from the calendar, the gym app, an iPhone Shortcut and git commits) and copies goals in from Notion. When an automation gets a day wrong, or a session wasn't recorded, you fix it in the app: tap a day and pick a status.

```
Calendar, gym app, Shortcut, git ─▶ n8n ─▶ PUT /api/days/...   ─▶ SQLite ─▶ GET /api/week ─▶ PWA
Notion goal databases            ─▶ n8n ─▶ PUT /api/goals/...  ─┤
Corrections in the app                  ─▶ PUT /api/days/...   ─┘
```

Loom has no login of its own. From outside, it's reachable only through Cloudflare Access, which is what protects edits made in the app. n8n calls it over the Docker network.

## Install

Run `ghcr.io/agusespa/loom` (amd64 and arm64; tags `:X.Y.Z`, `:X.Y` and `:latest`). The version shows in the footer. The image runs as distroless `nonroot` (uid 65532), so the data directory must be owned by that user.

## Data model

**Habits** have a key, a name, a position, `plannable` and `archived`. Plannable habits (Workout, Dance) can be `planned`, `done` or `missed` and count toward adherence. Writing is only ever `done`. The schema creates these three. Habits are never deleted: an archived habit leaves the dashboard (grid, adherence, insights) but keeps its history.

**Habit days** hold a `status` per habit per tracker day, and a `source` saying who set it: `app` for a correction made in the PWA, `n8n` for an automation. Loom works the source out itself (browsers send `Sec-Fetch-Site`, and Cloudflare Access adds the signed-in user; n8n's calls carry neither), so clients can't claim one. A blank day has no row. The tracker day runs from 05:00 to 05:00 (`DAY_START_HOUR`). n8n decides which date something belongs to. Loom uses the boundary only to know which day is "today".

The tables are `STRICT` and carry their own checks (key and date formats, ranges, allowed values), so a bad write is refused even if it gets past the Go validation.

**Goals** are read-only copies of the Notion Short-term Goals and Perennial Goals databases. Progress is the `Progress` you set in Notion (0-1). Tapping a goal opens its Notion page.

**Adherence** is done / (done + missed) over the plannable habits for the week. It stays empty until something planned has been done or missed.

## API

Every request that changes data refuses browser requests from other sites (`Sec-Fetch-Site`). Unknown JSON fields are rejected.

| Method | Path | Body | Notes |
|---|---|---|---|
| `GET` | `/api/week?start=YYYY-MM-DD` | | `start` must be a Monday. Without it, you get the current week. |
| `PUT` | `/api/days/{date}/{habit}[?if=blank,planned]` | `{"status": "done"}` | Sets the day; `null` clears it. With `?if=`, it writes only when the current status is one of those listed (`blank` = none). Answers `{"date", "habit", "status", "applied"}`. |
| `DELETE` | `/api/days/{date}/{habit}` | | Same as PUT with `null`. |
| `GET` | `/api/days?status=planned&before=YYYY-MM-DD` | | Logged days with that status, oldest first. `before` is optional and exclusive. |
| `PUT` | `/api/goals/{short_term\|perennial}` | `{"goals": [{"id", "name", "type", "priority", "progress", "due", "url"}]}` | Replaces that list. `priority` is `high`, `mid` or `low`, and `due` is `YYYY-MM-DD`. |
| `GET` | `/api/goals/{list}` | | |
| `GET` / `PUT` | `/api/habits`, `/api/habits/{key}` | `{"name", "position", "plannable", "archived"}` | Add, rename or archive a habit. |
| `GET` | `/api/healthz`, `/api/readyz` | | `loom healthcheck` probes readiness. |

### From n8n

Use an **HTTP Request** node against `http://loom:8080`:

The 05:00 jobs use `?if=` so they never undo a day you corrected by hand. Marking a day done needs no condition, since done is always right once it happened.

- **Fitness Tracker, 05:00**: for each habit with a calendar event today, `PUT /api/days/{{today}}/{habit}?if=blank` with `{"status": "planned"}`. Then `GET /api/days?status=planned&before={{today}}` and, for each day, `PUT ...?if=planned` with `{"status": "missed"}` if its event is still on that day, or `{"status": null}` if it moved or was deleted.
- **Fitness Tracker, webhooks**: a saved gym workout or a Shortcut dance sends `PUT /api/days/{{date}}/workout` (or `dance`) with `{"status": "done"}`. No condition: done replaces planned or missed.
- **Writing Tracker**: `PUT /api/days/{{date}}/writing` with `{"status": "done"}`.
- **Goals sync** (on a schedule): query each Notion goal database, map `Priority` "High priority" to `high` and so on, and `PUT /api/goals/short_term` and `/api/goals/perennial` with the whole list.

## Configuration

| Env | Default | |
|---|---|---|
| `PORT` | `8080` | |
| `DB_PATH` | `data/loom.db` | `/app/data/loom.db` in the image |
| `TIMEZONE` | `Europe/Stockholm` | Decides which day is "today" |
| `DAY_START_HOUR` | `5` | When the tracker day rolls over |

