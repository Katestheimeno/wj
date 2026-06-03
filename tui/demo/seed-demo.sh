#!/usr/bin/env bash
# seed-demo.sh — populate a throwaway wj data dir with demo entries via the real
# CLI, so you can explore wj-tui (or the plain CLI) without touching your own log.
#
#   ./tui/demo/seed-demo.sh            # seed /tmp/wj-demo, then print how to launch
#   WJ=~/.local/bin/wj ./tui/demo/seed-demo.sh
#
# Then launch the (freshly built) TUI against that data:
#
#   WJ_DATA_DIR=/tmp/wj-demo WJ_CONFIG=/tmp/wj-demo/config ./tui/wj-tui
#
# The days are reconstructed retroactively with --date/--at, anchored to *today*
# so they always land in the last work-week and today's tasks are left running
# (so the header clock ticks). Projects/tasks are deliberately generic.
set -euo pipefail

WJ="${WJ:-wj}"
export WJ_DATA_DIR="${WJ_DATA_DIR:-/tmp/wj-demo}"
export WJ_CONFIG="${WJ_CONFIG:-$WJ_DATA_DIR/config}"

rm -rf "$WJ_DATA_DIR"        # start clean every run
mkdir -p "$WJ_DATA_DIR"

# Pin a config so the demo is reproducible regardless of where you run it:
# git_tag=off means seeding can't absorb real commits from the surrounding repo.
cat >"$WJ_CONFIG" <<'CFG'
# wj demo config — wj's defaults, but with commit-tagging off so the seed is
# deterministic no matter which repo you launch it from.
shift_start=09:00
shift_end=19:00
slot_minutes=5
round=down
totals=exact
default_project=admin
git_tag=off
interface=ui
CFG

# `command` bypasses shell-function lookup, so when WJ defaults to "wj" this
# runs the CLI on PATH rather than recursing into this very function.
wj() { command "$WJ" "$@" >/dev/null; }

# dates relative to today, so the demo is always "this past week"
TODAY=$(date +%F)
ago()   { date -d "$TODAY -$1 day" +%F; }   # ago 4  -> four days ago
ahead() { date -d "$TODAY +$1 day" +%F; }   # ahead 1 -> tomorrow

D4=$(ago 4); D3=$(ago 3); D2=$(ago 2); D1=$(ago 1)

# --- four days ago: planning, then a backend stretch and some frontend ---
wj start "Sprint planning"  --project meetings --date "$D4" --at 09:00
wj complete                 --project meetings --date "$D4" --at 09:45
wj start "Refactor auth"    --project backend  --date "$D4" --at 10:00
wj log   "extracted token service"             --date "$D4" --at 11:00
wj complete                 --project backend  --date "$D4" --at 12:30
wj start "Landing page"     --project frontend --date "$D4" --at 13:30
wj complete                 --project frontend --date "$D4" --at 15:30

# --- three days ago: two projects at once, plus a pause/resume around review ---
wj start "API endpoints"    --project backend  --date "$D3" --at 09:00
wj start "Standup"          --project meetings --date "$D3" --at 09:30  # runs alongside
wj complete                 --project meetings --date "$D3" --at 09:45
wj pause                    --project backend  --date "$D3" --at 11:00 "blocked on review"
wj start "Wireframes"       --project design   --date "$D3" --at 11:00
wj complete                 --project design   --date "$D3" --at 12:30
wj resume                   --project backend  --date "$D3" --at 13:30
wj complete                 --project backend  --date "$D3" --at 15:00

# --- two days ago: a long infra task with a note + pause, then a deferral ---
wj start "k8s upgrade"      --project infra    --date "$D2" --at 09:00
wj log   "watching the rollout"                --date "$D2" --at 10:30
wj pause                    --project infra    --date "$D2" --at 11:00
wj resume                   --project infra    --date "$D2" --at 11:45
wj complete                 --project infra    --date "$D2" --at 13:30
wj start "Component library" --project frontend --date "$D2" --at 14:00
wj defer                    --project frontend --date "$D2" --at 15:00 "waiting on design"

# --- yesterday: docs, a mis-detected task re-homed with `move`, then research ---
wj start "API docs"         --project docs     --date "$D1" --at 09:00
wj complete                 --project docs     --date "$D1" --at 10:30
wj start "Fix login bug"    --project backend  --date "$D1" --at 10:45
wj move  T2 infra                              --date "$D1" --at 11:00  # actually infra work
wj complete T2                                 --date "$D1" --at 12:00
wj start "Evaluate libraries" --project research --date "$D1" --at 13:30
wj complete                 --project research --date "$D1" --at 15:00

# --- today: standup done, then two projects left RUNNING so the clock ticks ---
wj start "Standup"          --project meetings --date "$TODAY" --at 09:00
wj complete                 --project meetings --date "$TODAY" --at 09:15
wj start "Database migration" --project backend --date "$TODAY" --at 09:30  # still in-progress
wj start "Design review"    --project design   --date "$TODAY" --at 11:00  # concurrent, in-progress

# --- pending backlog: a mix of deadlines (overdue / today / soon / none / later) ---
wj add "Triage support inbox"  --project backend  --due "$D1"          # overdue
wj add "Prepare release notes" --project docs     --due "$TODAY"       # due today
wj add "Renew TLS certificate" --project infra    --due "$(ahead 1)"   # due soon
wj add "Spike: caching layer"  --project research                      # no deadline
wj add "Plan next quarter"     --project meetings --due "$(ahead 17)"  # later

echo "seeded $WJ_DATA_DIR  (5 days, 6 projects, a pending backlog)"
echo
echo "launch the TUI against the demo data (build it first with: make tui):"
echo "  WJ_DATA_DIR=$WJ_DATA_DIR WJ_CONFIG=$WJ_CONFIG ./tui/wj-tui"
echo
echo "or poke at it with the plain CLI, e.g.:"
echo "  WJ_DATA_DIR=$WJ_DATA_DIR WJ_CONFIG=$WJ_CONFIG $WJ gantt"
