package ui

import (
	"fmt"
	"strings"
	"unicode"
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

// runeInsert inserts ins at rune index i in s, returning the new string and the
// cursor position just after the inserted text. i is clamped into range.
func runeInsert(s string, i int, ins string) (string, int) {
	r := []rune(s)
	i = clamp(i, 0, len(r))
	ir := []rune(ins)
	out := make([]rune, 0, len(r)+len(ir))
	out = append(out, r[:i]...)
	out = append(out, ir...)
	out = append(out, r[i:]...)
	return string(out), i + len(ir)
}

// runeDeleteBefore removes the rune immediately before index i (Backspace) and
// returns the new string with the cursor at i-1. No-op when i<=0.
func runeDeleteBefore(s string, i int) (string, int) {
	r := []rune(s)
	i = clamp(i, 0, len(r))
	if i == 0 {
		return s, 0
	}
	out := append(append([]rune{}, r[:i-1]...), r[i:]...)
	return string(out), i - 1
}

// runeDeleteWordBefore removes the word immediately before index i (Ctrl+W /
// Ctrl+Backspace): it first eats any whitespace just before the cursor, then the
// run of non-whitespace, leaving the cursor where the word began. No-op at i<=0.
func runeDeleteWordBefore(s string, i int) (string, int) {
	r := []rune(s)
	i = clamp(i, 0, len(r))
	j := i
	for j > 0 && unicode.IsSpace(r[j-1]) { // skip trailing whitespace
		j--
	}
	for j > 0 && !unicode.IsSpace(r[j-1]) { // then the word itself
		j--
	}
	out := append(append([]rune{}, r[:j]...), r[i:]...)
	return string(out), j
}

// runeDeleteAt removes the rune at index i (forward Delete), leaving the cursor
// in place. No-op when i is at or past the end.
func runeDeleteAt(s string, i int) (string, int) {
	r := []rune(s)
	i = clamp(i, 0, len(r))
	if i >= len(r) {
		return s, len(r)
	}
	out := append(append([]rune{}, r[:i]...), r[i+1:]...)
	return string(out), i
}

// withCursor renders value with an insertion-bar cursor (▏) at rune index i,
// so the prompt shows where typed text will land.
func withCursor(value string, i int) string {
	r := []rune(value)
	i = clamp(i, 0, len(r))
	return string(r[:i]) + pickGlyph("|", "▏") + string(r[i:])
}

// wrapWords splits s into lines no wider than w display columns (rune-based),
// breaking on spaces. A single word longer than w is hard-cut. Existing newlines
// in s start fresh lines. Returns at least one line (possibly empty).
func wrapWords(s string, w int) []string {
	if w < 1 {
		w = 1
	}
	var lines []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		cur := ""
		for _, word := range words {
			for len([]rune(word)) > w { // hard-split an oversized word
				if cur != "" {
					lines = append(lines, cur)
					cur = ""
				}
				r := []rune(word)
				lines = append(lines, string(r[:w]))
				word = string(r[w:])
			}
			switch {
			case cur == "":
				cur = word
			case len([]rune(cur))+1+len([]rune(word)) <= w:
				cur += " " + word
			default:
				lines = append(lines, cur)
				cur = word
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "")
	}
	return lines
}

// hangingRow renders a plain-text prefix followed by body, word-wrapped to width
// w, with every continuation line indented to align under the body (a hanging
// indent). prefix must be unstyled — its rune length sets the indent.
func hangingRow(prefix, body string, w int) string {
	indent := len([]rune(prefix))
	textW := w - indent
	if textW < 1 {
		textW = 1
	}
	if strings.TrimSpace(body) == "" {
		return prefix + body
	}
	lines := wrapWords(body, textW)
	pad := strings.Repeat(" ", indent)
	out := prefix + lines[0]
	for _, l := range lines[1:] {
		out += "\n" + pad + l
	}
	return out
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
