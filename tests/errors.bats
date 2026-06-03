#!/usr/bin/env bats
# Error handling and idempotent state changes.

load test_helper

@test "unknown task id errors loudly (does not silently no-op)" {
    wj start "real" --project demo --date "$WJ_DAY" --at 9:00
    for sub in pause resume complete defer cancel; do
        run wj "$sub" T9 --date "$WJ_DAY" --at 9:30
        [ "$status" -ne 0 ]
        [[ "$output" == *"no such task"* ]]
    done
}

@test "amend/move on unknown id error" {
    wj start "real" --project demo --date "$WJ_DAY" --at 9:00
    run wj amend T9 x --date "$WJ_DAY" --at 9:30
    [ "$status" -ne 0 ]; [[ "$output" == *"no such task"* ]]
    run wj move T9 other --date "$WJ_DAY" --at 9:30
    [ "$status" -ne 0 ]; [[ "$output" == *"no such task"* ]]
}

@test "show on unknown id errors" {
    run wj show T9 --date "$WJ_DAY"
    [ "$status" -ne 0 ]
    [[ "$output" == *"no such task"* ]]
}

@test "pausing an already-paused task is an idempotent no-op" {
    wj start "task" --project demo --date "$WJ_DAY" --at 9:00
    wj pause T1 --date "$WJ_DAY" --at 9:30
    run wj pause T1 --date "$WJ_DAY" --at 9:40
    [ "$status" -eq 0 ]
    [[ "$output" == *"already paused"* ]]
}

@test "completing an already-completed task is an idempotent no-op" {
    wj start "task" --project demo --date "$WJ_DAY" --at 9:00
    wj complete T1 --date "$WJ_DAY" --at 9:30
    run wj complete T1 --date "$WJ_DAY" --at 9:40
    [ "$status" -eq 0 ]
    [[ "$output" == *"already completed"* ]]
}

@test "invalid --at is rejected" {
    run wj start "task" --project demo --date "$WJ_DAY" --at "half nine"
    [ "$status" -ne 0 ]
    [[ "$output" == *"--at"* ]]
}

@test "invalid --date is rejected" {
    run wj start "task" --project demo --date 2026-13-40 --at 9:00
    [ "$status" -ne 0 ]
    [[ "$output" == *"--date"* ]]
}

@test "start with no description errors" {
    run wj start --project demo --date "$WJ_DAY" --at 9:00
    [ "$status" -ne 0 ]
    [[ "$output" == *"description"* ]]
}

@test "unknown command errors" {
    run wj frobnicate
    [ "$status" -ne 0 ]
    [[ "$output" == *"unknown command"* ]]
}
