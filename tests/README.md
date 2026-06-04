# wj bash test suite

[bats](https://bats-core.readthedocs.io) tests for the pure-bash `wj` CLI. The
Go TUI has its own tests under `tui/`.

## Running

```sh
make test-cli          # needs `bats` on PATH
# or directly:
bats tests/
```

Install bats-core via your package manager (`pacman -S bash-bats`,
`brew install bats-core`, `apt install bats`) or from
<https://github.com/bats-core/bats-core>.

## Layout

| File | Covers |
|---|---|
| `unit_helpers.bats` | Pure helpers: time parsing/formatting, slot snapping, project & date normalization. |
| `tracking.bats` | The tracking lifecycle and time derivation (start/pause/resume/complete/cancel/amend/move, slot rounding, JSON). |
| `grid.bats` | The grid/gantt window: the shift-bound default frame, auto-expansion for out-of-hours work, and auto-fit when the bounds are unset. |
| `errors.bats` | Loud failures on unknown ids / bad input, and idempotent no-ops. |
| `concurrency.bats` | Parallel vs `--auto-pause` semantics and the write-path lock. |
| `pending.bats` | The pending backlog: add/list/due/reorder/drop/promote. |
| `test_helper.bash` | Shared setup: throwaway data dir/config, helpers. |

Integration tests pin a fixed past day with `--date`/`--at` so totals are
deterministic regardless of when the suite runs. Each test gets its own
throwaway `WJ_DATA_DIR`/`WJ_CONFIG`, so real data is never touched.
