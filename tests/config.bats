#!/usr/bin/env bats
# Config seeding, INI parsing/migration, and the wj-tui flag plumbing.

load test_helper

@test "a fresh config is seeded as INI with [sections]" {
    rm -f "$WJ_CONFIG"          # force a re-seed (setup leaves an empty file)
    wj config >/dev/null
    run grep -E '^\[(tracking|ui|colors)\]' "$WJ_CONFIG"
    [ "$status" -eq 0 ]
}

@test "a fresh config is seeded with the accent key" {
    rm -f "$WJ_CONFIG"
    wj config >/dev/null
    run grep -E '^accent[[:space:]]*=[[:space:]]*141' "$WJ_CONFIG"
    [ "$status" -eq 0 ]
}

@test "a fresh config is seeded with the per-panel color keys" {
    rm -f "$WJ_CONFIG"
    wj config >/dev/null
    for key in color_projects color_tasks color_pending color_range color_day color_timeline; do
        run grep -E "^$key[[:space:]]*=" "$WJ_CONFIG"
        [ "$status" -eq 0 ]
    done
}

@test "the parser keeps a leading-# value (hex color) and strips inline comments" {
    printf '[ui]\naccent = #9d7cd8   # my purple\nlayout = spotlight\n' >"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" == *"-accent #9d7cd8"* ]]
    [[ "$output" == *"-layout spotlight"* ]]
    [[ "$output" != *"purple"* ]]
}

@test "an unknown key in the config is ignored (no shell injection)" {
    # the old `source` would have run this; the parser must not.
    printf '[ui]\naccent = 99\nPWNED=$(touch %s/pwned)\n' "$WJ_DATA_DIR" >"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" == *"-accent 99"* ]]
    [ ! -e "$WJ_DATA_DIR/pwned" ]
}

@test "malformed lines (blank key, lone '=') are ignored without error" {
    printf '[ui]\n= orphan\n   =   spaced\naccent = 42\n' >"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" == *"-accent 42"* ]]   # the valid key still parses
    [[ "$output" != *"orphan"* ]]
    [[ "$output" != *"spaced"* ]]
}

@test "a quoted value has its surrounding quotes stripped" {
    printf '[ui]\naccent = "42"\n' >"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" == *"-accent 42"* ]]
    [[ "$output" != *'"42"'* ]]
}

@test "a legacy flat config is migrated to a sectioned cfg, preserving values" {
    # point at a .../cfg target with a sibling legacy `config` in flat format
    local dir; dir="$(mktemp -d "${BATS_TMPDIR:-/tmp}/wj-mig.XXXXXX")"
    WJ_CONFIG="$dir/cfg"
    printf 'accent=99\nshift_start=07:30\nlayout=golden\n' >"$dir/config"
    wj config >/dev/null 2>&1
    [ -f "$dir/cfg" ]
    [ -f "$dir/config.bak" ]
    [ ! -f "$dir/config" ]
    grep -Eq '^\[ui\]' "$dir/cfg"
    grep -Eq '^accent[[:space:]]*=[[:space:]]*99' "$dir/cfg"
    grep -Eq '^shift_start[[:space:]]*=[[:space:]]*07:30' "$dir/cfg"
    grep -Eq '^layout[[:space:]]*=[[:space:]]*golden' "$dir/cfg"
    rm -rf "$dir"
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

@test "wj ui forwards the sidebar side and custom layout ratios" {
    printf 'sidebar=right\nlayout=custom\nlayout_sidebar=28\nlayout_split=60,25,15\n' >"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" == *"-sidebar right"* ]]
    [[ "$output" == *"-layout-sidebar 28"* ]]
    [[ "$output" == *"-layout-split 60,25,15"* ]]
}

@test "empty custom-layout ratios omit their flags" {
    printf 'sidebar=left\nlayout_sidebar=\nlayout_split=\n' >"$WJ_CONFIG"
    PATH="$(stub_wj_tui_dir):$PATH"
    run wj ui
    [ "$status" -eq 0 ]
    [[ "$output" != *"-layout-sidebar"* ]]
    [[ "$output" != *"-layout-split"* ]]
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
