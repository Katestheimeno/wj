#!/usr/bin/env bats
# Regression tests for assorted CLI fixes:
#   - undo pairs the auto-pause an auto-pausing start/resume wrote
#   - --from/--to are validated (an unreal/far-future bound used to loop forever)
#   - the missing-task error names the actual --date day, not "today"

load test_helper

@test "undo of an auto-pausing start also removes the paired pause" {
    printf 'auto_pause = on\n' >"$WJ_CONFIG"
    wj start "first"  --project demo --date "$WJ_DAY" --at 9:00
    wj start "second" --project demo --date "$WJ_DAY" --at 9:05   # auto-pauses T1
    run wj undo --date "$WJ_DAY"
    [ "$status" -eq 0 ]
    [[ "$output" == *"auto-pause of T1"* ]]
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"in-progress"* ]]                            # T1 restored to running
    run wj status "$WJ_DAY"
    [[ "$output" != *"second"* ]]                                 # T2 fully gone
}

@test "undo of a plain start (no auto-pause) removes only the one event" {
    printf 'auto_pause = off\n' >"$WJ_CONFIG"
    wj start "first"  --project demo --date "$WJ_DAY" --at 9:00
    wj start "second" --project demo --date "$WJ_DAY" --at 9:05   # no auto-pause
    run wj undo --date "$WJ_DAY"
    [[ "$output" != *"auto-pause"* ]]
    run wj status "$WJ_DAY"
    [[ "$output" == *"first"* ]]                                  # T1 untouched
    [[ "$output" != *"second"* ]]                                 # only T2 removed
}

@test "--to rejects a non-date instead of looping forever" {
    run wj report --to 9999-99-99
    [ "$status" -ne 0 ]
    [[ "$output" == *"--to must be a real date"* ]]
}

@test "--from rejects a non-date" {
    run wj report --from not-a-date --to "$WJ_DAY"
    [ "$status" -ne 0 ]
    [[ "$output" == *"--from must be a real date"* ]]
}

@test "a valid --from/--to range still works" {
    wj start "task" --project demo --date "$WJ_DAY" --at 9:00
    run wj report --from "$WJ_DAY" --to "$WJ_DAY"
    [ "$status" -eq 0 ]
}

@test "missing-task error names the actual --date day, not 'today'" {
    run wj complete T9 --date "$WJ_DAY"
    [ "$status" -ne 0 ]
    [[ "$output" == *"no such task on $WJ_DAY"* ]]
}
