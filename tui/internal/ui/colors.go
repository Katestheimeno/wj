package ui

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"
)

// palette is a curated set of 256-color codes chosen to be distinct and
// readable on both dark and light terminals (no near-black / near-white).
var palette = []lipgloss.Color{
	lipgloss.Color("39"),  // blue
	lipgloss.Color("208"), // orange
	lipgloss.Color("170"), // magenta
	lipgloss.Color("78"),  // green
	lipgloss.Color("214"), // amber
	lipgloss.Color("141"), // violet
	lipgloss.Color("203"), // red
	lipgloss.Color("45"),  // cyan
	lipgloss.Color("113"), // lime
	lipgloss.Color("180"), // tan
}

// ProjectColor maps a project name to a stable color. The same project always
// gets the same hue across days and views, so the chart is learnable at a
// glance. Derived from a hash so no configuration is needed.
func ProjectColor(name string) lipgloss.Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return palette[h.Sum32()%uint32(len(palette))]
}
