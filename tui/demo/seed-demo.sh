#!/usr/bin/env bash
# seed-demo.sh — populate a throwaway wj data dir with demo entries via the real
# CLI, so you can explore wj-tui without touching your own log.
#
#   ./tui/demo/seed-demo.sh            # seed /tmp/wj-demo, then print how to launch
#   WJ=~/.local/bin/wj ./tui/demo/seed-demo.sh
#
# Then launch the (freshly built) TUI against that data:
#
#   WJ_DATA_DIR=/tmp/wj-demo WJ_CONFIG=/tmp/wj-demo/config ./tui/wj-tui
#
# Everything below uses --date/--at so the days are reconstructed retroactively;
# today's last task is left running so the header clock ticks.
set -euo pipefail

WJ="${WJ:-wj}"
export WJ_DATA_DIR="${WJ_DATA_DIR:-/tmp/wj-demo}"
export WJ_CONFIG="${WJ_CONFIG:-$WJ_DATA_DIR/config}"

rm -rf "$WJ_DATA_DIR"        # start clean every run
wj() { "$WJ" "$@" >/dev/null; }

# 2026-05-28 — Acme onboarding + a bit of Infra
wj start "User onboarding"      --project Acme  --date 2026-05-28 --at 09:00
wj complete T1                  --date 2026-05-28 --at 12:20
wj start "Server provisioning"  --project Infra --date 2026-05-28 --at 13:00
wj complete T2                  --date 2026-05-28 --at 14:15

# 2026-05-30 — MarocVentes redesign + an Ideas note
wj start "Landing redesign"     --project MarocVentes --date 2026-05-30 --at 09:30
wj complete T1                  --date 2026-05-30 --at 11:00
wj start "Blog outline"         --project Ideas       --date 2026-05-30 --at 14:00
wj complete T2                  --date 2026-05-30 --at 14:45

# 2026-05-31 — a long Infra task with a pause/resume + a note
wj start "k8s upgrade"          --project Infra --date 2026-05-31 --at 08:30
wj pause T1                     --date 2026-05-31 --at 10:30
wj resume T1                    --date 2026-05-31 --at 11:00
wj log "watching the rollout"   --date 2026-05-31 --at 11:45
wj complete T1                  --date 2026-05-31 --at 13:30

# 2026-06-01 — MarocVentes API + an Acme fix
wj start "API wiring"           --project MarocVentes --date 2026-06-01 --at 09:00
wj log "stubbed the endpoints"  --date 2026-06-01 --at 10:15
wj complete T1                  --date 2026-06-01 --at 11:30
wj start "Invoice fix"          --project Acme        --date 2026-06-01 --at 13:00
wj complete T2                  --date 2026-06-01 --at 14:00

# 2026-06-02 (today) — one finished, one left RUNNING so the clock ticks
wj start "Email triage"         --project Acme        --date 2026-06-02 --at 08:30
wj complete T1                  --date 2026-06-02 --at 08:55
wj start "CSS extraction"       --project MarocVentes --date 2026-06-02 --at 09:00
# (no complete on T2 — it stays in-progress)

echo "seeded $WJ_DATA_DIR"
echo
echo "launch the new TUI with:"
echo "  WJ_DATA_DIR=$WJ_DATA_DIR WJ_CONFIG=$WJ_CONFIG ./tui/wj-tui"
