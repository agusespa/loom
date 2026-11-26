<p align="center"> <img src="web/static/icons/icon-192-76e5359a3105.png" alt="Loom icon: six coloured threads rising as antlers from a geometric reindeer head" width="128"> </p>

# Loom

A life dashboard you install on your phone as a PWA: this week's habits, how they compare with last week, and your goals.

Loom is where habit days live. **Your automations do the tracking**: an n8n workflow (or anything else that can make HTTP requests) works out each habit's status from the sources you connect, such as a calendar, a fitness app or a phone shortcut, and copies your goals in from wherever you keep them. When an automation gets a day wrong, or something wasn't recorded, you fix it in the app: tap a day and pick a status.

```
Calendar, apps, shortcuts ─▶ automation ─▶ PUT /api/days/...   ─▶ SQLite ─▶ GET /api/week ─▶ PWA
Goal tracker              ─▶ automation ─▶ PUT /api/goals/...  ─┤
Corrections in the app                  ─▶ PUT /api/days/...   ─┘
```

Loom has no login of its own. Put it behind an access proxy such as Cloudflare Access, which is what protects edits made in the app, and let your automations call it over a private network.

## Install

Run `ghcr.io/agusespa/loom` (amd64 and arm64; tags `:X.Y.Z`, `:X.Y` and `:latest`). The version shows in the footer. The image runs as distroless `nonroot` (uid 65532), so the data directory must be owned by that user.

## Data model

**Habits** have a key, a name, a position, `plannable` and `archived`. Plannable habits (say, a workout you schedule) can be `planned`, `done` or `missed` and count toward adherence. Other habits (say, a daily walk) are only ever `done`. Habits are never deleted: an archived habit leaves the dashboard (grid, adherence, insights) but keeps its history.

**Habit days** hold a `status` per habit per tracker day, and a `source` saying who set it: a correction made in the app, or an automation. Loom works the source out itself (browsers send `Sec-Fetch-Site`, and the access proxy adds the signed-in user; automations carry neither), so clients can't claim one. A blank day has no row. The tracker day runs from 05:00 to 05:00 (`DAY_START_HOUR`), so something done after midnight still counts for the day before. Your automations decide which date something belongs to. Loom uses the boundary only to know which day is "today".

The tables are `STRICT` and carry their own checks (key and date formats, ranges, allowed values), so a bad write is refused even if it gets past the Go validation.

**Goals** are read-only copies of goals you keep elsewhere. Progress is a number from 0 to 1. Tapping a goal opens its `url`.

**Adherence** is done / (done + missed) over the plannable habits for the week. It stays empty until something planned has been done or missed. The insight cards under the header count every habit over the last 8 weeks instead: a plannable habit's chances are its sessions done or missed, any other habit's are the days since it was first done. Tap them for each habit's streak and adherence.

## Configuration

| Env | Default | |
|---|---|---|
| `PORT` | `8080` | |
| `DB_PATH` | `data/loom.db` | `/app/data/loom.db` in the image |
| `TIMEZONE` | `Europe/Stockholm` | Decides which day is "today" |
| `DAY_START_HOUR` | `5` | When the tracker day rolls over |
