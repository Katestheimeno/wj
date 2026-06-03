# wj — cross-project work journal & time tracker

A single, dependency-free bash CLI for tracking what you work on across every
project. Start, pause, resume, complete and defer tasks; see where your time
went on a slot-by-slot grid; and export everything to CSV/JSON for analysis.

An **optional** terminal UI (`wj-tui`) adds a multi-day Gantt overview with
project colors and an intraday drill-down — see [Terminal UI](#terminal-ui-optional).
The bash CLI is fully self-contained; the UI is a thin front-end over it and is
never required.

![wj-tui — the optional terminal UI: a multi-day Gantt, an intraday drill-down, a pending backlog and a per-task timeline, all driven by the CLI](assets/asset1.png)

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
- **Multi-day overview** — `wj gantt` prints a projects(or tasks)×days matrix of
  time totals right in the terminal (the CLI counterpart of the TUI's Range view).
- **Machine-readable** — `status`, `show`, `grid` and `gantt` accept `--json` for a
  stable contract that tooling (and the `wj-tui` front-end) can consume.
- **Git-aware** — on `complete`, commits made during a task's window are recorded
  automatically (no LLM, no prose).
- **Retroactive** — `--at HH:MM` backfills past times; chain it to reconstruct a
  whole day after the fact.
- **Exportable** — dump the raw event log to `csv`, `json`, or `tsv`, or roll it
  up with `report`.
- **No dependencies** — the CLI is pure bash + coreutils + `git` (only used when
  tagging commits). No `jq`, no database. (The optional `wj-tui` front-end is a
  separate, statically-linked Go binary — needed only if you opt into the UI.)
- **Optional terminal UI** — a lazygit-style `wj-tui` with a colored multi-day
  Gantt and intraday drill-down, driven entirely by the CLI's `--json` output.

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
| `WJ_WITH_UI` | `0` | Set to `1` (or pass `--with-ui`) to also build the UI. |

To install the optional terminal UI as well (needs [Go](https://go.dev/dl)):

```sh
curl -fsSL https://raw.githubusercontent.com/Katestheimeno/wj/main/install.sh | bash -s -- --with-ui
```

### Build from source (with `make`)

Clone the repo and use the Makefile to install both the CLI and the UI (the man
page comes along automatically). Needs Go for the UI.

```sh
git clone https://github.com/Katestheimeno/wj.git && cd wj

make install                 # build wj-tui, install wj + wj-tui + man -> ~/.local
hash -r                      # refresh your shell's command cache

# remove it again (keeps your config & data):
make uninstall

# reinstall:
make install && hash -r
```

Targets: `make install-cli` (just the bash CLI + man), `make install-ui` (just
the UI binary), `make test`, `make clean`. Override the location with
`PREFIX`, e.g. a system-wide install: `sudo make install PREFIX=/usr/local`.

> **Note on PATH:** if an older `wj` is already installed (e.g. a system package
> in `/usr/bin`), it may shadow `~/.local/bin/wj`. Remove the old one first
> (e.g. `sudo pacman -Rns wj`) and run `hash -r`. Check with `command -v wj`.

### Manual install (CLI only, no Go)

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
| `wj start <desc\|P#>` | Begin a new task (next id `T1`, `T2`… per day). Auto-pauses the running task in the same project. Given a pending id (`P#`) it promotes that backlog item instead (see [Pending backlog](#pending-backlog)). |
| `wj pause [id] [why]` | Pause the running task (or a given id). Stops its clock. |
| `wj resume [id]` | Resume the most-recent paused/deferred task (or a given id). |
| `wj complete [id]` | Finish a task; sum its time; record git commits in its window. |
| `wj defer [id] [why]` | Set a task aside (blocked, or for another day). |
| `wj log <note>` | Attach a timestamped note to the running task. |
| `wj amend [id] <desc>` | Replace a task's description (running task, or a given id). Appends an event — history is never rewritten. |
| `wj move [id] <proj>` | Re-home a task to another project (fix wrong auto-detection). |
| `wj cancel [id]` | Void a mistaken task: 0 time, hidden from status/grid/report (kept in the raw log for audit). |
| `wj ls` | List currently-open tasks (in-progress / paused / deferred). Today by default; `--days N` scans the last N days (adds a DATE column) to catch timers left running earlier. |
| `wj show <id>` | Full timeline of one task: start, notes, pauses/resumes, renames, moves, recorded commits, total time and status. Today by default; `--date` for a past day. |
| `wj status [date]` | Per-task totals table for a day (default: today). **Default command.** |
| `wj grid [date]` | Slot-by-slot schedule for a day. |
| `wj gantt [flags]` | Multi-day overview: a rows×days matrix of time totals. Rows are projects (or per-day tasks with `--by task`); columns are days. Default range: the last 7 days through `--to` (or today). Cancelled and zero-time rows are omitted. The CLI counterpart of the TUI's Range view. |
| `wj search <query>` | Find tasks across **all** recorded days by a case-insensitive substring of the id, project, or description. Most-recent first; `--json` for the UI's `/` overlay. |
| `wj report [flags]` | Aggregate time over a date range, grouped by `--by`. |
| `wj export [flags]` | Dump raw events as csv/json/tsv over a date range. |
| `wj ui` | Launch the optional `wj-tui` front-end (see [Terminal UI](#terminal-ui-optional)). A bare `wj` opens it too when `interface=ui`. |
| `wj completion <shell>` | Print a shell-completion script (`bash` or `zsh`). |
| `wj config` | Print the active config file path. |
| `wj version` | Print the version (also `--version`, `-V`). |
| `wj help` | Full help. |

### Flags

| Flag | Applies to | Purpose |
|---|---|---|
| `--at TIME` | start, pause, resume, complete, defer, log, amend, move, cancel, status, grid, show | Act at a past time instead of now. Flexible format — `9`, `930`, `0930`, `9:30`, `9.30`, `9am`, `9pm`, `9:30pm` all normalise to `HH:MM`. Backfills the grid. |
| `--date YYYY-MM-DD` | any write command + status, grid, ls, show | Act on another day, not today (alias `--on`). Combine with `--at` to reconstruct any past day. On a past day **without** `--at`, the time is inferred from that day's last event (or `shift_start`) and the inference is printed. |
| `--project NAME` | start (where the task lives); pause/complete/defer/log/resume/amend/cancel (scope) | Override project detection. Quote names with spaces. |
| `--from D --to D` | report, export, gantt | Inclusive date range `YYYY-MM-DD`. For `gantt`, `--to` (or `--date`/`--on`) is the range end and `--from` the start; if `--from` is omitted it defaults to 6 days before `--to` (last 7 days). For `report`/`export`, default is today. |
| `--by KEY` | report, gantt | Group rows. `report`: `project` \| `task` \| `day`. `gantt`: `project` \| `task`. Default: `project`. |
| `--format FMT` | export | `csv` \| `json` \| `tsv`. Default: `csv`. |
| `--days N` | ls | How many days back to scan for open tasks. Default: `1` (today). |
| `--due YYYY-MM-DD` | add | Optional deadline for a pending task. |
| `--json` | status, show, grid, gantt, search, pending | Emit machine-readable JSON instead of the text table — a stable contract (this is what the `wj-tui` front-end consumes). |

If you omit `--project` on `pause`/`complete`/`log`/`amend`/`move`/`cancel`, the
command acts on whatever is currently running. Pass `--project` to scope it to one
project, or a task id (`T2`) to target a specific task in any state.

The state changes are **idempotent**: repeating an action whose state the task
already holds is a no-op. `pause` on an already-paused task (or `resume` on a
running one, `complete` on a completed one, `defer`/`cancel` likewise) writes
nothing to the log and just prints, e.g., `T1  already paused`.

### Pending backlog

Tasks you intend to do but haven't started yet live in a separate backlog
(`P1`, `P2`… ids, stored in `pending.tsv`). They carry an optional project and
deadline and stay **out of** the time views — status, grid, gantt, reports — until
you start one, at which point it becomes a normal tracked task.

| Command | What it does |
|---|---|
| `wj add <desc>` | Add a backlog task. `--project NAME` sets its project; `--due YYYY-MM-DD` a deadline. |
| `wj pending` | List the backlog in manual (pinned) order. |
| `wj due <P#> <date\|->` | Set, or clear (`-`), a pending task's deadline. |
| `wj raise <P#>` / `wj lower <P#>` | Move a pending task one step up / down. |
| `wj drop <P#>` | Remove a pending task without starting it. |
| `wj start <P#>` | Promote: start the task now (carrying its desc + project) and remove it from the backlog. |

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
| `interface` | `minimal` | Front-end for bare `wj`: `minimal` (status table) or `ui` (launch `wj-tui`). |

Environment overrides:

- `WJ_CONFIG` — path to the config file (default `$XDG_CONFIG_HOME/wj/config`).
- `WJ_DATA_DIR` — root of the data tree (default `$XDG_DATA_HOME/wj`, i.e. `~/.local/share/wj`).

`XDG_CONFIG_HOME` / `XDG_DATA_HOME` are honored as the base for those defaults.

## Data layout

```
~/.config/wj/config                  # settings
~/.local/share/wj/
└── 2026/06/01.tsv                    # one append-only event log per day
```

Project detection order: git remote basename → repo folder name → `default_project`
(lowercased).

## Terminal UI (optional)

`wj-tui` is an optional, lazygit-style front-end. It renders the event log via
the CLI's `--json` output and triggers actions by calling `wj` — so the bash CLI
stays the single source of truth, and the UI can never disagree with it.

The layout fills the whole terminal: a narrow **sidebar** of lists drives a wide
**main** column of detail. The header shows the running task with a live clock
plus a today rollup (`>1 =0 x4 · Σ2h39m`); `?` opens a full keybinding overlay.
Navigation is vim-style — `j`/`k` move within the focused panel, `l`/`h` drill
in/out, `←`/`→` step days, `g`/`G` jump to first/last, `Ctrl-d`/`Ctrl-u`
half-page — and `Tab` cycles every panel.

![The `?` overlay — the full keybinding reference, grouped by panel](assets/asset_help.png)

Sidebar:

- **Projects** — every project (or task) in range with its total. Selecting one
  filters the day's Tasks to it (master→detail); the project running right now
  is flagged `>`. `[`/`]` shift the window, `t` jumps to today, `1`/`7`/`3` set
  the span (1/7/30 days), `b` toggles project/task rows.
- **Tasks** — the focused day's tasks, each led by a status glyph: `>` running,
  `=` paused, `»` deferred, `x` done.
- **Pending** — the [backlog](#pending-backlog): `a` add (`desc @project
  !YYYY-MM-DD`), `d` set/clear the due date, `[`/`]` reorder, `x` drop, `Enter`
  to start (promote). Deadlines are colored by urgency (overdue red, due-soon amber).

Main:

- **Range** — a multi-day Gantt: one row per project (or task), one column per
  day, with project-colored intensity bars.
- **Day** — the focused day's intraday Gantt: a time axis from `shift_start` to
  `shift_end` with a `now ▲` marker and colored segment bars per task.
- **Timeline** — the selected task's full event history.

`/` opens a global **search** overlay: type to filter every task ever recorded
(by id, project, or description); `Enter` jumps to a match, windowing the range
onto its day and selecting it.

![The `/` search overlay — fuzzy-filter every task ever recorded, across all days](assets/asset_search.png)

Mutations run the same commands as the CLI, on the selected task: `p` pause,
`r` resume, `c` complete, `d` defer, `a` amend, `m` move (with `⇥` project
autocomplete), `n` log a note, `x` cancel (with a confirm); `s` starts a
brand-new task. Acting on a **past** day first prompts for a time (`--at`), so an
edit can't collapse to a zero-length interval. Colors are assigned per project
(stable across days, including `--by task` rows) and respect `NO_COLOR`.

Every action echoes the CLI's confirmation in the footer — a cyan `✓` line such
as `✓ T1 12:30 completed — 1h30m`, or, for an [idempotent](#commands) no-op,
`✓ T1 already paused`; failures show in a red `⚠` line instead. The next keypress
dismisses it.

```sh
make install-ui            # build & install wj-tui (needs Go)
wj ui                      # launch it explicitly
# …or set `interface=ui` in the config so a bare `wj` opens it.
```

Install it via `--with-ui` (see [Install](#install)). If `wj-tui` isn't present,
`interface=ui` silently falls back to the status table, and `wj ui` prints a
clear error — the CLI never depends on it.

### Try it with demo data

To explore the UI (or the CLI) without touching your own log, the repo ships a
seed script that reconstructs a sample work-week — five recent days across a
handful of projects, with pauses, a deferral, a re-homed task, two tasks left
running *today* (so the header clock ticks), and a pending backlog — into a
throwaway data dir:

```sh
make tui                       # build ./tui/wj-tui (needs Go)
./tui/demo/seed-demo.sh        # populate /tmp/wj-demo via the real CLI

# launch the UI against the demo data — your real log stays untouched:
WJ_DATA_DIR=/tmp/wj-demo WJ_CONFIG=/tmp/wj-demo/config ./tui/wj-tui
```

The dates are anchored to today, so it's always "this past week". The same two
env vars point the plain CLI at the demo too — e.g.
`WJ_DATA_DIR=/tmp/wj-demo WJ_CONFIG=/tmp/wj-demo/config wj gantt`. Re-run the
script anytime to reset it; `rm -rf /tmp/wj-demo` removes it.

## Analysis & export

```sh
# Multi-day shape of the last week, by project (text matrix):
wj gantt

# A specific window, one row per task, as JSON for tooling:
wj gantt --from 2026-06-01 --to 2026-06-07 --by task --json

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
