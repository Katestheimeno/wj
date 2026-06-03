#!/usr/bin/env bats
# Auto-pause / parallel semantics and the write-path lock.

load test_helper

@test "tasks in the same project run in parallel by default" {
    wj start "a" --project demo --date "$WJ_DAY" --at 9:00
    wj start "b" --project demo --date "$WJ_DAY" --at 9:05
    # both should still be in-progress (no auto-pause)
    run wj status --date "$WJ_DAY"
    [[ "$output" == *"in-progress"* ]]
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"in-progress"* ]]
    run wj show T2 --date "$WJ_DAY"
    [[ "$output" == *"in-progress"* ]]
}

@test "--auto-pause pauses the project's other running task on start" {
    wj start "a" --project demo --date "$WJ_DAY" --at 9:00
    wj start "b" --project demo --date "$WJ_DAY" --at 9:05 --auto-pause
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"paused"* ]]
    run wj show T2 --date "$WJ_DAY"
    [[ "$output" == *"in-progress"* ]]
}

@test "auto-pause does not cross project boundaries" {
    wj start "a" --project alpha --date "$WJ_DAY" --at 9:00
    wj start "b" --project beta  --date "$WJ_DAY" --at 9:05 --auto-pause
    # different project -> T1 keeps running
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"in-progress"* ]]
}

@test "concurrent starts mint unique task ids under the lock" {
    if ! command -v flock >/dev/null; then
        skip "flock not available; lock is a no-op on this platform"
    fi
    for i in $(seq 1 12); do
        wj start "t$i" --project race --date "$WJ_DAY" --at 9:00 >/dev/null 2>&1 &
    done
    wait
    total=$(wj status --date "$WJ_DAY" --json | grep -o '"id":"T[0-9]*"' | wc -l)
    uniq=$(wj status --date "$WJ_DAY" --json | grep -o '"id":"T[0-9]*"' | sort -u | wc -l)
    [ "$total" -eq 12 ]
    [ "$uniq" -eq 12 ]
}
