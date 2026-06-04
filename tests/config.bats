#!/usr/bin/env bats
# Config seeding and the wj-tui accent-color plumbing.

load test_helper

@test "a fresh config is seeded with the accent key" {
    rm -f "$WJ_CONFIG"          # force a re-seed (setup leaves an empty file)
    wj config >/dev/null
    run grep '^accent=' "$WJ_CONFIG"
    [ "$status" -eq 0 ]
    [[ "$output" == "accent=141" ]]
}

@test "a fresh config is seeded with the per-panel color keys" {
    rm -f "$WJ_CONFIG"
    wj config >/dev/null
    for key in color_projects color_tasks color_pending color_range color_day color_timeline; do
        run grep "^$key=" "$WJ_CONFIG"
        [ "$status" -eq 0 ]
    done
}

# Put a wj-tui stub that echoes its args at the front of PATH, so we can assert
# what `wj ui` (which execs wj-tui) forwards — without the real binary. Echoes
# the stub directory for the caller to prepend.
stub_wj_tui_dir() {
    local dir="$WJ_DATA_DIR/bin"
    mkdir -p "$dir"
    printf '#!/bin/sh\necho "ARGS: $*"\n' >"$dir/wj-tui"
    chmod +x "$dir/wj-tui"
    printf '%s' "$dir"
}

@test "wj ui forwards the configured accent to wj-tui" {
    printf 'accent=99\n' >"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" == *"-accent 99"* ]]
}

@test "an empty accent omits the flag (wj-tui keeps its default)" {
    printf 'accent=\n' >"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" != *"-accent"* ]]
}

@test "wj ui forwards the configured layout to wj-tui" {
    printf 'layout=spotlight\n' >"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" == *"-layout spotlight"* ]]
}

@test "an empty layout omits the flag (wj-tui keeps its default)" {
    printf 'layout=\n' >"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" != *"-layout"* ]]
}

@test "wj ui forwards the per-panel colors as a -colors spec" {
    printf 'color_projects=99\ncolor_timeline=#abcdef\n' >"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" == *"-colors "* ]]
    [[ "$output" == *"projects=99"* ]]
    [[ "$output" == *"timeline=#abcdef"* ]]
}

@test "a cleared panel color is dropped from the -colors spec" {
    # start from the seeded defaults, then clear one panel color
    rm -f "$WJ_CONFIG"; wj config >/dev/null
    printf 'color_day=\n' >>"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" == *"-colors "* ]]
    [[ "$output" != *"day="* ]]
}
