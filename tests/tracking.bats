#!/usr/bin/env bats
# Integration tests for the core tracking lifecycle and time derivation.
# Everything is pinned to WJ_DAY with --at so totals are deterministic.

load test_helper

@test "start mints T1 then T2 per day" {
    run wj start "first" --project demo --date "$WJ_DAY" --at 9:00
    [ "$status" -eq 0 ]
    [[ "$output" == *"T1"* ]]
    run wj start "second" --project demo --date "$WJ_DAY" --at 9:05
    [[ "$output" == *"T2"* ]]
}

@test "completed task sums its active interval" {
    wj start "task" --project demo --date "$WJ_DAY" --at 9:00
    wj complete T1 --project demo --date "$WJ_DAY" --at 10:30
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"1h30m"* ]]
    [[ "$output" == *"completed"* ]]
}

@test "pause/resume excludes the paused gap from the total" {
    wj start "task" --project demo --date "$WJ_DAY" --at 9:00
    wj pause  T1 --date "$WJ_DAY" --at 9:30      # 30m worked
    wj resume T1 --date "$WJ_DAY" --at 10:00     # 30m idle (not counted)
    wj complete T1 --date "$WJ_DAY" --at 10:30   # +30m -> 1h00m total
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"1h00m"* ]]
}

@test "status day total adds across tasks" {
    wj start "a" --project demo --date "$WJ_DAY" --at 9:00
    wj complete T1 --date "$WJ_DAY" --at 10:00   # 1h
    wj start "b" --project demo --date "$WJ_DAY" --at 10:00
    wj complete T2 --date "$WJ_DAY" --at 10:30   # 30m
    run wj status "$WJ_DAY"
    [[ "$output" == *"Total tracked: 1h30m"* ]]
}

@test "cancelled task is voided (0 time) and hidden from status" {
    wj start "oops" --project demo --date "$WJ_DAY" --at 9:00
    wj cancel T1 --date "$WJ_DAY" --at 9:10
    run wj status "$WJ_DAY"
    [[ "$output" != *"oops"* ]]
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"cancelled"* ]]
    [[ "$output" == *"0m"* ]]
}

@test "amend updates the description but keeps history" {
    wj start "old name" --project demo --date "$WJ_DAY" --at 9:00
    wj amend T1 "new name" --date "$WJ_DAY" --at 9:05
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"new name"* ]]
    [[ "$output" == *"renamed"* ]]
}

@test "move re-homes a task to another project" {
    wj start "task" --project alpha --date "$WJ_DAY" --at 9:00
    wj move T1 beta --date "$WJ_DAY" --at 9:05
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"beta"* ]]
}

@test "open task on a past day is counted up to the day's last activity" {
    # T1 left open at 09:00; the last recorded event is T2 completing at 11:30,
    # so T1 is counted 09:00->11:30 = 2h30m — never extended to a guessed shift
    # end (which no longer exists once shift bounds can be unset).
    wj start "ongoing" --project demo --date "$WJ_DAY" --at 9:00
    wj start "other"   --project demo --date "$WJ_DAY" --at 11:00
    wj complete T2 --date "$WJ_DAY" --at 11:30
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"2h30m"* ]]
    [[ "$output" == *"in-progress"* ]]
}

@test "totals=slot rounds a partial slot up" {
    printf 'totals=slot\nslot_minutes=15\n' >"$WJ_CONFIG"
    wj start "task" --project demo --date "$WJ_DAY" --at 9:00
    wj complete T1 --date "$WJ_DAY" --at 9:20   # 20m -> rounds up to 30m (2x15)
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"30m"* ]]
}

@test "JSON status is well-formed and reports minutes" {
    wj start "task" --project demo --date "$WJ_DAY" --at 9:00
    wj complete T1 --date "$WJ_DAY" --at 10:00
    run wj status --date "$WJ_DAY" --json
    [ "$status" -eq 0 ]
    [[ "$output" == *'"minutes":60'* ]]
    [[ "$output" == *'"total_minutes":60'* ]]
    # validate it parses as JSON if a parser is available
    if command -v python3 >/dev/null; then
        echo "$output" | python3 -c 'import sys,json; json.load(sys.stdin)'
    fi
}
