# wj — collaborative work journal (design & plan)

Status: **in progress** on branch `feat/collab`. Target release: **0.12.0**.

Turn wj from a single-user tracker into a shared, multi-person work journal —
**without a server**, keeping the plain-files / append-only design intact.

---

## Why this is tractable for wj specifically

Two properties wj already has make collaboration almost merge-logic-free:

1. **Append-only log + actor-partitioned files.** Each person writes their *own*
   per-day file (`YYYY/MM/DD.<actor>.tsv`); a reader **unions** every `DD.*.tsv`
   for the day. Two people working the same day never touch the same file, so
   git has nothing to conflict on.
2. **Git's built-in `union` merge driver.** A `.gitattributes` rule
   `*.tsv merge=union` keeps *all* lines from both sides if the same file is ever
   edited from two machines — the textbook case for append-only logs.

So "collaborative wj" ≈ **add an author to events + sync the folder with git +
union the reads.** No CRDTs, no server, no conflict UI.

---

## Decisions (locked)

- **Transport:** serverless, **git** (`$WJ_DATA_DIR` is a repo; sync = pull/push).
- **Storage:** actor-partitioned append-only files `YYYY/MM/DD.<actor>.tsv`.
- **Reads:** union all `DD.*.tsv`; tasks keyed by **(actor, id)**.
- **Identity:** `actor` config, default `git config user.email` → `$USER`.
- **IDs:** friendly per-person `T1/T2…`; bare = yours, `alice/T1` = someone else's.
- **Default view = "mine"**; team/overall views are explicit (`wj team`, `--all`,
  `--by person`). The merged log makes the overall view natural — e.g. a `gantt`
  project row already rolls up everyone's time on that project.
- **Backward compatible:** single-user wj behaves identically (you are one actor);
  legacy `DD.tsv` is read as your own history.

## Auth model

- **ssh-agent** (common): unlock once per session → sync is silent thereafter.
- **Manual `wj sync`** from a shell may prompt interactively (fine).
- **Auto / TUI sync** runs non-interactive with a timeout; if auth isn't ready it
  **pauses syncing and says so — never hangs, never corrupts the TUI**, and local
  work continues. Catches up on the next sync.
- **Fully unattended** option: HTTPS + token in the OS keyring, or a passphrase-less
  deploy key.

---

## Build sequence (each step ships on its own; suite stays green throughout)

### Phase 1 — read-mostly core

**1a · Foundation** *(data-model change — built first, alone, no visible change)*
- [x] `actor` config + normalization; `cur_actor()` helper; `_CFG_KEYS` + seed.
- [x] Writes target `DD.<actor>.tsv`; legacy `DD.tsv` read as the local actor.
- [x] `replay_day` unions all `DD.*.tsv` for the day, keys tasks by `(actor, id)`.
- [x] Reads display bare `T#` for your own, `actor/T#` for others (`disp_id`);
      helpers resolve either via `qkey`; actions are scoped to your own tasks.
- [x] Single-user verified byte-identical (Go integration test + seed smoke green).
- [x] Multi-actor union verified (two authors, one day; cross-person rollups).
- [ ] `actor/T1` accepted by the mutating commands (currently own-tasks only).
- [ ] Migration: going-collaborative renames legacy `DD.tsv` → `DD.<actor>.tsv` (in 1c).
- [ ] Add automated multi-actor regression tests.

**1b · Owner in the contract + views** ✅
- [x] `actor` in status/grid/show/search JSON → Go types (`id_actor` on the CLI).
- [x] TUI loads its own actor (`wj _actor`); tints teammate tasks by author and
      shows their ids qualified (`alice/T3`); `show` accepts a qualified id.
- [x] Mutations gated to your own tasks — a teammate's task is read-only (notice,
      not a CLI error). Mutating commands stay bare-id/own-only by design.
- [x] Personal surfaces (header, today rollup, Today panel, active project) scoped
      to *your* live tasks via `myTasks()` — team rollups stay in the Range/Window.
- [ ] Owner colour in the Day-gantt rows (only the Tasks list tints today).

**1c · `wj sync`** ✅
- [x] `wj sync` = add/commit + pull --rebase + push; non-interactive (BatchMode +
      timeout + `GIT_TERMINAL_PROMPT=0`) so it never hangs, reports auth/network
      errors cleanly. `wj sync status` shows ahead/behind.
- [x] `wj sync init <remote>` one-liner (init + union `.gitattributes` + legacy
      rename + remote + first push/pull, incl. `--allow-unrelated-histories`).
- [x] Legacy `DD.tsv` → `DD.<actor>.tsv` migration (folds same-day files in).
- [x] TUI `S` triggers sync (non-interactive → never hangs the UI).
- [x] Verified end-to-end: two-user union sync + same-author union-merge (no lost
      events) against a real bare remote.

**1d · Team views** ✅
- [x] `wj team` (text + `--json`) — each author's running task (or idle) + their
      day total; you are marked `(you)`.
- [x] `report --by person` — range rollup by author (`id_actor` key).
- [ ] `gantt --by person` + the TUI `b` cycle including person (Phase 3 polish).
- [ ] `--mine` / `--all` read filters (today the views show the union).

### Phase 2 — shared backlog & assignment
- [ ] `wj assign P3 bob`, `pending --mine`, actor-partitioned pending.

### Phase 3 — liveness & polish
- [ ] Background auto-sync on a tick; TUI team/presence panel; filter-by-person.

---

## Risks & mitigations

- **You on two machines, same day, pre-sync** → two "your `T2`"s. Mitigate: sync
  often; union-merge keeps both; optionally fold a machine tag into the id. *(The
  one genuinely sharp edge — settle the paranoia level inside 1a.)*
- **Pending reorder conflicts** → Phase 2; actor-partition pending or last-writer-wins.
- **Privacy** → the synced repo is the *team* journal; private work lives in a
  separate non-synced `WJ_DATA_DIR`. Per-project privacy is a later refinement.
- **Clock skew** between machines → cosmetic for a journal; per-person ordering is fine.

## Logistics
- Built on `feat/collab`, phase by phase; lands as **0.12.0**.
- `wjdev` gains a multi-actor seed mode to test team views locally.
