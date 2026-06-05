package ui

import (
	"fmt"
	"github.com/Katestheimeno/wj/tui/internal/wj"
	"github.com/charmbracelet/lipgloss"
	"math"
	"strconv"
	"strings"
	"time"
)

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	w := m.width
	if w < 20 {
		w = 80
	}

	header := m.renderHeader(w)

	if m.showHelp {
		return header + "\n" +
			panel("Help", accent, m.helpOverlay(), true, w, 0) + "\n" +
			footerStyle.Render("press ? or esc to close")
	}

	if m.search.active {
		return header + "\n" +
			panel("Search", accent, m.searchOverlay(w), true, w, 0) + "\n" +
			footerStyle.Render("type to filter · ↑↓ move · enter jump · esc cancel")
	}

	foot := m.renderFooter(w)
	legend := m.bottomLegend(w)
	legendLines := 0
	if legend != "" {
		legendLines = 1
	}

	// fill the screen only once we know the terminal height (tests size 0).
	fill := m.height > 0
	bodyH := 0
	if fill {
		bodyH = m.height - 1 - legendLines - lineCount(foot) // minus the header line
		if bodyH < 6 {
			bodyH = 6
		}
	}

	var body string
	if m.zoomed {
		// maximize the focused pane's content to the full body area
		body = m.renderZoom(w, bodyH, fill)
	} else {
		sideW := clamp(w*m.activeLayout().sidePct/100, 18, 40)
		if sideW > w-24 {
			sideW = w - 24
		}
		if sideW < 12 {
			sideW = 12
		}
		mainW := w - sideW
		sb := m.renderSidebar(sideW, bodyH, fill)
		mn := m.renderMain(mainW, bodyH, fill)
		if sidebarRight {
			body = lipgloss.JoinHorizontal(lipgloss.Top, mn, sb)
		} else {
			body = lipgloss.JoinHorizontal(lipgloss.Top, sb, mn)
		}
	}

	parts := []string{header, body}
	if legend != "" {
		parts = append(parts, legend)
	}
	parts = append(parts, foot)
	out := strings.Join(parts, "\n")
	if fill { // hard guard: never render more lines than the terminal is tall
		out = clipLines(out, m.height)
	}
	return out
}

// bottomLegend is a single full-width key of every project in the current
// range, shown once at the foot of the view (the per-panel legends are gone).
// Swatches are dropped from the tail if they would overflow the width.
func (m Model) bottomLegend(w int) string {
	if m.g == nil {
		return ""
	}
	seen := map[string]bool{}
	var parts []string
	for _, r := range m.g.Rows {
		p := rowProject(r)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		sw := lipgloss.NewStyle().Foreground(ProjectColor(p)).Render("█")
		parts = append(parts, sw+" "+p)
	}
	if len(parts) == 0 {
		return ""
	}
	line := dimStyle.Render("legend: ")
	used := lipgloss.Width(line)
	add := func(seg string) bool {
		if used+lipgloss.Width(seg) > w {
			return false
		}
		line += seg
		used += lipgloss.Width(seg)
		return true
	}
	for i, part := range parts {
		seg := part
		if i > 0 {
			seg = "  " + part
		}
		if !add(seg) {
			return line // project swatches alone already fill the width
		}
	}
	add(statusKey()) // best-effort: append the status glyph key if it fits
	return line
}

// renderHeader is the full-width top bar: the range/grouping on the left and
// the live running-task clock right-aligned (so the line spans the width).
func (m Model) renderHeader(w int) string {
	left := fmt.Sprintf("wj · %s .. %s · by %s", m.from, m.to, m.by)
	// right side: today's status rollup, then the running-task clock.
	right := m.todayRollup()
	if run := m.runningHeader(); run != "" {
		if right != "" {
			right += "   "
		}
		right += run
	}
	rw := lipgloss.Width(right)
	if rw+1 > w { // right side alone won't fit — drop it rather than overflow
		right, rw = "", 0
	}
	if lipgloss.Width(left)+rw+2 > w {
		left = truncate(left, max2(1, w-rw-2))
	}
	gap := w - lipgloss.Width(left) - rw
	if gap < 1 {
		gap = 1
	}
	return titleStyle.Render(left) + strings.Repeat(" ", gap) + right
}

// renderFooter builds the (possibly multi-line) bottom area: an error line, an
// active prompt, a confirm guard, or the context-sensitive key hint.
func (m Model) renderFooter(w int) string {
	var b strings.Builder
	if m.err != "" {
		b.WriteString(errStyle.Render(truncate(pickGlyph("!", "⚠")+" "+m.err, w)) + "\n")
	} else if m.notice != "" {
		b.WriteString(noticeStyle.Render(truncate(pickGlyph("+", "✓")+" "+m.notice, w)) + "\n")
	}
	switch {
	case m.input.active:
		b.WriteString(inputStyle.Render(truncate(m.input.prompt+": "+withCursor(m.input.value, m.input.cursor), w)) + "\n")
		hint := "[enter] confirm   [esc] cancel"
		if m.input.action == "move" {
			prefix := m.input.value
			if m.input.acPrefix != "" {
				prefix = m.input.acPrefix
			}
			if ms := m.projectMatches(prefix); len(ms) > 0 {
				if len(ms) > 6 {
					ms = ms[:6]
				}
				hint = pickGlyph("Tab", "⇥") + " " + strings.Join(ms, " ") + "   [esc] cancel"
			}
		}
		b.WriteString(footerStyle.Render(truncate(hint, w)))
	case m.confirm.active:
		b.WriteString(inputStyle.Render(m.confirm.prompt+"  ") + footerStyle.Render("[y/n] ("+pickGlyph("Enter", "⏎")+"=yes)"))
	default:
		b.WriteString(footerStyle.Render(truncate(m.footerLine(), w)))
	}
	return b.String()
}

// renderSidebar stacks the Projects and Tasks list panels in the left column.
func (m Model) renderSidebar(w, h int, fill bool) string {
	cw := w - 4 // content width: minus border(2) + padding(2)
	if cw < 6 {
		cw = 6
	}
	taskTitle := "Tasks"
	if d := m.currentDay(); d != "" {
		taskTitle = "Tasks · " + shortDate(d)
	}
	pendTitle := "Pending"
	if n := len(m.pending); n > 0 {
		pendTitle = fmt.Sprintf("Pending (%d)", n)
	}
	if !fill {
		return lipgloss.JoinVertical(lipgloss.Left,
			panel("Projects", colorProjects, m.renderProjects(cw, 1<<30), m.pane == paneRange, w, 0),
			panel(taskTitle, colorTasks, m.renderTasks(cw, 1<<30), m.pane == paneDay, w, 0),
			panel(pendTitle, colorPending, m.renderPending(cw, 1<<30, m.pane == panePending), m.pane == panePending, w, 0),
		)
	}
	// focused sidebar panel gets the most room (−1 = none focused → equal thirds)
	fi := -1
	switch m.pane {
	case paneRange:
		fi = 0
	case paneDay:
		fi = 1
	case panePending:
		fi = 2
	}
	hs := m.activeLayout().sidebarSplit(h, fi)
	// auto-hide an empty backlog: collapse Pending to a slim title strip and give
	// the reclaimed rows to Projects/Tasks (it stays focusable, so nav is intact).
	if len(m.pending) == 0 {
		if slim := 3; hs[2] > slim {
			extra := hs[2] - slim
			hs[2] = slim
			hs[0] += extra / 2
			hs[1] += extra - extra/2
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		panel("Projects", colorProjects, m.renderProjects(cw, hs[0]-3), m.pane == paneRange, w, hs[0]),
		panel(taskTitle, colorTasks, m.renderTasks(cw, hs[1]-3), m.pane == paneDay, w, hs[1]),
		panel(pendTitle, colorPending, m.renderPending(cw, hs[2]-3, m.pane == panePending), m.pane == panePending, w, hs[2]),
	)
}

// renderPending lists the backlog: a deadline-urgency glyph + the description
// (project-colored when set), with the due date right-aligned.
func (m Model) renderPending(cw, maxRows int, active bool) string {
	if len(m.pending) == 0 {
		// only surface the "press a to add" affordance when this panel is
		// focused — a is a Pending-pane key, so the hint would mislead otherwise.
		if active {
			return dimStyle.Render("(empty — press a to add)")
		}
		return dimStyle.Render("(empty)")
	}
	items := make([]string, len(m.pending))
	for i, p := range m.pending {
		glyph, gc, due := m.dueBadge(p.Due)
		left := p.Desc
		if left == "" {
			left = p.ID
		}
		lc := lipgloss.Color("250")
		if p.Project != "" {
			lc = ProjectColor(p.Project)
		}
		items[i] = listLine(glyph, gc, lc, left, due, i == m.selPend, cw)
	}
	items = windowRows(items, m.selPend, maxRows)
	return strings.Join(items, "\n")
}

// renderPendingDetail shows the selected backlog task in full — its description
// word-wrapped, with project, deadline, and created date. It is the master→detail
// counterpart of the Timeline, shown in the main column while the Pending pane has
// focus, so a long description stays fully readable despite the narrow list.
func (m Model) renderPendingDetail(innerW, maxBody int) string {
	if m.selPend < 0 || m.selPend >= len(m.pending) {
		return dimStyle.Render("(no pending task selected)")
	}
	p := m.pending[m.selPend]
	var lines []string
	head := lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true).Render(p.ID)
	if p.Project != "" {
		head += "  " + lipgloss.NewStyle().Foreground(ProjectColor(p.Project)).Render("["+p.Project+"]")
	}
	if p.Due != "" {
		glyph, gc, due := m.dueBadge(p.Due)
		label := "due " + due
		if g := strings.TrimSpace(glyph); g != "" {
			label = g + " " + label
		}
		head += "  " + lipgloss.NewStyle().Foreground(gc).Render(label)
	}
	lines = append(lines, head)
	if when := p.Created; when != "" {
		if len(when) >= 16 { // YYYY-MM-DDTHH:MM -> "YYYY-MM-DD HH:MM"
			when = when[:10] + " " + when[11:16]
		}
		lines = append(lines, dimStyle.Render("created "+when))
	}
	lines = append(lines, "")
	if strings.TrimSpace(p.Desc) == "" {
		lines = append(lines, dimStyle.Render("(no description)"))
	} else {
		lines = append(lines, wrapWords(p.Desc, innerW)...)
	}
	if maxBody > 0 && len(lines) > maxBody { // clamp to the panel height
		lines = lines[:maxBody]
		lines[maxBody-1] = dimStyle.Render("…")
	}
	return strings.Join(lines, "\n")
}

// dueBadge maps a deadline to an urgency glyph, color, and compact label.
// Overdue → red "!", due today/≤2d → amber "!", further out → dim.
func (m Model) dueBadge(due string) (string, lipgloss.Color, string) {
	if due == "" {
		return " ", lipgloss.Color("244"), "—"
	}
	t, err := time.Parse(dateLayout, due)
	if m.today == "" || err != nil {
		return " ", lipgloss.Color("244"), shortDate(due)
	}
	today, err2 := time.Parse(dateLayout, m.today)
	if err2 != nil {
		return " ", lipgloss.Color("244"), shortDate(due)
	}
	days := int(t.Sub(today).Hours() / 24)
	switch {
	case days < 0:
		return "!", lipgloss.Color("203"), fmt.Sprintf("%dd", days) // e.g. -2d
	case days == 0:
		return "!", lipgloss.Color("214"), "today"
	case days <= 2:
		return "!", lipgloss.Color("214"), fmt.Sprintf("%dd", days)
	case days <= 6:
		return " ", lipgloss.Color("244"), fmt.Sprintf("%dd", days)
	default:
		return " ", lipgloss.Color("244"), shortDate(due)
	}
}

// renderMain stacks the Range / Day / Timeline visualizations in the right
// column; the focused level's panel gets the most vertical room.
func (m Model) renderMain(w, h int, fill bool) string {
	innerW := w - 4
	if innerW < 8 {
		innerW = 8
	}
	dayTitle := "Day — " + m.currentDay()
	// The bottom main panel is the Timeline by default; when the Pending pane has
	// focus it becomes the selected backlog item's full detail (Pending lives only
	// in the sidebar, so its detail maps onto this slot — the Timeline's place).
	botTitle, botColor := "Timeline", colorTimeline
	if m.show != nil {
		botTitle = "Timeline · " + m.show.ID
	}
	botBody := func(rows int) string { return m.renderTimeline(innerW, rows) }
	if m.pane == panePending {
		botTitle, botColor = "Pending", colorPending
		if id := m.selectedPendID(); id != "" {
			botTitle = "Pending · " + id
		}
		botBody = func(rows int) string { return m.renderPendingDetail(innerW, rows) }
	}
	if !fill {
		return lipgloss.JoinVertical(lipgloss.Left,
			panel("Range", colorRange, m.rangeBody(innerW, 1<<30), m.pane == paneRange, w, 0),
			panel(dayTitle, colorDay, m.renderDay(innerW, 1<<30), m.pane == paneDay, w, 0),
			panel(botTitle, botColor, botBody(1<<30), m.pane == paneTimeline, w, 0),
		)
	}
	// the main column has 3 panels (Range/Day/Timeline); Pending (pane 3) lives
	// only in the sidebar, so it maps onto the bottom (Timeline) slot here.
	mainFocus := int(m.pane)
	if mainFocus > 2 {
		mainFocus = 2
	}
	hs := m.activeLayout().split(h, mainFocus)
	return lipgloss.JoinVertical(lipgloss.Left,
		panel("Range", colorRange, m.rangeBody(innerW, hs[0]-3), m.pane == paneRange, w, hs[0]),
		panel(dayTitle, colorDay, m.renderDay(innerW, hs[1]-3), m.pane == paneDay, w, hs[1]),
		panel(botTitle, botColor, botBody(hs[2]-3), m.pane == paneTimeline, w, hs[2]),
	)
}

// renderZoom draws a single panel filling the whole body area — the focused
// pane's main content (Range/Day/Timeline), or the Pending backlog when that
// pane has focus. Navigation still works while zoomed, so the view follows
// focus; z (or Esc) returns to the split layout.
func (m Model) renderZoom(w, h int, fill bool) string {
	innerW := w - 4
	if innerW < 8 {
		innerW = 8
	}
	ph, body := 0, 1<<30 // !fill: content-sized, unbounded rows
	if fill {
		ph, body = h, h-3 // panel height h; content = minus border(2)+title(1)
	}
	switch m.pane {
	case paneRange:
		return panel("Range", colorRange, m.rangeBody(innerW, body), true, w, ph)
	case paneDay:
		return panel("Day — "+m.currentDay(), colorDay, m.renderDay(innerW, body), true, w, ph)
	case panePending:
		title := "Pending"
		if n := len(m.pending); n > 0 {
			title = fmt.Sprintf("Pending (%d)", n)
		}
		return panel(title, colorPending, m.renderPending(innerW, body, true), true, w, ph)
	default: // timeline
		title := "Timeline"
		if m.show != nil {
			title = "Timeline · " + m.show.ID
		}
		return panel(title, colorTimeline, m.renderTimeline(innerW, body), true, w, ph)
	}
}

// rangeBody renders the Range gantt, or a placeholder when the range is empty.
func (m Model) rangeBody(innerW, maxBody int) string {
	if m.g != nil && len(m.g.Rows) == 0 {
		return dimStyle.Render("(no tracked time in this range)")
	}
	return m.renderRange(innerW, maxBody)
}

// layoutProfile parameterizes the panel arrangement: the sidebar width and how
// each column's height is divided among its three stacked panels. The focused
// panel gets focusNum/focusDen of the height; the remaining two split the rest
// by the restHi:restLo weights (so a profile can make them uneven).
type layoutProfile struct {
	name               string
	sidePct            int // sidebar width as a percent of the terminal width
	focusNum, focusDen int // focused panel's share of its column height
	restHi, restLo     int // weights for splitting the rest between the other two
}

// layouts are the switchable presets (cycled with Shift+L, default set via the
// config's layout= / -layout). balanced is the original even-ish look.
var layouts = []layoutProfile{
	{"balanced", 24, 1, 2, 1, 1},   // focused ½, others ¼ each
	{"spotlight", 22, 7, 10, 1, 1}, // focused dominates (~70%), others thin
	{"golden", 32, 62, 100, 3, 2},  // wider sidebar, uneven 62 / 23 / 15
}
var defaultLayout = 0 // index into layouts; overridden by SetLayout at startup

func layoutIndex(name string) int {
	for i, lp := range layouts {
		if lp.name == name {
			return i
		}
	}
	return -1
}

// SetLayout selects the startup layout by name (balanced | spotlight | golden |
// custom). An unknown or empty name keeps the default (balanced). Call once at
// startup — after SetLayoutRatios, so a configured "custom" profile exists.
func SetLayout(name string) {
	if i := layoutIndex(strings.TrimSpace(name)); i >= 0 {
		defaultLayout = i
	}
}

// sidebarRight puts the lists column on the right (default: left). Set from the
// config's sidebar= via SetSidebar.
var sidebarRight = false

// SetSidebar places the sidebar on the "left" (default) or "right". Any other
// value (incl. empty) is ignored. Call once at startup.
func SetSidebar(side string) {
	if strings.TrimSpace(side) == "right" {
		sidebarRight = true
	}
}

// useIcons swaps the status markers for Nerd-Font icons. Off by default (the
// ASCII set renders in any font); set from the config's icons= via SetIcons.
// A terminal app can't detect a font's glyph coverage, so this is an explicit
// opt-in rather than auto-detection.
var useIcons = false

// SetIcons enables Nerd-Font status icons when mode is "on" (anything else,
// including "off"/empty, keeps the universal ASCII set). Call once at startup.
func SetIcons(mode string) {
	useIcons = strings.TrimSpace(mode) == "on"
}

// pickGlyph returns the Nerd-Font icon when icons are on, else the ASCII
// fallback. Both are a single cell wide.
func pickGlyph(ascii, icon string) string {
	if useIcons {
		return icon
	}
	return ascii
}

// SetLayoutRatios registers a "custom" layout from config overrides, so named
// presets stay pure: sidebar is the sidebar width percent ("28"), and split is
// the three panel weights "focused,hi,lo" ("60,25,15" → focused 60% of the
// column, the other two splitting the rest 25:15). Either may be empty to keep
// balanced's value. With both empty, no custom layout is added. Call once at
// startup, before SetLayout.
func SetLayoutRatios(sidebar, split string) {
	sidebar, split = strings.TrimSpace(sidebar), strings.TrimSpace(split)
	if sidebar == "" && split == "" {
		return
	}
	lp := layouts[0] // start from balanced
	lp.name = "custom"
	if n, err := strconv.Atoi(sidebar); err == nil && n >= 5 && n <= 60 {
		lp.sidePct = n
	}
	if parts := strings.Split(split, ","); len(parts) == 3 {
		f, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		hi, e2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		lo, e3 := strconv.Atoi(strings.TrimSpace(parts[2]))
		if e1 == nil && e2 == nil && e3 == nil && f > 0 && hi > 0 && lo > 0 {
			lp.focusNum, lp.focusDen = f, f+hi+lo
			lp.restHi, lp.restLo = hi, lo
		}
	}
	if layoutIndex("custom") < 0 {
		layouts = append(layouts, lp)
	}
}

// split divides a column height among its three stacked panels; the focused one
// (index 0/1/2) gets the profile's major share and the other two split the rest
// by restHi:restLo (top-most of the two gets restHi). The three always sum to h.
// A too-short column or an out-of-range focus falls back to equal thirds.
func (lp layoutProfile) split(h, focused int) [3]int {
	if h < 12 || focused < 0 || focused > 2 {
		a := h / 3
		return [3]int{a, a, h - 2*a}
	}
	f := h * lp.focusNum / lp.focusDen
	rest := h - f
	hi := rest * lp.restHi / (lp.restHi + lp.restLo)
	lo := rest - hi
	var out [3]int
	out[focused] = f
	sizes := [2]int{hi, lo}
	j := 0
	for i := 0; i < 3; i++ {
		if i == focused {
			continue
		}
		out[i] = sizes[j]
		j++
	}
	// keep the non-focused panels readable: never shrink one below minPanel rows
	// (mostly rescues spotlight's thin strips), borrowing from the focused panel
	// as long as it can spare the rows. h<12 already short-circuited above.
	const minPanel = 4
	for i := 0; i < 3; i++ {
		if i == focused || out[i] >= minPanel {
			continue
		}
		if deficit := minPanel - out[i]; out[focused]-deficit >= minPanel {
			out[focused] -= deficit
			out[i] = minPanel
		}
	}
	return out
}

// sidebarSplit is split with a negative focus (no sidebar panel active, e.g. the
// Timeline has focus) falling back to equal thirds.
func (lp layoutProfile) sidebarSplit(h, focused int) [3]int {
	if focused < 0 {
		a := h / 3
		return [3]int{a, a, h - 2*a}
	}
	return lp.split(h, focused)
}

// runningHeader shows the currently-running task with a live-ticking elapsed
// time (today's status as of liveAt, plus wall-clock seconds since). Empty when
// nothing is running.
func (m Model) runningHeader() string {
	if m.live == nil || m.liveAt.IsZero() {
		return ""
	}
	for _, t := range m.myTasks() { // your running task, not a teammate's
		if t.Status != "in-progress" {
			continue
		}
		mins := t.Minutes + int(time.Since(m.liveAt).Minutes())
		color := ProjectColor(t.Project)
		run := lipgloss.NewStyle().Foreground(color).Render(pickGlyph(">", "▶") + " " + t.ID + " [" + t.Project + "]")
		return run + " " + t.Desc + dimStyle.Render(" · "+fmtDur(mins)) + m.pauseBadge()
	}
	return dimStyle.Render(pickGlyph("-", "◦")+" idle") + m.pauseBadge()
}

// pauseBadge is a compact indicator of how start/resume treat a same-project
// running task: parallel (default) or auto-pause. Toggled with "A".
func (m Model) pauseBadge() string {
	if m.autoPause {
		return dimStyle.Render(" · " + pickGlyph("<>", "⇄") + " 1-at-a-time")
	}
	return dimStyle.Render(" · " + pickGlyph("||", "∥") + " parallel")
}

// footerLine is a short, context-sensitive hint that fits on one line; the full
// keymap lives in the ? overlay.
func (m Model) footerLine() string {
	if m.zoomed {
		return "ZOOM · h/l/1-4 switch panel · z/esc unzoom · j/k move · / search · ? help · q quit"
	}
	switch m.pane {
	case paneRange:
		return "j/k project · T today/window · h/l panel · 1-4 jump · ←→ day · [ ] window · ⇧1/2/3 span · z zoom · b by · / search · s start · ? help"
	case paneDay:
		return "j/k task · p/r/c/d pause/resume/done/defer (⇧=at time) · a/m/n amend/move/note · # tags · o carry-over · u undo · / search · ?"
	case panePending:
		return "j/k pick · enter start · a add · d due · [ ] reorder · x drop · h/l panel · z zoom · ? help"
	default:
		return "j/k scroll · ^d/^u page · h/l panel · 1-4 jump · z zoom · / search · s start · ? help · q quit"
	}
}

// searchOverlay renders the search prompt and its results list (one task per
// row: status glyph, id, description, project, day, duration), windowed to fit.
func (m Model) searchOverlay(w int) string {
	cw := w - 4 // panel content width
	var b strings.Builder
	b.WriteString(inputStyle.Render("/"+m.search.query+pickGlyph("|", "▏")) + "\n")
	if len(m.search.results) == 0 {
		if m.search.query == "" {
			b.WriteString(dimStyle.Render("  (no recorded tasks)"))
		} else {
			b.WriteString(dimStyle.Render("  (no matches)"))
		}
		return b.String()
	}
	rows := make([]string, len(m.search.results))
	for i, r := range m.search.results {
		g, gc := statusGlyph(r.Status)
		meta := fmt.Sprintf("[%s]  %s  %s", r.Project, r.Date, fmtDur(r.Minutes))
		if len(r.Tags) > 0 {
			meta += "  #" + strings.Join(r.Tags, " #")
		}
		desc := r.Desc
		if desc == "" {
			desc = "(no description)"
		}
		if i == m.search.sel {
			plain := fmt.Sprintf("%s %-4s %-32.32s %s", g, r.ID, desc, meta)
			rows[i] = selStyle.Render(padRight(plain, cw))
		} else {
			left := fmt.Sprintf("%-4s %-32.32s", r.ID, desc)
			rows[i] = lipgloss.NewStyle().Foreground(gc).Render(g) + " " +
				lipgloss.NewStyle().Foreground(ProjectColor(r.Project)).Render(left) + " " +
				dimStyle.Render(meta)
		}
	}
	maxRows := 200
	if m.height > 8 {
		maxRows = m.height - 8 // leave room for header, prompt, borders, footer
	}
	rows = windowRows(rows, m.search.sel, maxRows)
	b.WriteString(strings.Join(rows, "\n"))
	return b.String()
}

// helpOverlay is the full keymap, shown when ? is pressed.
func (m Model) helpOverlay() string {
	rows := [][2]string{
		{"~Navigation", ""},
		{"h / l", "cycle panels (wraps): Projects → Tasks → Timeline → Pending"},
		{"Tab / Shift+Tab", "cycle panels (same as l / h)"},
		{"1 / 2 / 3 / 4", "jump straight to Projects / Tasks / Timeline / Pending"},
		{"j / k", "move the selection in the focused panel"},
		{"g / G", "jump to first / last"},
		{"Ctrl+d / Ctrl+u", "half-page down / up"},
		{"← / →", "previous / next day (from any panel)"},
		{"Enter", "drill in (Projects→Tasks→Timeline) / start a pending task"},
		{"Esc", "return to Projects"},
		{"[ / ]", "shift the date window earlier / later"},
		{"t", "jump to today / recenter the window"},
		{"T", "in Projects: toggle between the Today and Window sections"},
		{"⇧1 / ⇧2 / ⇧3", "set window span: 1 / 7 / 30 days"},
		{"~View", ""},
		{"/", "search all tasks (id / project / description); Enter jumps"},
		{"b", "toggle the Projects rows between project and task"},
		{"", "selecting a project filters the day's Tasks"},
		{"", "selecting a Today-section project also jumps to today"},
		{"Shift+L", "cycle the panel layout: balanced / spotlight / golden"},
		{"z", "zoom: maximize the focused panel full-screen (esc/z to exit)"},
		{"Ctrl+R", "reload everything from disk"},
		{"~Pending (backlog panel)", ""},
		{"a", "add: 'desc @project !YYYY-MM-DD' (project/due optional)"},
		{"d", "set / clear the selected task's deadline"},
		{"Enter", "start (promote) the selected pending task"},
		{"[ / ]", "move it up / down · x drop it"},
		{"", "focusing Pending shows the selected item in full in the main column"},
		{"~Actions (on the selected task)", ""},
		{"s", "start a new task: 'desc @project %time' (both optional; ⇥ completes)"},
		{"A", "toggle start/resume: parallel (default) vs auto-pause same project"},
		{"p / r / c / d", "pause / resume / complete / defer (now)"},
		{"P / R / C / D", "same, but prompt for an explicit time (--at)"},
		{"a / m", "amend description / move (⇥ completes project)"},
		{"n", "add a note (log) to the running task"},
		{"#", "edit tags (space-separated; -tag removes; ⇥ completes a known tag)"},
		{"o", "carry over (continue) a past day's task today — copies desc + project + tags to a new id"},
		{"x / X", "cancel (void) — destructive · X also prompts for a time"},
		{"u", "undo the last logged event on the focused day"},
		{"", "on a past day, actions prompt for a time first"},
		{"", "confirm guard set by the `confirm` config: all | destructive | off"},
		{"~Text prompts (start / amend / move / note / add / due)", ""},
		{"← / → · Home/End", "move the caret · Ctrl+A / Ctrl+E also jump to ends"},
		{"⌫ / Del", "delete back / forward · Ctrl+W or Ctrl+⌫ deletes a word"},
		{"⇥", "autocomplete a known project (in start / move)"},
		{"Enter / Esc", "submit · cancel"},
		{"~General", ""},
		{"S", "sync the shared journal (git pull/push) — needs `wj sync init` once"},
		{"?", "toggle this help"},
		{"Ctrl+Z", "suspend to the background (fg to resume)"},
		{"q / Ctrl+C", "quit"},
	}
	var b strings.Builder
	for _, r := range rows {
		key, desc := r[0], r[1]
		if strings.HasPrefix(key, "~") {
			b.WriteString("\n" + titleStyle.Render(strings.TrimPrefix(key, "~")) + "\n")
			continue
		}
		b.WriteString("  " + selStyle.Render(padRight(key, 16)) + "  " + dimStyle.Render(desc) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderProjects is the sidebar list. It stacks two sections — Today (today's
// projects, from live status) and Window (the range rows, led by "All") — with a
// sub-header before each when both are present. The selected row is the
// master→detail filter; T toggles between the sections.
func (m Model) renderProjects(cw, maxRows int) string {
	if m.g == nil {
		return dimStyle.Render("(loading…)")
	}
	rows := m.projRows()
	hasToday := false
	for _, r := range rows {
		if r.today {
			hasToday = true
			break
		}
	}
	// the Window section always carries a synthetic "All" row, so fall back to the
	// empty-state message only when there is genuinely nothing: no today work and
	// no range rows.
	if !hasToday && len(m.g.Rows) == 0 {
		return dimStyle.Render("(no tracked time)")
	}
	// section subtotals shown in the headers: today = sum of its rows, window =
	// the "All" entry's total.
	todayTotal, windowTotal := 0, 0
	for _, r := range rows {
		switch {
		case r.today:
			todayTotal += r.minutes
		case r.isAll:
			windowTotal = r.minutes
		}
	}
	disp := make([]string, 0, len(rows)+2)
	activeDisp, prevToday, first := 0, false, true
	for i, r := range rows {
		if hasToday && (first || r.today != prevToday) { // section boundary header
			title, total := "Window", windowTotal
			if r.today {
				title, total = "Today", todayTotal
			}
			disp = append(disp, sectionHeader(title, total, cw))
		}
		prevToday, first = r.today, false
		glyph, gc := " ", lipgloss.Color("78")
		if r.running {
			glyph, _ = statusGlyph("in-progress") // same marker (icon or ASCII) as the task lists
		}
		lc := lipgloss.Color("250")
		if !r.isAll && r.project != "" {
			lc = ProjectColor(r.project)
		}
		if i == m.focusedRow {
			activeDisp = len(disp)
		}
		disp = append(disp, listLine(glyph, gc, lc, r.label, fmtDur(r.minutes), i == m.focusedRow, cw))
	}
	return strings.Join(windowRows(disp, activeDisp, maxRows), "\n")
}

// tagColor is the single hue used for "#tag" chips (kept distinct from the
// per-project palette so tags read as a separate, secondary axis).
var tagColor = lipgloss.Color("111")

// renderTags formats a task's tags as space-separated "#tag" chips ("" if none).
func renderTags(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	st := lipgloss.NewStyle().Foreground(tagColor)
	parts := make([]string, len(tags))
	for i, t := range tags {
		parts[i] = st.Render("#" + t)
	}
	return strings.Join(parts, " ")
}

// sectionHeader renders a bold sub-header that labels a Projects-panel section
// (Today / Window) with its subtotal right-aligned to width cw.
func sectionHeader(label string, mins, cw int) string {
	dur := fmtDur(mins)
	leftMax := cw - 1 - len([]rune(dur))
	if leftMax < 1 {
		leftMax = 1
	}
	l := padRight(truncate(label, leftMax), leftMax)
	head := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true).Render(l)
	return head + " " + dimStyle.Render(dur)
}

// renderTasks is the sidebar list of the focused day's tasks (after the project
// filter), each showing its tracked duration.
func (m Model) renderTasks(cw, maxRows int) string {
	if m.grid == nil {
		return dimStyle.Render("(no day selected)")
	}
	ts := m.filteredTasks()
	if len(ts) == 0 {
		return dimStyle.Render("(no tasks)")
	}
	items := make([]string, len(ts))
	for i, t := range ts {
		label := t.ID + " " + t.Project
		if t.Desc != "" {
			label = t.ID + " " + t.Desc
		}
		glyph, gc := statusGlyph(t.Status)
		// own tasks read in their project colour; a teammate's task (its id is
		// already shown qualified, e.g. alice/T3) is tinted by author instead.
		lc := ProjectColor(t.Project)
		if !m.taskOwned(t) {
			lc = ProjectColor(t.Actor)
		}
		items[i] = listLine(glyph, gc, lc, label, fmtDur(t.Minutes), i == m.selTask, cw)
	}
	items = windowRows(items, m.selTask, maxRows)
	return strings.Join(items, "\n")
}

// listLine formats one sidebar row: a status glyph, a left label, and a
// right-aligned value, padded to cw. The glyph carries the status hue and the
// label the project hue, so both read at once; the selected row is
// reverse-highlighted (which replaces the old "> " cursor).
func listLine(glyph string, glyphColor, labelColor lipgloss.Color, left, right string, selected bool, cw int) string {
	leftMax := cw - 3 - len([]rune(right)) // glyph(1) + space(1) + gap(1)
	if leftMax < 1 {
		leftMax = 1
	}
	l := padRight(truncate(left, leftMax), leftMax)
	if selected {
		return selStyle.Render(padRight(glyph+" "+l+" "+right, cw))
	}
	return lipgloss.NewStyle().Foreground(glyphColor).Render(glyph) + " " +
		lipgloss.NewStyle().Foreground(labelColor).Render(l) + " " + dimStyle.Render(right)
}

// statusGlyph maps a task status to its glyph and accent color. The glyph is the
// universal ASCII marker by default, or a Nerd-Font icon when icons=on (see
// pickGlyph). The same set is used in the task lists, header rollup, and legend.
func statusGlyph(status string) (string, lipgloss.Color) {
	switch status {
	case "in-progress":
		return pickGlyph(">", ""), lipgloss.Color("78") // play — running now
	case "paused":
		return pickGlyph("=", ""), lipgloss.Color("214") // pause
	case "deferred":
		return pickGlyph("»", ""), lipgloss.Color("39") // clock — set aside
	case "completed":
		return pickGlyph("x", ""), lipgloss.Color("244") // check — done
	case "cancelled":
		return pickGlyph("x", ""), lipgloss.Color("240") // ban — voided
	default:
		return " ", lipgloss.Color("244")
	}
}

// activeProject is the project of today's in-progress task ("" if none),
// used to flag the running project in the Projects list.
func (m Model) activeProject() string {
	for _, t := range m.myTasks() { // your running project, not a teammate's
		if t.Status == "in-progress" {
			return t.Project
		}
	}
	return ""
}

// todayRollup is a compact count of today's tasks by status plus the day total,
// e.g. ">1 =0 x4 · Σ16h44m" (empty until today's status has loaded).
func (m Model) todayRollup() string {
	mine := m.myTasks()
	if len(mine) == 0 {
		return ""
	}
	var run, paused, deferred, done, total int
	for _, t := range mine {
		total += t.Minutes
		switch t.Status {
		case "in-progress":
			run++
		case "paused":
			paused++
		case "deferred":
			deferred++
		case "completed":
			done++
		}
	}
	seg := func(st string, n int) string {
		g, c := statusGlyph(st)
		return lipgloss.NewStyle().Foreground(c).Render(g) + dimStyle.Render(fmt.Sprintf(" %d", n))
	}
	counts := seg("in-progress", run) + " " + seg("paused", paused)
	if deferred > 0 {
		counts += " " + seg("deferred", deferred)
	}
	counts += " " + seg("completed", done)
	return counts + dimStyle.Render(" · Σ"+fmtDur(total)) // your total, not the team's
}

// statusKey is the legend's decoder for the status glyphs.
func statusKey() string {
	items := []struct{ st, label string }{
		{"in-progress", "running"}, {"paused", "paused"},
		{"deferred", "deferred"}, {"completed", "done"},
	}
	parts := make([]string, len(items))
	for i, it := range items {
		g, c := statusGlyph(it.st)
		parts[i] = lipgloss.NewStyle().Foreground(c).Render(g) + " " + dimStyle.Render(it.label)
	}
	return dimStyle.Render("   ·   ") + strings.Join(parts, "  ")
}

// renderRange draws the multi-day matrix with per-project colored intensity
// bars, sizing the day columns to fill the available width.
func (m Model) renderRange(innerW, maxBody int) string {
	if m.g == nil {
		return "loading…"
	}
	n := len(m.g.Days)
	if n == 0 {
		return dimStyle.Render("(no days in range)")
	}
	lw := labelWidth(innerW)
	const totalW = 8 // "  10h01m"
	dw := clamp((innerW-lw-totalW)/n, 2, 24)
	max := m.maxCell()

	header := strings.Repeat(" ", lw)
	for i, d := range m.g.Days {
		lbl := center(shortDate(d), dw)
		switch {
		case i == m.focusedDay:
			lbl = focusStyle.Render(lbl)
		case d == m.today:
			lbl = todayStyle.Render(lbl) // mark today even when not focused
		default:
			lbl = dimStyle.Render(lbl)
		}
		header += lbl
	}
	header += "  " + dimStyle.Render("TOTAL")

	rows := make([]string, len(m.g.Rows))
	for ri, row := range m.g.Rows {
		color := ProjectColor(rowProject(row)) // stable per-project hue, even in --by task
		label := padRight(row.Label, lw)
		if ri == m.focusedRow-1 {
			label = selStyle.Render(label)
		} else {
			label = lipgloss.NewStyle().Foreground(color).Render(label)
		}
		line := label
		for _, d := range m.g.Days {
			line += bar(row.PerDay[d], max, dw, color)
		}
		rows[ri] = line + "  " + fmtDur(row.TotalMinutes)
	}

	rows = windowRows(rows, max2(0, m.focusedRow-1), maxBody-1) // 1 = header

	return header + "\n" + strings.Join(rows, "\n")
}

// renderDay draws the focused day's intraday Gantt: one row per task (after the
// project filter), a time axis spanning the grid's effective window (the shift
// frame, auto-expanded/auto-fit by the CLI to cover the day), colored bars for
// active segments.
func (m Model) renderDay(innerW, maxBody int) string {
	if m.grid == nil {
		return dimStyle.Render("(loading…)")
	}
	ts := m.filteredTasks()
	if len(ts) == 0 {
		return dimStyle.Render("(no tasks on this day)")
	}
	lw := labelWidth(innerW)
	start, end := hm(m.grid.ShiftStart), hm(m.grid.ShiftEnd)
	span := end - start
	if span <= 0 {
		span = 1
	}
	const metaW = 9 // " 10h01m" + slack
	axisW := innerW - lw - metaW
	if axisW < 6 {
		axisW = 6
	}
	col := func(minute int) int { return clamp((minute-start)*axisW/span, 0, axisW-1) }
	gutter := strings.Repeat(" ", lw)

	// hour-tick axis: a label every two hours across the width
	ticks := []rune(strings.Repeat(" ", axisW))
	for h := (start + 59) / 60 * 60; h <= end; h += 120 {
		lbl := []rune(fmt.Sprintf("%02d", h/60))
		at := col(h)
		if at+len(lbl) > axisW { // would overflow the right edge: pull the label in
			at = axisW - len(lbl)
		}
		for i, r := range lbl {
			if at+i >= 0 && at+i < axisW {
				ticks[at+i] = r
			}
		}
	}
	axis := gutter + dimStyle.Render(string(ticks))

	rows := make([]string, len(ts))
	for ti, t := range ts {
		color := ProjectColor(t.Project)
		cells := make([]rune, axisW)
		for i := range cells {
			cells[i] = ' '
		}
		for _, seg := range t.Segments {
			a := clamp((hm(seg.From)-start)*axisW/span, 0, axisW)
			z := clamp((hm(seg.To)-start)*axisW/span, 0, axisW)
			for i := a; i < z; i++ {
				cells[i] = '█'
			}
		}
		barStr := lipgloss.NewStyle().Foreground(color).Render(string(cells))
		glyph, gc := statusGlyph(t.Status)
		body := padRight(fmt.Sprintf("%-4s %s", t.ID, t.Project), lw-2) // glyph(1)+space(1)
		var label string
		if ti == m.selTask {
			label = selStyle.Render(padRight(glyph+" "+body, lw))
		} else {
			label = lipgloss.NewStyle().Foreground(gc).Render(glyph) + " " +
				lipgloss.NewStyle().Foreground(color).Render(body)
		}
		rows[ti] = label + barStr + " " + fmtDur(t.Minutes)
	}

	// fixed lines: axis + now-marker; window task rows into the rest
	var nowLine string
	if nm := hm(m.grid.Now); nm >= start && nm <= end {
		marker := []rune(strings.Repeat(" ", axisW))
		marker[col(nm)] = []rune(pickGlyph("^", "▲"))[0]
		nowLine = gutter + dimStyle.Render(string(marker))
	}
	reserved := 1 // axis
	if nowLine != "" {
		reserved++
	}
	rows = windowRows(rows, m.selTask, maxBody-reserved)

	out := axis + "\n" + strings.Join(rows, "\n")
	if nowLine != "" {
		out += "\n" + nowLine
	}
	return out
}

// renderTimeline lists the selected task's events, scrollable via tlOffset. Long
// descriptions word-wrap with a hanging indent so continuation lines align under
// the description column rather than flowing back to the left edge.
func (m Model) renderTimeline(innerW, maxBody int) string {
	if m.show == nil {
		return dimStyle.Render("(select a task)")
	}
	s := m.show
	cw := max2(innerW, 8)
	head := lipgloss.NewStyle().Foreground(ProjectColor(s.Project)).
		Render(hangingRow(fmt.Sprintf("%s  [%s]  ", s.ID, s.Project), s.Desc, cw))
	sub := dimStyle.Render(fmt.Sprintf("%s · %s · %s", s.Date, s.Status, fmtDur(s.Minutes)))

	rows := make([]string, len(s.Events))
	for i, e := range s.Events {
		label, extra := timelineLabel(e)
		// columns: 2 gutter + time(5) + 2 + status(9, left) + 2, then the desc
		rows[i] = hangingRow(fmt.Sprintf("  %s  %-9s  ", e.Time, label), extra, cw)
	}
	header := head + "\n" + sub
	if tg := renderTags(s.Tags); tg != "" { // a tags line under the project/status
		header += "\n" + tg
	}
	rows = windowRows(rows, m.tlOffset, maxBody-lineCount(header))
	return header + "\n" + strings.Join(rows, "\n")
}

// timelineLabel maps an event to its human label + trailing detail (mirrors the
// CLI's `show` command).
func timelineLabel(e wj.Event) (string, string) {
	switch e.Event {
	case "start":
		return "started", e.Note
	case "resume":
		return "resumed", ""
	case "pause":
		return "paused", dash(e.Note)
	case "defer":
		return "deferred", dash(e.Note)
	case "log":
		return "note", e.Note
	case "amend":
		return "renamed", e.Note
	case "move":
		return "moved", e.Note + " -> " + e.Project
	case "cancel":
		return "cancelled", ""
	case "complete":
		return "completed", ""
	}
	return e.Event, e.Note
}

func dash(s string) string {
	if s == "" {
		return ""
	}
	return "— " + s
}

// rowProject is the project a gantt row should be colored by (falls back to the
// row key for older payloads that predate the project field).
func rowProject(row wj.GanttRow) string {
	if row.Project != "" {
		return row.Project
	}
	return row.Key
}

func (m Model) maxCell() int {
	max := 0
	for _, row := range m.g.Rows {
		for _, v := range row.PerDay {
			if v > max {
				max = v
			}
		}
	}
	return max
}

// bar renders a single intensity cell scaled to max, in the given color.
func bar(minutes, max, width int, color lipgloss.Color) string {
	if minutes <= 0 || max <= 0 {
		return strings.Repeat(" ", width)
	}
	filled := int(math.Round(float64(minutes) / float64(max) * float64(width)))
	if filled < 1 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	fill := lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("█", filled))
	return fill + strings.Repeat(" ", width-filled)
}

// panel wraps body in a titled, bordered box; the active pane is highlighted.
// A positive height forces the box to that total height (content top-aligned),
// so stacked panels fill the screen; height 0 leaves it content-sized.
func panel(title string, tcolor lipgloss.Color, body string, active bool, width, height int) string {
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	if active {
		st = st.BorderForeground(accent) // focused panel: accent border
	} else {
		st = st.BorderForeground(lipgloss.Color("240"))
	}
	if width > 6 {
		st = st.Width(width - 2)
	}
	// the title carries the panel's own signature color; the focused panel also
	// underlines it (its border already turns the accent color).
	hs := lipgloss.NewStyle().Bold(true).Foreground(tcolor)
	if active {
		hs = hs.Underline(true)
	}
	heading := hs.Render(title)
	inner := heading + "\n" + body
	if height > 2 {
		st = st.Height(height - 2)
		inner = clipLines(inner, height-2) // never grow past the forced height
	}
	return st.Render(inner)
}

// clipLines keeps at most the first n lines of s (a guard so a panel's content
// can't exceed its forced height and push the layout past the screen).
func clipLines(s string, n int) string {
	if n < 1 {
		n = 1
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
}

// accent is the main UI color: it draws the focused panel's border and the top
// header line. It defaults to purple and is overridable from the config file
// (accent=…) via SetAccent.
const defaultAccent = "141" // 256-color violet
var accent = lipgloss.Color(defaultAccent)

// Each of the six panels carries its own signature color, shown on its title,
// so the panels stay visually distinct regardless of which one is focused. All
// overridable from the config file (color_projects=, color_tasks=, …) via
// SetPanelColors. Defaults are picked to be distinct from each other and from
// the purple accent.
var (
	colorProjects = lipgloss.Color("39")  // blue
	colorTasks    = lipgloss.Color("214") // amber
	colorPending  = lipgloss.Color("170") // orchid
	colorRange    = lipgloss.Color("78")  // green
	colorDay      = lipgloss.Color("45")  // cyan
	colorTimeline = lipgloss.Color("180") // tan
)

// styles. The accent-derived ones are (re)built by applyTheme(); the rest are
// fixed because their color carries meaning.
var (
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	todayStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("84"))                                    // green = today
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))                                              // red = error
	noticeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("45"))                                               // cyan: neutral feedback
	selStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("238")) // selected row
	inputStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))                                   // yellow: typing

	titleStyle lipgloss.Style // the top header line (accent)
	focusStyle lipgloss.Style // the focused day label in the range view (accent)
)

func init() { applyTheme() }

// applyTheme rebuilds the accent-derived styles from the current accent.
func applyTheme() {
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(accent)
	focusStyle = lipgloss.NewStyle().Bold(true).Foreground(accent).Underline(true)
}

// SetAccent overrides the main UI color (focused border + header) from a
// lipgloss spec — a 256-color code ("141"), a hex value ("#9d7cd8"), or an ANSI
// name. An empty spec is ignored, keeping the purple default. Call once at
// startup, before the program runs.
func SetAccent(spec string) {
	if spec == "" {
		return
	}
	accent = lipgloss.Color(spec)
	applyTheme()
}

// SetPanelColors overrides the per-panel title colors from a "name=spec,…"
// string, e.g. "projects=39,timeline=#888888". Recognised names: projects,
// tasks, pending, range, day, timeline. Unknown names and empty specs are
// ignored, keeping the defaults. Call once at startup.
func SetPanelColors(spec string) {
	for _, pair := range strings.Split(spec, ",") {
		name, val, ok := strings.Cut(pair, "=")
		name, val = strings.TrimSpace(name), strings.TrimSpace(val)
		if !ok || val == "" {
			continue
		}
		c := lipgloss.Color(val)
		switch name {
		case "projects":
			colorProjects = c
		case "tasks":
			colorTasks = c
		case "pending":
			colorPending = c
		case "range":
			colorRange = c
		case "day":
			colorDay = c
		case "timeline":
			colorTimeline = c
		}
	}
}

// windowRows returns at most max of rows, scrolled to keep `active` visible.
// When content is hidden, the edge rows become "↑/↓ N more" indicators.
func windowRows(rows []string, active, max int) []string {
	n := len(rows)
	if max < 1 {
		max = 1
	}
	if n <= max {
		return rows
	}
	start := active - max/2
	if start < 0 {
		start = 0
	}
	if start > n-max {
		start = n - max
	}
	end := start + max
	out := append([]string(nil), rows[start:end]...)
	if start > 0 {
		out[0] = dimStyle.Render(fmt.Sprintf("  %s %d more", pickGlyph("^", "↑"), start))
	}
	if end < n {
		out[len(out)-1] = dimStyle.Render(fmt.Sprintf("  %s %d more", pickGlyph("v", "↓"), n-end))
	}
	return out
}
