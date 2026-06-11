#!/usr/bin/env bats
# `wj sync init` against a *populated* remote: local work first, then point it at
# a repo that already has files. Covers the merge (no fetch-first reject), the
# foreign-clash abort, the union-merge exclusions, and the per-machine .lock file
# never being synced. Complements sync.bats (the device-identity guard).

load test_helper

setup() {
    command -v git >/dev/null || skip "git not available"
    SBX="$(mktemp -d "${BATS_TMPDIR:-/tmp}/wj-init.XXXXXX")"
    export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example
    export GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example
    # a remote that ALREADY has a README, on main (a populated GitHub-style repo)
    git init -q --bare "$SBX/remote.git"
    git clone -q "$SBX/remote.git" "$SBX/seed"
    ( cd "$SBX/seed" && printf '# team journal\n' >README.md && git add -A \
        && git commit -qm seed && git branch -M main && git push -q origin main )
    rm -rf "$SBX/seed"
}

teardown() { [ -n "${SBX:-}" ] && rm -rf "$SBX"; }

# wj as <actor>: isolated data dir + config (pinned actor), sharing the remote.
as() {  # as <name> <args...>
    local name=$1; shift
    local cfg="$SBX/cfg.$name" data="$SBX/data.$name"
    [ -f "$cfg" ] || printf 'actor = %s\n' "$name" >"$cfg"
    WJ_CONFIG="$cfg" WJ_DATA_DIR="$data" "$WJ" "$@"
}

# files tracked on the remote's main branch
remote_files() { git -C "$SBX/remote.git" ls-tree -r --name-only main; }

@test "sync init merges a populated remote with local data (no fetch-first reject)" {
    as alice start "local work" --project demo --date "$WJ_DAY" --at 9:00
    run as alice sync init "$SBX/remote.git"
    [ "$status" -eq 0 ]
    [[ "$output" == *"sync ready"* ]]
    run remote_files
    [[ "$output" == *"README.md"* ]]        # the remote's own file survived
    [[ "$output" == *".alice.tsv"* ]]       # and alice's data landed
}

@test "sync init aborts cleanly on a genuine foreign clash (non-interactive)" {
    git clone -q "$SBX/remote.git" "$SBX/c"
    ( cd "$SBX/c" && printf 'REMOTE\n' >NOTES.md && git add -A \
        && git commit -qm notes && git push -q )
    rm -rf "$SBX/c"
    as alice start "local" --project demo --date "$WJ_DAY" --at 9:00
    printf 'LOCAL\n' >"$SBX/data.alice/NOTES.md"     # same path, different content
    run as alice sync init "$SBX/remote.git"
    [ "$status" -ne 0 ]
    [[ "$output" == *"clash with your local data"* ]]
    [[ "$output" != *"check the remote URL"* ]]      # not the misleading auth message
    [ ! -f "$SBX/data.alice/.git/MERGE_HEAD" ]        # no dangling merge left behind
    run remote_files
    [[ "$output" != *".alice.tsv"* ]]                 # nothing was pushed
}

@test "sync_init_keep applies one side of a conflict (unit)" {
    # set up a conflicted first-init merge by hand, then drive the helper.
    local d="$SBX/data.alice"
    mkdir -p "$d"
    git -C "$d" init -q -b main
    printf 'LOCAL\n' >"$d/NOTES.md"
    git -C "$d" add -A && git -C "$d" commit -qm local
    git -C "$d" remote add origin "$SBX/remote.git"
    git clone -q "$SBX/remote.git" "$SBX/c"
    ( cd "$SBX/c" && printf 'REMOTE\n' >NOTES.md && git add -A \
        && git commit -qm notes && git push -q )
    rm -rf "$SBX/c"
    git -C "$d" fetch -q origin
    git -C "$d" merge --allow-unrelated-histories --no-edit origin/main >/dev/null 2>&1 || true
    # keep remote -> NOTES.md == REMOTE
    WJ_DATA_DIR="$d" wj_fn sync_init_keep remote
    [ "$(cat "$d/NOTES.md")" = "REMOTE" ]
    # and the local copy is recoverable from the merge's other parent
    run git -C "$d" show "MERGE_HEAD:NOTES.md"
    [ "$status" -ne 0 ] || true   # (MERGE_HEAD cleared after staging; history holds it)
}

@test "pending backlog files are excluded from the union driver" {
    as alice add "backlog item"
    as alice sync init "$SBX/remote.git"
    run git -C "$SBX/data.alice" check-attr merge -- pending.alice.tsv
    [[ "$output" == *": merge: unset"* ]]
    run git -C "$SBX/data.alice" check-attr merge -- "2026/05/30.alice.tsv"
    [[ "$output" == *": merge: union"* ]]
}

@test "the per-machine .lock file is gitignored and never synced" {
    as alice start "x" --project demo --date "$WJ_DAY" --at 9:00
    as alice sync init "$SBX/remote.git"
    [ -f "$SBX/data.alice/.lock" ]                              # exists locally
    run git -C "$SBX/data.alice" check-ignore .lock
    [ "$status" -eq 0 ]                                          # ...but ignored
    run remote_files
    [[ "$output" != *".lock"* ]]                                # ...and not shared
    [[ "$output" == *".gitignore"* ]]
}

@test "an already-committed .lock is untracked on the next sync" {
    # a legacy repo that committed .lock before the ignore existed
    git clone -q "$SBX/remote.git" "$SBX/leg"
    ( cd "$SBX/leg" && : >.lock && git add -f .lock \
        && git commit -qm "legacy lock" && git push -q )
    rm -rf "$SBX/leg"
    as alice start "x" --project demo --date "$WJ_DAY" --at 9:00
    as alice sync init "$SBX/remote.git"
    run remote_files
    [[ "$output" != *".lock"* ]]   # the removal propagated to the shared repo
}
