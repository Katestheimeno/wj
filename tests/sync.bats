#!/usr/bin/env bats
# Shared-journal identity guard: two *people* who derive the same actor must not
# silently clobber each other. Each clone mints a device id (kept in .git, never
# synced) and registers it against its actor; sync refuses to push when the actor
# is already claimed by another machine, with `wj sync claim` for a same-person
# new machine.

load test_helper

WJ_DAY="2026-05-30"

setup() {
    command -v git >/dev/null || skip "git not available"
    SYNC_TMP="$(mktemp -d "${BATS_TMPDIR:-/tmp}/wj-sync.XXXXXX")"
    git init -q --bare "$SYNC_TMP/remote.git"
    # deterministic git identity for the data-repo commits
    export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@example
    export GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@example
}

teardown() {
    [ -n "${SYNC_TMP:-}" ] && rm -rf "$SYNC_TMP"
}

# run wj as a given "user": its own data dir + config (with a pinned actor).
as_user() {  # as_user <name> <actor> <args...>
    local name=$1 actor=$2; shift 2
    local cfg="$SYNC_TMP/cfg.$name" data="$SYNC_TMP/data.$name"
    printf 'actor = %s\n' "$actor" >"$cfg"
    WJ_CONFIG="$cfg" WJ_DATA_DIR="$data" "$WJ" "$@"
}

@test "a single author sets up and syncs without tripping the guard" {
    as_user a alice start "task" --project demo --date "$WJ_DAY" --at 9:00
    run as_user a alice sync init "$SYNC_TMP/remote.git"
    [ "$status" -eq 0 ]
    run as_user a alice sync
    [ "$status" -eq 0 ]
    [[ "$output" == *"synced"* ]]
}

@test "a second clone with the SAME actor is refused on init" {
    as_user a alice start "atask" --project demo --date "$WJ_DAY" --at 9:00
    as_user a alice sync init "$SYNC_TMP/remote.git"
    as_user a alice sync

    as_user b alice start "btask" --project demo --date "$WJ_DAY" --at 10:00
    run as_user b alice sync init "$SYNC_TMP/remote.git"
    [ "$status" -ne 0 ]
    [[ "$output" == *"already registered to another machine"* ]]
    # B's data must NOT have reached the remote
    run git -C "$SYNC_TMP/remote.git" grep -q "btask" HEAD
    [ "$status" -ne 0 ]
}

@test "wj sync claim lets the same person adopt the actor on a new machine" {
    as_user a alice start "atask" --project demo --date "$WJ_DAY" --at 9:00
    as_user a alice sync init "$SYNC_TMP/remote.git"
    as_user a alice sync

    as_user b alice start "btask" --project demo --date "$WJ_DAY" --at 10:00
    as_user b alice sync init "$SYNC_TMP/remote.git" || true   # refused, as expected
    run as_user b alice sync claim
    [ "$status" -eq 0 ]
    [[ "$output" == *"synced"* ]]
    # now B's data is shared and the actor has two registered devices
    run git -C "$SYNC_TMP/remote.git" grep -q "btask" HEAD
    [ "$status" -eq 0 ]
    run git -C "$SYNC_TMP/remote.git" show HEAD:.wj-registry
    [ "$(printf '%s\n' "$output" | grep -c '^alice')" -eq 2 ]
}

@test "a different person with a UNIQUE actor syncs fine alongside" {
    as_user a alice start "atask" --project demo --date "$WJ_DAY" --at 9:00
    as_user a alice sync init "$SYNC_TMP/remote.git"
    as_user a alice sync

    as_user c bob start "ctask" --project demo --date "$WJ_DAY" --at 11:00
    run as_user c bob sync init "$SYNC_TMP/remote.git"
    [ "$status" -eq 0 ]
    run as_user c bob sync
    [ "$status" -eq 0 ]
    [[ "$output" == *"synced"* ]]
    run git -C "$SYNC_TMP/remote.git" show HEAD:.wj-registry
    [[ "$output" == *alice* && "$output" == *bob* ]]
}

@test "the identity registry is not mistaken for event data" {
    as_user a alice start "task" --project realproj --date "$WJ_DAY" --at 9:00
    as_user a alice sync init "$SYNC_TMP/remote.git"
    as_user a alice sync
    # list_projects must not surface the registry's host column as a project
    run env WJ_CONFIG="$SYNC_TMP/cfg.a" WJ_DATA_DIR="$SYNC_TMP/data.a" \
        bash -c 'source "$1"; list_projects' _ "$WJ"
    [ "$status" -eq 0 ]
    [[ "$output" == *realproj* ]]
    [[ "$output" != *"$(uname -n)"* ]]
    # and the registry is not a *.tsv file
    run bash -c "find '$SYNC_TMP/data.a' -name '*.tsv' -type f | grep -c registry || true"
    [[ "$output" == "0" ]]
}

@test "repeat syncs don't add duplicate registry lines" {
    as_user a alice start "atask" --project demo --date "$WJ_DAY" --at 9:00
    as_user a alice sync init "$SYNC_TMP/remote.git"
    as_user a alice sync
    as_user a alice sync
    as_user a alice sync
    run git -C "$SYNC_TMP/remote.git" show HEAD:.wj-registry
    [ "$(printf '%s\n' "$output" | grep -c '^alice')" -eq 1 ]
}
