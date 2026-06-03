#!/usr/bin/env bats
# Pending backlog: add / list / due / reorder / drop / promote.

load test_helper

@test "add then list a pending task" {
    wj add "Fix invoice" --project billing
    run wj pending
    [ "$status" -eq 0 ]
    [[ "$output" == *"P1"* ]]
    [[ "$output" == *"Fix invoice"* ]]
    [[ "$output" == *"billing"* ]]
}

@test "pending ids increment" {
    wj add "one"
    wj add "two"
    run wj pending
    [[ "$output" == *"P1"* ]]
    [[ "$output" == *"P2"* ]]
}

@test "due sets and clears a deadline" {
    wj add "task"
    wj due P1 2026-07-01
    run wj pending
    [[ "$output" == *"2026-07-01"* ]]
    wj due P1 -
    run wj pending
    [[ "$output" != *"2026-07-01"* ]]
}

@test "due rejects an invalid date" {
    wj add "task"
    run wj due P1 2026-13-40
    [ "$status" -ne 0 ]
}

@test "raise reorders a pending task upward" {
    wj add "first"
    wj add "second"
    wj raise P2
    run wj pending
    # P2 should now appear before P1
    line2=$(printf '%s\n' "$output" | grep -n 'P2' | cut -d: -f1)
    line1=$(printf '%s\n' "$output" | grep -n 'P1' | cut -d: -f1)
    [ "$line2" -lt "$line1" ]
}

@test "drop removes a pending task" {
    wj add "task"
    wj drop P1
    run wj pending
    [[ "$output" == *"(empty)"* ]]
}

@test "drop on unknown id errors" {
    run wj drop P9
    [ "$status" -ne 0 ]
    [[ "$output" == *"no such pending"* ]]
}

@test "start P# promotes a pending task and drops it from the backlog" {
    wj add "Backlog item" --project ops
    run wj start P1 --date "$WJ_DAY" --at 9:00
    [ "$status" -eq 0 ]
    [[ "$output" == *"Backlog item"* ]]
    [[ "$output" == *"ops"* ]]
    # no longer pending
    run wj pending
    [[ "$output" == *"(empty)"* ]]
    # now a tracked task
    run wj status --date "$WJ_DAY"
    [[ "$output" == *"Backlog item"* ]]
}

@test "pending JSON is well-formed" {
    wj add "task" --project ops --due 2026-07-01
    run wj pending --json
    [[ "$output" == *'"id":"P1"'* ]]
    [[ "$output" == *'"due":"2026-07-01"'* ]]
    if command -v python3 >/dev/null; then
        echo "$output" | python3 -c 'import sys,json; json.load(sys.stdin)'
    fi
}
