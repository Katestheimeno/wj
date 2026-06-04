# Shared setup for the wj bats suite.
#
# Every test runs against a throwaway data dir and config so it never touches a
# real ~/.local/share/wj. Integration tests pin a fixed past day with --date/--at
# so results are deterministic regardless of when the suite runs.

# Absolute path to the wj script under test (repo root is one level up).
WJ="${BATS_TEST_DIRNAME}/../wj"

# A fixed, arbitrary past date used by the integration tests. Using a past day
# (never "today") makes totals deterministic: an open interval is capped at the
# day's last recorded event rather than the moving wall clock.
WJ_DAY="2026-05-30"

setup() {
    WJ_DATA_DIR="$(mktemp -d "${BATS_TMPDIR:-/tmp}/wj-data.XXXXXX")"
    WJ_CONFIG="$(mktemp "${BATS_TMPDIR:-/tmp}/wj-config.XXXXXX")"
    export WJ_DATA_DIR WJ_CONFIG
    : >"$WJ_CONFIG"   # empty config: built-in defaults, shift bounds unset (auto-fit grid)
}

teardown() {
    [ -n "${WJ_DATA_DIR:-}" ] && rm -rf "$WJ_DATA_DIR"
    [ -n "${WJ_CONFIG:-}" ] && rm -f "$WJ_CONFIG"
}

# Run wj with the given args (positional helper to keep tests terse).
wj() { "$WJ" "$@"; }

# Source wj into a clean subshell and call one of its helper functions, e.g.
#   run wj_fn hm_to_min 09:30
# Sourcing in a subshell keeps the script's `set -u` out of the bats process.
wj_fn() { bash -c 'source "$1"; shift; "$@"' _ "$WJ" "$@"; }
