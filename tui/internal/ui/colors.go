package ui

import (
	"hash/fnv"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// palette is a curated set of 256-color codes chosen to be distinct and
// readable on both dark and light terminals (no near-black / near-white).
// Order matters only as a tie-break; ProjectColor spreads projects across it.
var palette = []lipgloss.Color{
	lipgloss.Color("39"),  // blue
	lipgloss.Color("208"), // orange
	lipgloss.Color("170"), // orchid
	lipgloss.Color("78"),  // green
	lipgloss.Color("214"), // amber
	lipgloss.Color("141"), // violet
	lipgloss.Color("203"), // red
	lipgloss.Color("45"),  // cyan
	lipgloss.Color("113"), // lime
	lipgloss.Color("180"), // tan
	lipgloss.Color("75"),  // cornflower
	lipgloss.Color("198"), // rose
	lipgloss.Color("36"),  // teal
	lipgloss.Color("105"), // slate-purple
}

// colorReg memoises the project→color assignment so a project keeps one hue for
// the whole session (stable across days and views). `used` tracks which palette
// slots are taken so distinct projects never share a color until the palette is
// exhausted — a plain hash%N collides well before that (7 projects, 10 colors
// still clashed). Guarded by a mutex: View runs on one goroutine today, but the
// lock keeps this safe if that ever changes.
var (
	colorMu  sync.Mutex
	colorReg = map[string]lipgloss.Color{}
	colorUse = make([]bool, len(palette))
)

// resetColorReg clears the session assignment. Test-only, so a test starts from
// a known-empty registry regardless of what earlier tests coloured.
func resetColorReg() {
	colorMu.Lock()
	defer colorMu.Unlock()
	colorReg = map[string]lipgloss.Color{}
	colorUse = make([]bool, len(palette))
}

// ProjectColor maps a project name to a stable, distinct color. The name hashes
// to a preferred slot; if that slot is already taken by another project we
// linear-probe to the next free one, so no two projects collide until there are
// more projects than colors (then it wraps and reuse is unavoidable). Derived
// without configuration — the same name yields the same color all session.
func ProjectColor(name string) lipgloss.Color {
	colorMu.Lock()
	defer colorMu.Unlock()
	if c, ok := colorReg[name]; ok {
		return c
	}
	n := len(palette)
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	start := int(h.Sum32() % uint32(n))
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		if !colorUse[idx] {
			colorUse[idx] = true
			colorReg[name] = palette[idx]
			return palette[idx]
		}
	}
	// every slot taken (more projects than colors): fall back to the hashed
	// slot. Memoised so this name stays consistent even while it shares a hue.
	colorReg[name] = palette[start]
	return palette[start]
}
