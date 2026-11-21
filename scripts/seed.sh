#!/usr/bin/env bash
# Fill a local loom with sample data for the current week: ./scripts/seed.sh [base-url]
# Pretends to be n8n. Never point it at the real instance.
set -euo pipefail

BASE="${1:-http://localhost:8080}"
case "$BASE" in
  http://localhost:*|http://127.0.0.1:*) ;;
  *) echo "refusing to seed $BASE: local URLs only" >&2; exit 1 ;;
esac

put() { curl -fsS -X PUT -H 'Content-Type: application/json' --data "$2" "$BASE$1" >/dev/null; }

# offset N: the date N days after this week's Monday (BSD or GNU date).
mon=$(( 1 - $(date +%u) ))
offset() {
  local n=$(( mon + $1 )) sign=""
  (( n >= 0 )) && sign="+"
  date -v"${sign}${n}d" +%F 2>/dev/null || date -d "$n days" +%F
}

put "/api/days/$(offset 0)/workout" '{"status":"done","note":"Upper-body"}'
put "/api/days/$(offset 2)/workout" '{"status":"missed"}'
put "/api/days/$(offset 4)/workout" '{"status":"done","note":"Lower-body"}'
put "/api/days/$(offset 6)/workout" '{"status":"planned"}'
put "/api/days/$(offset 1)/dance" '{"status":"done"}'
put "/api/days/$(offset 5)/dance" '{"status":"planned"}'
for i in 0 1 3 4; do put "/api/days/$(offset $i)/meditation" '{"status":"done","value":12}'; done
put "/api/days/$(offset 2)/meditation" '{"value":4}'
for i in 0 2 3; do put "/api/days/$(offset $i)/writing" '{"status":"done"}'; done
# Last week, for the comparison.
put "/api/days/$(offset -7)/workout" '{"status":"done"}'
put "/api/days/$(offset -5)/workout" '{"status":"missed"}'
put "/api/days/$(offset -3)/dance" '{"status":"missed"}'

put /api/goals/short_term '{"goals":[
  {"id":"s1","name":"Bench 100 kg","type":"Performance","priority":"high","progress":0.92,"due":"2027-03-31","url":"https://www.notion.so/s1"},
  {"id":"s2","name":"Read 24 books","type":"Outcome","priority":"mid","progress":0.79,"due":"2026-12-31","url":"https://www.notion.so/s2"},
  {"id":"s3","name":"Learn a new dance routine","priority":"low","progress":0.3,"url":"https://www.notion.so/s3"}
]}'
put /api/goals/perennial '{"goals":[
  {"id":"p1","name":"Train four times a week","type":"Performance","priority":"high","progress":0.75,"url":"https://www.notion.so/p1"},
  {"id":"p2","name":"Write every day","type":"Performance","priority":"mid","progress":0.5,"url":"https://www.notion.so/p2"}
]}'
echo "seeded $BASE"
