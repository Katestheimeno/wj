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

**1b · Owner in the contract + views**
- [ ] `actor` in status/grid/show/gantt/search JSON → Go types.
- [ ] Owner shown in CLI views; TUI colours by person, badges/qualifies foreign ids.

**1c · `wj sync`**
- [ ] `wj sync` = pull --rebase + push, non-interactive + graceful-degradation.
- [ ] Auto-write `.gitattributes` union rule; `wj sync init <remote>` one-liner.

**1d · Team views**
- [ ] `wj team` — who's on what right now (today's in-progress across actors).
- [ ] `report --by person`, `gantt --by person`, a range team journal.
- [ ] `--mine` / `--all` where it matters.

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
