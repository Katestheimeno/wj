package ui

import (
	"fmt"
	"strings"
)

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// labelWidth is the project/task label column, capped so the chart still fits
// in a narrow main column (the sidebar holds the wide width).
func labelWidth(innerW int) int {
	lw := labelW
	if third := innerW / 3; lw > third {
		lw = third
	}
	if lw < 6 {
		lw = 6
	}
	return lw
}

func lineCount(s string) int {
	return strings.Count(s, "\n") + 1
}

// hm parses "HH:MM" into minutes since midnight (0 on malformed input).
func hm(s string) int {
	if len(s) < 5 || s[2] != ':' {
		return 0
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	mn := int(s[3]-'0')*10 + int(s[4]-'0')
	return h*60 + mn
}

// truncate clips a plain string to w display columns (rune-based), adding an
// ellipsis. Used to keep single-line footers/titles from overflowing.
func truncate(s string, w int) string {
	r := []rune(s)
	if w <= 0 || len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

func shortDate(d string) string {
	if len(d) >= 10 {
		return d[5:]
	}
	return d
}

func fmtDur(mins int) string {
	h, m := mins/60, mins%60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func padRight(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return string(r[:w])
	}
	return s + strings.Repeat(" ", w-len(r))
}

func center(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return string(r[:w])
	}
	left := (w - len(r)) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", w-len(r)-left)
}
