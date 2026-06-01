# wj — cross-project work journal & time tracker

A single, dependency-free bash CLI for tracking what you work on across every
project. Start, pause, resume, complete and defer tasks; see where your time
went on a slot-by-slot grid; and export everything to CSV/JSON for analysis.

The source of truth is an **append-only, per-day TSV event log**. Every other
view — status tables, the schedule grid, reports, exports — is *derived* by
replaying that log. Your data stays plain text: greppable, diffable, and trivial
to analyse with `awk`, `sqlite`, a spreadsheet, or a notebook.

```
$ wj start "Refactor auth" --project backend
T1  09:00  [backend]  started T1 — Refactor auth

$ wj start "Standup" --project meetings      # different project → runs alongside
T2  09:30  [meetings]  started T2 — Standup

$ wj complete --project meetings
T2  09:45  completed — 15m

$ wj status

Tasks — 2026-06-01

ID    PROJECT           STATUS        TIME     DESCRIPTION
----  ----------------  ------------  -------  -----------
T1    backend           in-progress   1h00m    Refactor auth
T2    meetings          completed     15m      Standup

Total tracked: 1h15m
```

## Features

- **Cross-project** — one task list per day, shared across all your repos.
- **Concurrency-aware** — one running task *per project*, but different projects
  can run at the same time (each is its own grid column).
- **Time grid** — a configurable slot grid (default 5-minute "jumps", 09:00–19:00)
  visualises the shape of your day.
- **Git-aware** — on `complete`, commits made during a task's window are recorded
  automatically (no LLM, no prose).
- **Retroactive** — `--at HH:MM` backfills past times; chain it to reconstruct a
  whole day after the fact.
- **Exportable** — dump the raw event log to `csv`, `json`, or `tsv`, or roll it
  up with `report`.
- **No dependencies** — pure bash + coreutils + `git` (only used when tagging
  commits). No `jq`, no database.

## Install

One line — no clone needed:

```sh
curl -fsSL https://raw.githubusercontent.com/Katestheimeno/wj/main/install.sh | bash
```

This fetches `wj` and installs it to `~/.local/bin/wj`. If that dir isn't on your
`PATH`, the installer tells you the line to add to your shell rc.

> Prefer not to pipe to `bash`? Download `install.sh`, read it, then run it.

**Options** (pass after `bash -s --`, or as flags when running the script directly):

```sh
# Uninstall (keeps your config & data)
curl -fsSL https://raw.githubusercontent.com/Katestheimeno/wj/main/install.sh | bash -s -- --uninstall

# Uninstall and delete config + data too
curl -fsSL https://raw.githubusercontent.com/Katestheimeno/wj/main/install.sh | bash -s -- --uninstall --purge
```

| Env var | Default | Purpose |
|---|---|---|
| `WJ_BIN_DIR` | `~/.local/bin` | Where to install/remove the binary. |
| `WJ_REF` | `main` | Git branch/tag to install from. |

### Manual install

```sh
git clone https://github.com/Katestheimeno/wj.git && cd wj
chmod +x wj
ln -s "$PWD/wj" ~/.local/bin/wj          # or: sudo ln -s "$PWD/wj" /usr/local/bin/wj
```

First run seeds a config file at `~/.config/wj/config`. Data is written under
`~/.local/share/wj/`. Both locations are overridable (see [Configuration](#configuration)).

## Commands

| Command | What it does |
|---|---|
| `wj start <desc>` | Begin a new task (next id `T1`, `T2`… per day). Auto-pauses the running task in the same project. |
| `wj pause [id] [why]` | Pause the running task (or a given id). Stops its clock. |
| `wj resume [id]` | Resume the most-recent paused/deferred task (or a given id). |
| `wj complete [id]` | Finish a task; sum its time; record git commits in its window. |
| `wj defer [id] [why]` | Set a task aside (blocked, or for another day). |
| `wj log <note>` | Attach a timestamped note to the running task. |
| `wj amend [id] <desc>` | Replace a task's description (running task, or a given id). Appends an event — history is never rewritten. |
| `wj move [id] <proj>` | Re-home a task to another project (fix wrong auto-detection). |
| `wj cancel [id]` | Void a mistaken task: 0 time, hidden from status/grid/report (kept in the raw log for audit). |
| `wj status [date]` | Per-task totals table for a day (default: today). **Default command.** |
| `wj grid [date]` | Slot-by-slot schedule for a day. |
| `wj report [flags]` | Aggregate time over a date range, grouped by `--by`. |
| `wj export [flags]` | Dump raw events as csv/json/tsv over a date range. |
| `wj completion <shell>` | Print a shell-completion script (`bash` or `zsh`). |
| `wj config` | Print the active config file path. |
| `wj help` | Full help. |

### Flags

| Flag | Applies to | Purpose |
|---|---|---|
| `--at HH:MM` | start, pause, resume, complete, defer, log, amend, move, cancel, status, grid | Act at a past time instead of now (24h). Backfills the grid. |
| `--date YYYY-MM-DD` | any write command + status, grid | Act on another day, not today (alias `--on`). Combine with `--at` to reconstruct any past day. |
| `--project NAME` | start (where the task lives); pause/complete/defer/log/resume/amend/cancel (scope) | Override project detection. Quote names with spaces. |
| `--from D --to D` | report, export | Inclusive date range `YYYY-MM-DD`. Default: today. |
| `--by KEY` | report | Group by `project` \| `task` \| `day`. Default: `project`. |
| `--format FMT` | export | `csv` \| `json` \| `tsv`. Default: `csv`. |

If you omit `--project` on `pause`/`complete`/`log`/`amend`/`move`/`cancel`, the
command acts on whatever is currently running. Pass `--project` to scope it to one
project, or a task id (`T2`) to target a specific task in any state.

**Shell completion:** add `eval "$(wj completion bash)"` to your `~/.bashrc` (or
`wj completion zsh` to `~/.zshrc`) to complete commands, flags, task ids and
project names.

## How it works

### The event log

Each day is one tab-separated file. Every action appends one row; nothing is ever
rewritten:

```
timestamp           task_id  project        event     note
2026-06-01T09:00    T1       backend        start     Refactor auth
2026-06-01T09:30    T1       backend        pause     auto-paused (started new task)
2026-06-01T09:30    T2       meetings       start     Standup
2026-06-01T09:45    T2       meetings       complete
2026-06-01T11:00    T1       backend        resume
2026-06-01T11:30    T1       backend        complete
2026-06-01T11:30    T1       backend        commit    89a5697 wire up token refresh
```

Events: `start` · `pause` · `resume` · `complete` · `defer` · `log` · `amend` · `move` · `cancel` · `commit`.
A task's description is the note on its `start` event; its status, time totals and
grid placement are all computed by replaying the rows.

### Statuses & grid symbols

| Status | Grid |
|---|---|
| in-progress | `>>` |
| paused | `--` |
| completed | `**` |
| deferred | `~~` |
| idle | _(blank)_ |

### Time totals

Totals sum the active intervals between `start`/`resume` and `pause`/`complete`/`defer`.
With `totals=exact` (default) they're exact to the minute; with `totals=slot` each
total rounds up to a whole `slot_minutes`. A still-running task counts up to "now"
(or `shift_end` when you view a past day). The grid is always slot-aligned for
display, independent of how totals are summed.

## Configuration

`~/.config/wj/config` (seeded on first run; `key=value`, shell-sourced):

| Key | Default | Meaning |
|---|---|---|
| `shift_start` / `shift_end` | `09:00` / `19:00` | Working-day bounds drawn by the grid. |
| `slot_minutes` | `5` | Grid time-step — the "jump" between slots. |
| `round` | `down` | Grid snapping of event times: `down` or `nearest`. |
| `totals` | `exact` | Time summing: `exact` minutes or `slot`-rounded. |
| `default_project` | `admin` | Project used outside a git repo when no `--project` is given. |
| `git_tag` | `on` | Record commits in a task's window on `complete`. |

Environment overrides:

- `WJ_CONFIG` — path to the config file.
- `WJ_DATA_DIR` — root of the data tree (defaults to `~/.local/share/wj`).

## Data layout

```
~/.config/wj/config                  # settings
~/.local/share/wj/
└── 2026/06/01.tsv                    # one append-only event log per day
```

Project detection order: git remote basename → repo folder name → `default_project`
(lowercased).

## Analysis & export

```sh
# Where did June go, by project?
wj report --from 2026-06-01 --to 2026-06-30 --by project

# Per-day breakdown:
wj report --from 2026-06-01 --to 2026-06-30 --by day

# Raw events to CSV for a spreadsheet / notebook:
wj export --from 2026-06-01 --to 2026-06-30 --format csv > june.csv

# Or query the TSV directly — it's just text:
awk -F'\t' '$4=="complete"{print $3}' ~/.local/share/wj/2026/06/*.tsv | sort | uniq -c
```

`export` emits well-formed CSV (RFC-4180 quoting) and JSON (validates as a JSON
array), so it drops straight into pandas, `jq`, SQLite, or any spreadsheet.

## Retroactive entry

`--at` lets you log work after the fact; chain it to rebuild a whole morning:

```sh
wj start "Fixing auth flow" --at 09:00 --project backend
wj pause standup            --at 09:30
wj start "Code review"      --at 09:30 --project reviews
wj complete T2              --at 10:00
wj resume T1                --at 10:00
wj complete                 --at 11:30 --project backend
```

## Design notes

- **Append-only:** commands read the log, then append — they never rewrite it, so
  the history is auditable and safe to sync.
- **Task ids are per-day** (`T1`, `T2`…). Continuing yesterday's work is a fresh
  task today (give it a note); ids are not linked across days.
- **Git window:** the commit window on `complete` spans the task's whole
  start→complete range, including any pauses inside it.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
