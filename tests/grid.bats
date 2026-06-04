#!/usr/bin/env bats
# The grid/gantt window. The configured shift bounds are only a *default* frame:
# the window auto-expands to fit a day's real activity, and an unset bound
# (commented out in the config) drops the frame so the grid auto-fits the day.
# Nothing tracked outside shift hours is ever clipped or lost. See `grid_window`
# and `report_nm` in ./wj.

load test_helper

# the "shift_start"/"shift_end" pair from a day's grid --json (the *effective*
# window, not the raw config) — this is what frames the wj-tui Day axis.
grid_window_of() {
    wj grid "$1" --json | grep -o '"shift_start":"[^"]*","shift_end":"[^"]*"'
}

@test "a normal day inside the shift keeps the configured 09:00-19:00 frame" {
    printf 'shift_start=09:00\nshift_end=19:00\n' >"$WJ_CONFIG"
    wj start "task" --project demo --date "$WJ_DAY" --at 9:30
    wj complete T1 --date "$WJ_DAY" --at 17:00
    run grid_window_of "$WJ_DAY"
    [[ "$output" == *'"shift_start":"09:00","shift_end":"19:00"'* ]]
}

@test "work outside the shift auto-expands the window (nothing clipped)" {
    printf 'shift_start=09:00\nshift_end=19:00\n' >"$WJ_CONFIG"
    wj start "early" --project demo --date "$WJ_DAY" --at 8:10
    wj complete T1 --date "$WJ_DAY" --at 20:30
    run grid_window_of "$WJ_DAY"
    # 08:10..20:30, floored/ceiled to whole hours around the activity
    [[ "$output" == *'"shift_start":"08:00","shift_end":"21:00"'* ]]
}

@test "minutes outside the shift are counted (grid agrees with status)" {
    printf 'shift_start=09:00\nshift_end=19:00\n' >"$WJ_CONFIG"
    wj start "late" --project demo --date "$WJ_DAY" --at 18:00
    wj complete T1 --date "$WJ_DAY" --at 20:30   # 2h30m, mostly past shift_end
    run wj grid "$WJ_DAY" --json
    [[ "$output" == *'"minutes":150'* ]]
    run wj show T1 --date "$WJ_DAY"
    [[ "$output" == *"2h30m"* ]]
}

@test "unset shift bounds (commented out) auto-fit the grid to the day" {
    : >"$WJ_CONFIG"   # no shift_start / shift_end at all
    wj start "task" --project demo --date "$WJ_DAY" --at 7:40
    wj complete T1 --date "$WJ_DAY" --at 21:10
    run grid_window_of "$WJ_DAY"
    [[ "$output" == *'"shift_start":"07:00","shift_end":"22:00"'* ]]
}
