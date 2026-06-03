#!/usr/bin/env bats
# Unit tests for wj's pure helper functions (no file I/O).

load test_helper

@test "hm_to_min: midnight is 0" {
    run wj_fn hm_to_min 00:00
    [ "$status" -eq 0 ]; [ "$output" = "0" ]
}

@test "hm_to_min: 09:30 -> 570" {
    run wj_fn hm_to_min 09:30
    [ "$output" = "570" ]
}

@test "min_to_hm: 570 -> 09:30" {
    run wj_fn min_to_hm 570
    [ "$output" = "09:30" ]
}

@test "fmt_dur: under an hour" {
    run wj_fn fmt_dur 20
    [ "$output" = "20m" ]
}

@test "fmt_dur: hours and minutes zero-pad" {
    run wj_fn fmt_dur 65
    [ "$output" = "1h05m" ]
}

@test "fmt_dur: exact hours" {
    run wj_fn fmt_dur 120
    [ "$output" = "2h00m" ]
}

@test "normalize_time accepts bare hour" {
    run wj_fn normalize_time 9
    [ "$output" = "09:00" ]
}

@test "normalize_time accepts 930" {
    run wj_fn normalize_time 930
    [ "$output" = "09:30" ]
}

@test "normalize_time accepts 9:30" {
    run wj_fn normalize_time 9:30
    [ "$output" = "09:30" ]
}

@test "normalize_time accepts 9.30" {
    run wj_fn normalize_time 9.30
    [ "$output" = "09:30" ]
}

@test "normalize_time accepts 9pm" {
    run wj_fn normalize_time 9pm
    [ "$output" = "21:00" ]
}

@test "normalize_time accepts 12am as midnight" {
    run wj_fn normalize_time 12am
    [ "$output" = "00:00" ]
}

@test "normalize_time accepts 12pm as noon" {
    run wj_fn normalize_time 12pm
    [ "$output" = "12:00" ]
}

@test "normalize_time rejects garbage" {
    run wj_fn normalize_time nope
    [ "$status" -ne 0 ]
}

@test "normalize_time rejects out-of-range minutes" {
    run wj_fn normalize_time 9:75
    [ "$status" -ne 0 ]
}

@test "normalize_time rejects 25:00" {
    run wj_fn normalize_time 25:00
    [ "$status" -ne 0 ]
}

@test "normalize_project lowercases" {
    run wj_fn normalize_project Backend
    [ "$output" = "backend" ]
}

@test "normalize_project collapses internal spaces" {
    run wj_fn normalize_project "foo   bar"
    [ "$output" = "foo bar" ]
}

@test "normalize_project trims ends" {
    run wj_fn normalize_project "  spaced  "
    [ "$output" = "spaced" ]
}

@test "snap_min snaps down to a 5-min slot" {
    # snap_min minute start slot ; start=540 (09:00), slot=5
    run wj_fn snap_min 547 540 5
    [ "$output" = "545" ]
}

@test "valid_date accepts a real date" {
    run wj_fn valid_date 2026-02-28
    [ "$status" -eq 0 ]
}

@test "valid_date rejects a rolled-over date" {
    run wj_fn valid_date 2026-02-31
    [ "$status" -ne 0 ]
}

@test "valid_date rejects a non-date string" {
    run wj_fn valid_date "not-a-date"
    [ "$status" -ne 0 ]
}
