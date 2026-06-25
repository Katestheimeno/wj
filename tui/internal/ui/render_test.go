package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Katestheimeno/wj/tui/internal/wj"
)

// keyMsg builds a tea.KeyMsg whose String() matches the keys handleKey switches on.
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "shift+tab":
		return tea.KeyMsg{Type: tea.KeyShiftTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+z":
		return tea.KeyMsg{Type: tea.KeyCtrlZ}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func sampleModel() Model {
	return Model{
		by: "project", from: "2026-05-28", to: "2026-06-01", ready: true,
		confirmLevel: confirmAll, // exercise the confirm guards by default in tests
		g: &wj.Gantt{
			From: "2026-05-28", To: "2026-06-01", By: "project",
			Days: []string{"2026-05-28", "2026-05-29", "2026-05-30", "2026-05-31", "2026-06-01"},
			Rows: []wj.GanttRow{
				{Key: "backend", Label: "backend", TotalMinutes: 601, PerDay: map[string]int{"2026-05-28": 180, "2026-06-01": 421}},
				{Key: "meetings", Label: "meetings", TotalMinutes: 30, PerDay: map[string]int{"2026-05-28": 30}},
			},
		},
	}
}

func TestRenderGantt(t *testing.T) {
	out := sampleModel().View()
	for _, want := range []string{"backend", "meetings", "05-28", "06-01", "10h01m", "30m", "by project"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q\n---\n%s", want, out)
		}
	}
}

func TestEmptyRange(t *testing.T) {
	m := sampleModel()
	m.g.Rows = nil
	if out := m.View(); !strings.Contains(out, "no tracked time") {
		t.Errorf("empty range should note no time:\n%s", out)
	}
}

func TestBar(t *testing.T) {
	// any positive work shows at least one block; zero shows none
	if got := bar(0, 100, 7, "39", false); strings.Contains(got, "█") {
		t.Errorf("zero minutes should render no block, got %q", got)
	}
	if got := bar(1, 100, 7, "39", false); !strings.Contains(got, "█") {
		t.Errorf("tiny work should render a sliver, got %q", got)
	}
	// every cell is exactly width-wide once color codes are stripped
	if w := len([]rune(stripANSI(bar(50, 100, 7, "39", false)))); w != 7 {
		t.Errorf("bar width = %d, want 7", w)
	}
	// a running bar keeps its width but caps the leading edge with the chevron
	// (ascii fallback in tests) instead of a full block
	run := stripANSI(bar(100, 100, 7, "39", true))
	if w := len([]rune(run)); w != 7 {
		t.Errorf("running bar width = %d, want 7", w)
	}
	if !strings.HasSuffix(strings.TrimRight(run, " "), pickGlyph(">", "►")) {
		t.Errorf("running bar should end in the chevron tip, got %q", run)
	}
}

// In the Day panel an in-progress task's bar tapers to a chevron; a settled
// (completed/paused) task stays a flat rectangle.
func TestRenderDayTapersRunningTask(t *testing.T) {
	m := sampleModel()
	m.pane = paneDay
	m.grid = &wj.Grid{
		Date: "2026-06-10", ShiftStart: "09:00", ShiftEnd: "19:00", Now: "12:00",
		Tasks: []wj.GridTask{
			{ID: "T1", Project: "backend", Status: "in-progress", Minutes: 180,
				Segments: []wj.Segment{{From: "09:00", To: "12:00"}}},
			{ID: "T2", Project: "meetings", Status: "completed", Minutes: 60,
				Segments: []wj.Segment{{From: "13:00", To: "14:00"}}},
		},
	}
	chevron := pickGlyph(">", "►")
	var t1, t2 string
	for _, ln := range strings.Split(stripANSI(m.renderDay(80, 20)), "\n") {
		switch {
		case strings.Contains(ln, "T1"):
			t1 = ln
		case strings.Contains(ln, "T2"):
			t2 = ln
		}
	}
	if !strings.Contains(t1, chevron) {
		t.Errorf("running T1 bar should carry the chevron tip: %q", t1)
	}
	if strings.Contains(t2, chevron) {
		t.Errorf("completed T2 bar must stay a flat rectangle: %q", t2)
	}
}

// In the Range panel only today's cell of a currently-running project is
// tipped; a non-running project's row (and past cells) stay flat.
func TestRenderRangeTipsRunningProjectToday(t *testing.T) {
	m := sampleModel() // by project; rows backend + meetings; days 05-28..06-01
	m.today = "2026-06-01"
	m.live = &wj.Status{Date: "2026-06-01", Tasks: []wj.Task{
		{ID: "T1", Project: "backend", Status: "in-progress", Minutes: 60},
	}}
	chevron := pickGlyph(">", "►")
	var backend, meetings string
	for _, ln := range strings.Split(stripANSI(m.renderRange(80, 20)), "\n") {
		switch {
		case strings.Contains(ln, "backend"):
			backend = ln
		case strings.Contains(ln, "meetings"):
			meetings = ln
		}
	}
	if !strings.Contains(backend, chevron) {
		t.Errorf("running backend should tip its today cell: %q", backend)
	}
	if strings.Contains(meetings, chevron) {
		t.Errorf("non-running meetings must stay a flat rectangle: %q", meetings)
	}
}

// The gantt-row index the Range panel highlights must account for the Today
// section: selecting a Today/All row maps to <0 (no gantt highlight), and the
// first gantt project maps to index 0 regardless of how many Today rows precede.
func TestSelectedGanttRowOffset(t *testing.T) {
	// liveModel has a 2-row Today section, so allRow()==2.
	live := liveModel()
	for focused, want := range map[int]int{
		0: -3, // today backend  → no gantt row
		1: -2, // today frontend → no gantt row
		2: -1, // the "All" entry → no gantt row
		3: 0,  // first gantt project (backend)
		4: 1,  // second gantt project (meetings)
	} {
		live.focusedRow = focused
		if got := live.selectedGanttRow(); got != want {
			t.Errorf("live focusedRow=%d → selectedGanttRow %d, want %d", focused, got, want)
		}
	}
	// Without a Today section (allRow()==0) the mapping is the legacy one:
	// "All" at 0 → -1, the first gantt project at 1 → 0.
	plain := sampleModel()
	if got := plain.selectedGanttRow(); got != -1 { // focusedRow 0 = All
		t.Errorf("no-Today All row → %d, want -1", got)
	}
	plain.focusedRow = 1
	if got := plain.selectedGanttRow(); got != 0 {
		t.Errorf("no-Today first gantt row → %d, want 0", got)
	}
}

func TestFmtDur(t *testing.T) {
	cases := map[int]string{0: "0m", 5: "5m", 60: "1h00m", 601: "10h01m", 90: "1h30m"}
	for in, want := range cases {
		if got := fmtDur(in); got != want {
			t.Errorf("fmtDur(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestSetAccent(t *testing.T) {
	defer SetAccent(defaultAccent) // restore the default for other tests
	SetAccent("99")
	// the accent drives the header/border style (titleStyle) and the package
	// `accent` color used for the focused panel border.
	if got := titleStyle.GetForeground(); got != lipgloss.Color("99") {
		t.Errorf("titleStyle foreground = %v, want 99", got)
	}
	if accent != lipgloss.Color("99") {
		t.Errorf("accent = %v, want 99", accent)
	}
	SetAccent("") // empty is a no-op (keeps the previous accent)
	if accent != lipgloss.Color("99") {
		t.Errorf("empty SetAccent should be a no-op, accent = %v", accent)
	}
}

func TestDefaultAccentIsApplied(t *testing.T) {
	SetAccent(defaultAccent)
	if got := titleStyle.GetForeground(); got != lipgloss.Color(defaultAccent) {
		t.Errorf("default accent not applied: titleStyle foreground = %v, want %s", got, defaultAccent)
	}
}

func TestSetPanelColors(t *testing.T) {
	// restore the defaults for other tests
	defer SetPanelColors("projects=39,tasks=214,pending=170,range=78,day=45,timeline=180")

	SetPanelColors("projects=99,timeline=#abcdef")
	if colorProjects != lipgloss.Color("99") {
		t.Errorf("projects color = %v, want 99", colorProjects)
	}
	if colorTimeline != lipgloss.Color("#abcdef") {
		t.Errorf("timeline color = %v, want #abcdef", colorTimeline)
	}
	if colorTasks != lipgloss.Color("214") {
		t.Errorf("an unspecified panel must keep its color, tasks = %v, want 214", colorTasks)
	}

	// malformed / empty entries are ignored, leaving colors untouched
	SetPanelColors("garbage,day=,=99,")
	if colorDay != lipgloss.Color("45") {
		t.Errorf("malformed input must not change a color, day = %v, want 45", colorDay)
	}
}

func TestProjectColorStable(t *testing.T) {
	first := ProjectColor("backend")
	second := ProjectColor("backend")
	if first != second {
		t.Error("project color must be deterministic")
	}
}

// distinct projects (up to the palette size) must never share a color — a plain
// hash%N collides well before that, which is the duplication this guards against.
func TestProjectColorNoDuplicateWithinPalette(t *testing.T) {
	resetColorReg()
	names := []string{"meetings", "backend", "frontend", "design", "infra", "docs", "research"}
	first := map[string]lipgloss.Color{}
	seen := map[lipgloss.Color]string{}
	for _, n := range names {
		c := ProjectColor(n)
		if other, dup := seen[c]; dup {
			t.Errorf("%q and %q share color %v", other, n, c)
		}
		seen[c] = n
		first[n] = c
	}
	// the assignment is stable on a second pass (memoised for the session)
	for _, n := range names {
		if got := ProjectColor(n); got != first[n] {
			t.Errorf("%q color changed: %v -> %v", n, first[n], got)
		}
	}
}

func TestHM(t *testing.T) {
	cases := map[string]int{"09:00": 540, "00:00": 0, "23:59": 1439, "bad": 0, "": 0}
	for in, want := range cases {
		if got := hm(in); got != want {
			t.Errorf("hm(%q) = %d, want %d", in, got, want)
		}
	}
}

// drilled returns a model with all three panels populated, on the Day pane.
func drilled() Model {
	m := sampleModel()
	m.pane = paneDay
	m.grid = &wj.Grid{
		Date: "2026-05-28", ShiftStart: "09:00", ShiftEnd: "19:00", Now: "12:30",
		Tasks: []wj.GridTask{
			{ID: "T1", Project: "backend", Desc: "Refactor auth", Status: "completed", Minutes: 180,
				Segments: []wj.Segment{{From: "09:00", To: "10:30", State: "pause"}, {From: "11:00", To: "12:30", State: "complete"}}},
		},
	}
	m.show = &wj.Show{
		ID: "T1", Date: "2026-05-28", Project: "backend", Status: "completed", Desc: "Refactor auth", Minutes: 180,
		Events: []wj.Event{
			{Time: "09:00", Event: "start", Project: "backend", Note: "Refactor auth"},
			{Time: "10:30", Event: "pause", Project: "backend"},
			{Time: "11:00", Event: "resume", Project: "backend"},
			{Time: "12:30", Event: "complete", Project: "backend"},
		},
	}
	return m
}

func TestDrillDownRender(t *testing.T) {
	out := drilled().View()
	// timeline content, panel title, intraday legend, and the now (12:30) marker
	for _, want := range []string{"09:00", "T1", "backend", "Refactor auth", "started", "completed", "Day — 2026-05-28", "legend:", "^"} {
		if !strings.Contains(out, want) {
			t.Errorf("drilled view missing %q\n---\n%s", want, out)
		}
	}
}

func TestStaleResultsDiscarded(t *testing.T) {
	m := sampleModel() // focusedDay 0 -> "2026-05-28"
	// a grid result for a day we are NOT focused on must be ignored
	updated, _ := m.Update(gridMsg{day: "2026-05-31", g: &wj.Grid{Date: "2026-05-31"}})
	if updated.(Model).grid != nil {
		t.Error("stale gridMsg should be discarded")
	}
	// matching day is accepted
	updated, _ = m.Update(gridMsg{day: "2026-05-28", g: &wj.Grid{Date: "2026-05-28"}})
	if updated.(Model).grid == nil {
		t.Error("matching gridMsg should be applied")
	}
}

func TestPaneCycle(t *testing.T) {
	m := sampleModel()
	step := func(mod Model, key string) Model {
		next, _ := mod.handleKey(keyMsg(key))
		return next.(Model)
	}
	if m = step(m, "tab"); m.pane != paneDay {
		t.Fatalf("tab from range -> %d, want paneDay", m.pane)
	}
	if m = step(m, "tab"); m.pane != paneTimeline {
		t.Fatalf("tab -> %d, want paneTimeline", m.pane)
	}
	if m = step(m, "tab"); m.pane != panePending {
		t.Fatalf("tab -> %d, want panePending", m.pane)
	}
	if m = step(m, "tab"); m.pane != paneRange {
		t.Fatalf("tab wraps -> %d, want paneRange", m.pane)
	}
	// l cycles forward like Tab, wrapping past Pending back to Projects
	for _, want := range []pane{paneDay, paneTimeline, panePending, paneRange} {
		if m = step(m, "l"); m.pane != want {
			t.Fatalf("l -> %d, want %d", m.pane, want)
		}
	}
	// h cycles backward, wrapping Projects back to Pending
	for _, want := range []pane{panePending, paneTimeline, paneDay, paneRange} {
		if m = step(m, "h"); m.pane != want {
			t.Fatalf("h -> %d, want %d", m.pane, want)
		}
	}
	// 1-4 jump straight to a panel from anywhere
	for key, want := range map[string]pane{"1": paneRange, "2": paneDay, "3": paneTimeline, "4": panePending} {
		m.pane = paneTimeline // start somewhere unrelated each time
		if m = step(m, key); m.pane != want {
			t.Fatalf("%q -> %d, want %d", key, m.pane, want)
		}
	}
	// esc from a non-range pane returns to range
	m.pane = paneTimeline
	if m = step(m, "esc"); m.pane != paneRange {
		t.Fatalf("esc -> %d, want paneRange", m.pane)
	}
}

func TestPendingEmptyHintFocusAware(t *testing.T) {
	m := sampleModel() // no pending items
	// unfocused: bare "(empty)", no misleading key affordance
	m.pane = paneRange
	if out := m.renderPending(40, 1<<30, m.pane == panePending); !strings.Contains(out, "(empty)") || strings.Contains(out, "press a") {
		t.Errorf("unfocused pending hint = %q, want bare (empty)", out)
	}
	// focused: spell out the add affordance
	m.pane = panePending
	if out := m.renderPending(40, 1<<30, m.pane == panePending); !strings.Contains(out, "press a to add") {
		t.Errorf("focused pending hint = %q, want the add affordance", out)
	}
}

func TestRangeNavClamps(t *testing.T) {
	m := sampleModel() // 5 days, focusedDay starts 0
	// left at the start is a no-op (no underflow)
	next, _ := m.keyRange(keyMsg("left"))
	if next.(Model).focusedDay != 0 {
		t.Errorf("left at day 0 = %d, want 0", next.(Model).focusedDay)
	}
	// walk right past the end stays clamped at len-1
	cur := m
	for i := 0; i < 10; i++ {
		n, _ := cur.keyRange(keyMsg("right"))
		cur = n.(Model)
	}
	if cur.focusedDay != len(cur.g.Days)-1 {
		t.Errorf("focusedDay = %d, want %d", cur.focusedDay, len(cur.g.Days)-1)
	}
}

func TestPadCenter(t *testing.T) {
	if got := padRight("ab", 5); got != "ab   " {
		t.Errorf("padRight = %q", got)
	}
	if got := padRight("toolonglabel", 4); got != "tool" {
		t.Errorf("padRight truncate = %q", got)
	}
	if got := center("ab", 6); len([]rune(got)) != 6 {
		t.Errorf("center width = %d", len([]rune(got)))
	}
}

func TestInputAndConfirmRender(t *testing.T) {
	// active input prompt shows in the footer area
	m := drilled()
	m.input = inputMode{active: true, action: "amend", prompt: "amend T1 (new description)", value: "Refac", cursor: 5}
	out := m.View()
	if !strings.Contains(out, "amend T1 (new description): Refac") {
		t.Errorf("input prompt not rendered:\n%s", out)
	}
	if !strings.Contains(out, "confirm") {
		t.Errorf("input help (enter/esc) not rendered:\n%s", out)
	}
	// active confirm shows the prompt + [y/n]
	m2 := drilled()
	m2.confirm = confirmMode{active: true, prompt: "cancel (void) T1?"}
	out2 := m2.View()
	if !strings.Contains(out2, "cancel (void) T1?") || !strings.Contains(out2, "[y/n]") {
		t.Errorf("confirm prompt not rendered:\n%s", out2)
	}
}

func TestStartOpensInput(t *testing.T) {
	m := sampleModel() // range pane
	// 's' arms a confirm; confirming with 'y' opens the start prompt
	m, _ = mustModel(m.handleKey(keyMsg("s")))
	if !m.confirm.active || !m.confirm.input.active {
		t.Fatalf("'s' should arm a start confirm, got %+v", m.confirm)
	}
	next, cmd := m.handleKey(keyMsg("y"))
	nm := next.(Model)
	if !nm.input.active || nm.input.action != "start" {
		t.Fatalf("confirming 's' should open the start prompt, got %+v", nm.input)
	}
	if cmd != nil {
		t.Error("opening the prompt should not issue a command yet")
	}
}

func TestStartRequiresDescription(t *testing.T) {
	m := sampleModel()
	m, _ = mustModel(m.handleKey(keyMsg("s"))) // arm the start confirm
	m, _ = mustModel(m.handleKey(keyMsg("y"))) // confirm → open the start prompt
	// type a bare "@proj" with no task text, then submit
	m.input.value = "@proj"
	m, _ = mustModel(m.handleKey(keyMsg("enter")))
	// the prompt must stay open (not silently vanish) with a clear, tailored hint
	if !m.input.active {
		t.Fatal("project-only start should keep the prompt open, not submit")
	}
	if !strings.Contains(m.err, "@proj needs task text too") {
		t.Errorf("hint = %q, want it to name the @proj-needs-text case", m.err)
	}
	// and the hint must not echo a misleading "e.g. ... @proj" example
	if strings.Contains(m.err, "e.g.") {
		t.Errorf("hint should not use a cryptic e.g. example: %q", m.err)
	}
}

func TestInputTypingAndSubmit(t *testing.T) {
	m := drilled()
	// open amend on the selected task: 'a' arms a confirm, 'y' opens the prompt
	m, _ = mustModel(m.handleKey(keyMsg("a")))
	if !m.confirm.active || !m.confirm.input.active {
		t.Fatalf("'a' should arm an amend confirm, got %+v", m.confirm)
	}
	m, _ = mustModel(m.handleKey(keyMsg("y")))
	if !m.input.active || m.input.action != "amend" || m.input.taskID != "T1" {
		t.Fatalf("confirming 'a' should open amend for T1, got %+v", m.input)
	}
	// type "hi", then backspace -> "h", then a space + "x" -> "h x"
	for _, k := range []string{"h", "i"} {
		m, _ = mustModel(m.handleKey(keyMsg(k)))
	}
	m, _ = mustModel(m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace}))
	if m.input.value != "h" {
		t.Fatalf("after backspace value = %q, want %q", m.input.value, "h")
	}
	// enter closes the prompt
	m, _ = mustModel(m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}))
	if m.input.active {
		t.Error("enter should close the input")
	}
}

func TestInputEscCancels(t *testing.T) {
	m := drilled()
	m, _ = mustModel(m.handleKey(keyMsg("m")))
	if !m.confirm.active || !m.confirm.input.active {
		t.Fatal("'m' should arm a move confirm")
	}
	m, _ = mustModel(m.handleKey(keyMsg("y")))
	if !m.input.active {
		t.Fatal("confirming 'm' should open move prompt")
	}
	m, _ = mustModel(m.handleKey(tea.KeyMsg{Type: tea.KeyEsc}))
	if m.input.active {
		t.Error("esc should cancel the input")
	}
}

func TestCancelConfirmFlow(t *testing.T) {
	m := drilled()
	// 'x' opens a confirm guard, not an immediate cancel
	m, cmd := mustModel(m.handleKey(keyMsg("x")))
	if !m.confirm.active || cmd != nil {
		t.Fatalf("'x' should open a confirm guard, got active=%v cmd=%v", m.confirm.active, cmd)
	}
	if m.confirm.verb != "cancel" || strings.Join(m.confirm.valueArgs, " ") != "T1" {
		t.Errorf("confirm = verb %q valueArgs %v", m.confirm.verb, m.confirm.valueArgs)
	}
	// 'n' aborts
	m2, _ := mustModel(m.handleKey(keyMsg("n")))
	if m2.confirm.active {
		t.Error("'n' should dismiss the confirm")
	}
	// 'y' confirms and issues a command
	m3, cmd := mustModel(m.handleKey(keyMsg("y")))
	if m3.confirm.active {
		t.Error("'y' should dismiss the confirm")
	}
	if cmd == nil {
		t.Error("'y' should issue the cancel command")
	}
}

func TestBaseArgs(t *testing.T) {
	cases := []struct {
		verb string
		vals []string
		day  string
		want string
	}{
		{"pause", []string{"T1"}, "2026-05-28", "pause T1 --date 2026-05-28"},
		{"start", []string{"Refactor auth"}, "2026-05-28", "start Refactor auth --date 2026-05-28"},
		{"move", []string{"T2", "frontend"}, "2026-05-28", "move T2 frontend --date 2026-05-28"},
	}
	for _, c := range cases {
		if got := strings.Join(baseArgs(c.verb, c.vals, c.day), " "); got != c.want {
			t.Errorf("baseArgs(%q,%v,%q) = %q, want %q", c.verb, c.vals, c.day, got, c.want)
		}
	}
}

func TestTodayMutationIsImmediate(t *testing.T) {
	m := drilled()
	m.today = m.currentDay() // focused day is "today"
	m, _ = mustModel(m.handleKey(keyMsg("p")))
	if !m.confirm.active {
		t.Fatal("'p' should arm a confirm")
	}
	next, cmd := mustModel(m.handleKey(keyMsg("y")))
	if next.input.active {
		t.Error("a today mutation must not open a time prompt")
	}
	if cmd == nil {
		t.Error("a today mutation should issue immediately after confirming")
	}
}

func TestPastDayMutationPromptsForTime(t *testing.T) {
	m := drilled()         // focused day 2026-05-28
	m.today = "2026-06-01" // …which is in the past
	m, _ = mustModel(m.handleKey(keyMsg("p")))
	if !m.confirm.active {
		t.Fatal("'p' should arm a confirm")
	}
	next, cmd := mustModel(m.handleKey(keyMsg("y")))
	if !next.input.active || next.input.action != "at" {
		t.Fatalf("past-day pause should open a time prompt after confirming, got %+v", next.input)
	}
	if cmd != nil {
		t.Error("must not mutate before a time is given")
	}
	if got := strings.Join(next.input.pending, " "); got != "pause T1 --date 2026-05-28" {
		t.Errorf("pending argv = %q", got)
	}
	// type a time and submit -> issues the mutation (argv + --at)
	for _, k := range []string{"1", "4", ":", "3", "0"} {
		next, _ = mustModel(next.handleKey(keyMsg(k)))
	}
	if next.input.value != "14:30" {
		t.Fatalf("typed value = %q", next.input.value)
	}
	done, cmd2 := mustModel(next.handleKey(tea.KeyMsg{Type: tea.KeyEnter}))
	if done.input.active {
		t.Error("enter should close the time prompt")
	}
	if cmd2 == nil {
		t.Error("a completed time entry should issue the mutation")
	}
}

func TestTimedMutationPromptsEvenOnToday(t *testing.T) {
	m := drilled()
	m.today = m.currentDay() // focused day is "today" — a plain 'p' would run now
	// Shift+P asks for an explicit time instead of acting immediately.
	m, _ = mustModel(m.handleKey(keyMsg("P")))
	if !m.confirm.active || !m.confirm.atTime {
		t.Fatalf("Shift+P should arm a timed confirm, got %+v", m.confirm)
	}
	next, cmd := mustModel(m.handleKey(keyMsg("y")))
	if !next.input.active || next.input.action != "at" {
		t.Fatalf("confirming Shift+P should open a time prompt, got %+v", next.input)
	}
	if cmd != nil {
		t.Error("must not mutate before a time is given")
	}
	if got := strings.Join(next.input.pending, " "); got != "pause T1 --date "+m.currentDay() {
		t.Errorf("pending argv = %q", got)
	}
	// a blank submit keeps the prompt open with a hint (not a silent dismiss)
	still, cmd2 := mustModel(next.handleKey(tea.KeyMsg{Type: tea.KeyEnter}))
	if !still.input.active {
		t.Error("a blank time should keep the prompt open, not dismiss it")
	}
	if cmd2 != nil {
		t.Error("a blank time should not mutate")
	}
	if still.err == "" {
		t.Error("a blank time should surface a hint")
	}
	// esc then aborts outright and clears the hint
	gone, _ := mustModel(still.handleKey(tea.KeyMsg{Type: tea.KeyEsc}))
	if gone.input.active || gone.err != "" {
		t.Error("esc should close the prompt and clear the hint")
	}
}

func TestTimedCancelConfirmsThenPromptsForTime(t *testing.T) {
	m := drilled()
	m.today = m.currentDay()
	// Shift+X opens the destructive-confirm guard, flagged to ask for a time next.
	next, _ := mustModel(m.handleKey(keyMsg("X")))
	if !next.confirm.active || next.confirm.verb != "cancel" || !next.confirm.atTime {
		t.Fatalf("Shift+X should arm a timed cancel confirm, got %+v", next.confirm)
	}
	// confirming does NOT run yet — it opens the time prompt
	armed, cmd := mustModel(next.handleKey(keyMsg("y")))
	if cmd != nil {
		t.Error("confirming a timed cancel must not mutate before a time is given")
	}
	if !armed.input.active || armed.input.action != "at" {
		t.Fatalf("confirming should open the time prompt, got %+v", armed.input)
	}
	if got := strings.Join(armed.input.pending, " "); got != "cancel T1 --date "+m.currentDay() {
		t.Errorf("pending argv = %q", got)
	}
}

func TestMutationErrorStaysVisible(t *testing.T) {
	m := drilled()
	// a failed mutation records the error
	m, _ = mustModel(m.Update(mutationMsg{err: errors.New("no such task today: T9")}))
	if m.err == "" {
		t.Fatal("failed mutation should set m.err")
	}
	// a subsequent *successful* background reload must NOT wipe it
	m, _ = mustModel(m.Update(ganttMsg{g: sampleModel().g}))
	if m.err == "" {
		t.Error("a load success must not clear a mutation error")
	}
	// but the next keypress dismisses it
	m, _ = mustModel(m.handleKey(keyMsg("j")))
	if m.err != "" {
		t.Error("a keypress should dismiss the error")
	}
}

func TestMutationNoticeShownAndCollapsed(t *testing.T) {
	m := drilled()
	// a successful mutation echoes the CLI's confirmation as a notice. A
	// multi-line reply must collapse to a single line so it can't break the
	// fixed-height footer.
	m, _ = mustModel(m.Update(mutationMsg{note: "T1  10:00  completed — 1h00m\n      extra detail line"}))
	if m.notice == "" {
		t.Fatal("successful mutation should set m.notice")
	}
	if strings.Contains(m.notice, "\n") {
		t.Errorf("notice must be a single line, got %q", m.notice)
	}
	if m.err != "" {
		t.Error("a successful mutation must not set m.err")
	}
	// the idempotent no-op message is surfaced (whitespace collapsed to single
	// spaces, which is all a one-line footer needs)
	m, _ = mustModel(m.Update(mutationMsg{note: "T1  already completed"}))
	if m.notice != "T1 already completed" {
		t.Errorf("no-op notice = %q, want %q", m.notice, "T1 already completed")
	}
	// the next keypress dismisses it
	m, _ = mustModel(m.handleKey(keyMsg("j")))
	if m.notice != "" {
		t.Error("a keypress should dismiss the notice")
	}
}

func TestMutationKeyGatedToDetailPane(t *testing.T) {
	// in the range pane, 'c' is not a mutation (no selected task context there)
	m := sampleModel() // paneRange
	_, cmd := m.handleKey(keyMsg("c"))
	if cmd != nil {
		t.Error("'c' in range pane should not trigger a mutation")
	}
	// in the day pane with a selection, 'p' arms a confirm; 'y' issues the command
	d := drilled()
	d, _ = mustModel(d.handleKey(keyMsg("p")))
	if !d.confirm.active {
		t.Fatal("'p' on a selected task should arm a confirm")
	}
	if _, cmd := d.handleKey(keyMsg("y")); cmd == nil {
		t.Error("confirming 'p' should issue a mutation command")
	}
}

func mustModel(mod tea.Model, cmd tea.Cmd) (Model, tea.Cmd) {
	return mod.(Model), cmd
}

func TestCtrlZSuspends(t *testing.T) {
	// ctrl+z from the normal panes must emit Bubble Tea's SuspendMsg so the
	// program drops to the background (job control).
	_, cmd := drilled().Update(keyMsg("ctrl+z"))
	if cmd == nil {
		t.Fatal("ctrl+z should return a command")
	}
	if _, ok := cmd().(tea.SuspendMsg); !ok {
		t.Errorf("ctrl+z should produce a tea.SuspendMsg, got %T", cmd())
	}
}

func TestCtrlZSuspendsFromOverlay(t *testing.T) {
	// suspend must work even while an overlay (e.g. search) is open.
	m := drilled()
	m.search.active = true
	_, cmd := m.Update(keyMsg("ctrl+z"))
	if cmd == nil {
		t.Fatal("ctrl+z should return a command even with an overlay open")
	}
	if _, ok := cmd().(tea.SuspendMsg); !ok {
		t.Errorf("ctrl+z should produce a tea.SuspendMsg, got %T", cmd())
	}
}

func TestLayoutFillsWidthNoOverflow(t *testing.T) {
	for _, W := range []int{60, 80, 120, 200} {
		u, _ := drilled().Update(tea.WindowSizeMsg{Width: W, Height: 50})
		out := u.(Model).View()
		maxw := 0
		for _, ln := range strings.Split(out, "\n") {
			if w := lipgloss.Width(ln); w > maxw {
				maxw = w
			}
		}
		if maxw > W {
			t.Errorf("width=%d: a line overflows to %d", W, maxw)
		}
		// the bordered panels should reach the full width (no half-used screen)
		if maxw != W {
			t.Errorf("width=%d: layout only fills %d cols", W, maxw)
		}
	}
}

func TestHelpOverlayToggle(t *testing.T) {
	u, _ := drilled().Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m := u.(Model)
	// '?' opens help
	m, _ = mustModel(m.handleKey(keyMsg("?")))
	if !m.showHelp {
		t.Fatal("'?' should open help")
	}
	out := m.View()
	for _, want := range []string{"Help", "cycle panels", "Actions", "pause / resume"} {
		if !strings.Contains(out, want) {
			t.Errorf("help overlay missing %q\n%s", want, out)
		}
	}
	// esc closes it
	m, _ = mustModel(m.handleKey(keyMsg("esc")))
	if m.showHelp {
		t.Error("esc should close help")
	}
}

func TestRangeSpanPresets(t *testing.T) {
	m := sampleModel()
	m.today = "2026-06-01"
	// ⇧1 ("!") => single-day window ending today
	n, cmd := mustModel(m.handleKey(keyMsg("!")))
	if n.from != "2026-06-01" || n.to != "2026-06-01" {
		t.Errorf("span 1: from=%q to=%q", n.from, n.to)
	}
	if cmd == nil {
		t.Error("span change should reload")
	}
	// ⇧2 ("@") => last 7 days
	n, _ = mustModel(m.handleKey(keyMsg("@")))
	if n.from != "2026-05-26" || n.to != "2026-06-01" {
		t.Errorf("span 7: from=%q to=%q", n.from, n.to)
	}
	// a bare digit no longer sets the span — it jumps panels, leaving the
	// window exactly as it was
	n, _ = mustModel(m.handleKey(keyMsg("1")))
	if n.from != m.from || n.to != m.to {
		t.Errorf("bare digit must not change the span: from=%q to=%q (was %q..%q)", n.from, n.to, m.from, m.to)
	}
}

func TestLogKeyOpensPrompt(t *testing.T) {
	m := drilled()
	m.today = m.currentDay()
	want := m.selectedTaskID()
	m, _ = mustModel(m.handleKey(keyMsg("n")))
	if !m.confirm.active || !m.confirm.input.active {
		t.Fatalf("'n' should arm a log confirm, got %+v", m.confirm)
	}
	n, _ := mustModel(m.handleKey(keyMsg("y")))
	if !n.input.active || n.input.action != "log" {
		t.Fatalf("confirming 'n' should open a log (note) prompt, got %+v", n.input)
	}
	// the note must target the selected task (like amend/move), not the running one
	if n.input.taskID != want {
		t.Fatalf("log prompt taskID = %q, want selected task %q", n.input.taskID, want)
	}
}

func TestProjectAutocomplete(t *testing.T) {
	m := drilled()
	m.projects = []string{"backend", "backlog", "frontend"}
	m.input = inputMode{active: true, action: "move", taskID: "T1", value: "ba"}
	tab := func(mm Model) Model { n, _ := mm.handleKey(tea.KeyMsg{Type: tea.KeyTab}); return n.(Model) }
	m = tab(m)
	if m.input.value != "backend" {
		t.Fatalf("tab 1 -> %q, want backend", m.input.value)
	}
	m = tab(m)
	if m.input.value != "backlog" {
		t.Fatalf("tab 2 -> %q, want backlog", m.input.value)
	}
	m = tab(m)
	if m.input.value != "backend" {
		t.Fatalf("tab 3 should wrap -> %q, want backend", m.input.value)
	}
}

func TestRunningHeader(t *testing.T) {
	m := drilled()
	m.live = &wj.Status{Date: "2026-06-01", Now: "12:30", Tasks: []wj.Task{
		{ID: "T1", Project: "backend", Status: "in-progress", Minutes: 72, Desc: "Refactor auth"},
	}}
	m.liveAt = time.Now()
	h := m.runningHeader()
	// default (icons off) uses the ASCII running marker
	if !strings.Contains(h, "> T1") || !strings.Contains(h, "Refactor auth") {
		t.Errorf("running header = %q", h)
	}
	// with icons on, the same spot shows the play glyph
	SetIcons("on")
	if h := m.runningHeader(); !strings.Contains(h, "▶") {
		t.Errorf("icons on: running header should use ▶, got %q", h)
	}
	SetIcons("off")
	// no running task -> idle
	m.live.Tasks[0].Status = "paused"
	if h := m.runningHeader(); !strings.Contains(h, "idle") {
		t.Errorf("idle header = %q", h)
	}
}

func TestWindowRows(t *testing.T) {
	rows := make([]string, 10)
	for i := range rows {
		rows[i] = fmt.Sprintf("row%d", i)
	}
	// near the top: tail becomes a "↓ more" indicator
	top := windowRows(rows, 0, 4)
	if len(top) != 4 || top[0] != "row0" || !strings.Contains(top[3], "more") {
		t.Errorf("top window = %v", top)
	}
	// near the bottom: head becomes an "↑ more" indicator
	bot := windowRows(rows, 9, 4)
	if len(bot) != 4 || !strings.Contains(bot[0], "more") || bot[3] != "row9" {
		t.Errorf("bottom window = %v", bot)
	}
	// fits entirely: returned unchanged
	if got := windowRows(rows[:3], 0, 4); len(got) != 3 {
		t.Errorf("no-window case = %v", got)
	}
}

func TestLayoutFitsHeight(t *testing.T) {
	m := drilled()
	for i := 0; i < 30; i++ {
		m.grid.Tasks = append(m.grid.Tasks, wj.GridTask{ID: fmt.Sprintf("T%d", i+2), Project: "p", Minutes: 30})
	}
	// the 3-panel layout fits any realistic terminal (>= 24 rows); below ~21
	// rows it can't compress further, which no real terminal hits.
	for _, H := range []int{24, 30, 50, 80} {
		u, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: H})
		lines := strings.Count(u.(Model).View(), "\n") + 1
		if lines > H {
			t.Errorf("height=%d: rendered %d lines (overflow)", H, lines)
		}
	}
}

// stripANSI removes SGR escape sequences so width assertions ignore color codes.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			inEsc = true
		case inEsc && r == 'm':
			inEsc = false
		case !inEsc:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestSearchOpenAndJump(t *testing.T) {
	m := sampleModel()
	m.today = "2026-06-01"
	// "/" opens the overlay and primes an (empty-query) search
	m, cmd := mustModel(m.handleKey(keyMsg("/")))
	if !m.search.active || cmd == nil {
		t.Fatalf("'/' should open search and run a query, got active=%v cmd=%v", m.search.active, cmd)
	}
	// a result for the live query is accepted; a stale one is dropped
	stale, _ := mustModel(m.Update(searchMsg{query: "old", results: []wj.Found{{ID: "T9"}}}))
	if len(stale.search.results) != 0 {
		t.Error("a searchMsg for a stale query must be ignored")
	}
	m, _ = mustModel(m.Update(searchMsg{query: "", results: []wj.Found{
		{ID: "T2", Date: "2026-05-30", Project: "backend", Desc: "Refactor auth", Status: "completed", Minutes: 90},
	}}))
	if len(m.search.results) != 1 {
		t.Fatalf("matching searchMsg should populate results, got %d", len(m.search.results))
	}
	// Enter jumps: closes overlay, windows the range onto the hit, arms the task
	m, cmd = mustModel(m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}))
	if m.search.active {
		t.Error("enter should close the search overlay")
	}
	if m.to != "2026-05-30" || m.from != "2026-05-24" {
		t.Errorf("jump should window onto the hit: from=%q to=%q", m.from, m.to)
	}
	if m.jumpTaskID != "T2" || m.pane != paneTimeline || cmd == nil {
		t.Errorf("jump should arm T2 + drill + reload: jump=%q pane=%d cmd=%v", m.jumpTaskID, m.pane, cmd)
	}
	// when that day's grid lands, the armed task is selected and cleared
	m, _ = mustModel(m.Update(ganttMsg{g: &wj.Gantt{From: "2026-05-24", To: "2026-05-30",
		Days: []string{"2026-05-30"}, Rows: m.g.Rows}}))
	m, _ = mustModel(m.Update(gridMsg{day: "2026-05-30", g: &wj.Grid{Date: "2026-05-30",
		Tasks: []wj.GridTask{{ID: "T1", Project: "x"}, {ID: "T2", Project: "backend"}}}}))
	if m.jumpTaskID != "" {
		t.Error("jumpTaskID should be cleared after landing")
	}
	if m.selectedTaskID() != "T2" {
		t.Errorf("grid landing should select the jumped task, got %q", m.selectedTaskID())
	}
}

// When the day rolls over while the TUI is parked on what was "today", the view
// must follow forward (slide the window onto the new date and refocus it) so
// open tasks don't freeze on yesterday. A view browsing an earlier day is left
// in place, but its notion of today is still refreshed.
func TestAdvanceForDateRollover(t *testing.T) {
	// parked on today (the last day of the range) → follow forward
	m := liveModel() // today 2026-06-01; Days end 2026-06-01 (5-day span)
	m.focusInit = true
	m.focusedDay = len(m.g.Days) - 1 // the "today" column
	got, cmd := m.advanceForDate("2026-06-02")
	if got.today != "2026-06-02" {
		t.Errorf("today should refresh to 2026-06-02, got %q", got.today)
	}
	if got.to != "2026-06-02" || got.from != "2026-05-29" {
		t.Errorf("window should slide to 2026-05-29..2026-06-02, got %q..%q", got.from, got.to)
	}
	if got.focusInit {
		t.Error("focusInit should reset so the reload refocuses the new today")
	}
	if cmd == nil {
		t.Error("following a rollover should issue a gantt reload")
	}

	// browsing an earlier day → don't yank the view, but still refresh today
	m2 := liveModel()
	m2.focusInit = true
	m2.focusedDay = 0 // an older day, deliberately
	got2, cmd2 := m2.advanceForDate("2026-06-02")
	if got2.today != "2026-06-02" {
		t.Errorf("today should refresh even when not following, got %q", got2.today)
	}
	if got2.to != "2026-06-01" || cmd2 != nil {
		t.Errorf("a browsing view must keep its window (to=%q, cmd=%v)", got2.to, cmd2)
	}

	// same day → no-op
	if got3, cmd3 := liveModel().advanceForDate("2026-06-01"); cmd3 != nil || got3.to != "2026-06-01" {
		t.Errorf("same-day advance should be a no-op, got to=%q cmd=%v", got3.to, cmd3)
	}
}

// With a Today section present, jumping to a search hit must clear the project
// filter (index 0 is a Today row, not "All"), so a hit in another project is
// still found and selected rather than filtered out.
func TestSearchJumpClearsFilterWithTodaySection(t *testing.T) {
	m := liveModel() // Today section → allRow()==2, today 2026-06-01
	m.search = searchMode{active: true}
	m, _ = mustModel(m.Update(searchMsg{query: "", results: []wj.Found{
		{ID: "T7", Date: "2026-06-01", Project: "frontend", Desc: "x", Status: "completed", Minutes: 10},
	}}))
	m, _ = mustModel(m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})) // jump
	if got := m.focusedRow; got != m.allRow() {
		t.Fatalf("jump should park focus on the All row (%d), got %d", m.allRow(), got)
	}
	// the gantt reload re-anchors; an All anchor resolves back to no filter
	m, _ = mustModel(m.Update(ganttMsg{g: m.g}))
	if pf := m.projectFilter(); pf != "" {
		t.Fatalf("jump must clear the project filter, got %q", pf)
	}
	// the hit day's grid lands → the frontend task is found despite backend Today rows
	day := m.currentDay()
	m, _ = mustModel(m.Update(gridMsg{day: day, g: &wj.Grid{Date: day,
		Tasks: []wj.GridTask{{ID: "T1", Project: "backend"}, {ID: "T7", Project: "frontend"}}}}))
	if m.selectedTaskID() != "T7" {
		t.Errorf("jumped task should be selected after the filter clears, got %q", m.selectedTaskID())
	}
}

func TestSearchEscCancels(t *testing.T) {
	m := sampleModel()
	m, _ = mustModel(m.handleKey(keyMsg("/")))
	m, _ = mustModel(m.handleKey(tea.KeyMsg{Type: tea.KeyEsc}))
	if m.search.active {
		t.Error("esc should close the search overlay")
	}
}

// pendingModel returns a model focused on the backlog with a few items.
func pendingModel() Model {
	m := sampleModel()
	m.today = "2026-06-02"
	m.pane = panePending
	m.pending = []wj.Pending{
		{ID: "P1", Project: "Acme", Due: "2026-06-01", Desc: "Fix invoice"}, // overdue
		{ID: "P2", Due: "2026-06-02", Desc: "Call client"},                  // today
		{ID: "P3", Project: "Ideas", Desc: "Write blog"},                    // no due
	}
	return m
}

func TestPendingAddOpensPrompt(t *testing.T) {
	m, _ := mustModel(pendingModel().handleKey(keyMsg("a")))
	if !m.confirm.active || !m.confirm.input.active {
		t.Fatalf("'a' should arm an add confirm, got %+v", m.confirm)
	}
	m, _ = mustModel(m.handleKey(keyMsg("y")))
	if !m.input.active || m.input.action != "add" {
		t.Fatalf("confirming 'a' should open the add prompt, got %+v", m.input)
	}
	// typing + enter issues `add <desc>` (a plain mutate, no date prompt)
	for _, k := range []string{"H", "i"} {
		m, _ = mustModel(m.handleKey(keyMsg(k)))
	}
	m, cmd := mustModel(m.handleKey(tea.KeyMsg{Type: tea.KeyEnter}))
	if m.input.active || cmd == nil {
		t.Errorf("enter should close prompt and issue add, got active=%v cmd=%v", m.input.active, cmd)
	}
}

func TestPendingPromoteAndDrop(t *testing.T) {
	// Enter arms a promote confirm on the selected pending task (P1); 'y' runs it
	promo, _ := mustModel(pendingModel().handleKey(tea.KeyMsg{Type: tea.KeyEnter}))
	if !promo.confirm.active || promo.confirm.verb != "start" || !promo.confirm.raw {
		t.Fatalf("enter should arm a raw promote confirm, got %+v", promo.confirm)
	}
	if _, cmd := promo.handleKey(keyMsg("y")); cmd == nil {
		t.Error("confirming enter should issue a start (promote)")
	}
	// 'x' opens a *raw* drop confirm (no --date round-trip)
	m := pendingModel()
	m, _ = mustModel(m.handleKey(keyMsg("x")))
	if !m.confirm.active || m.confirm.verb != "drop" || !m.confirm.raw {
		t.Fatalf("'x' should arm a raw drop confirm, got %+v", m.confirm)
	}
	m, cmd := mustModel(m.handleKey(keyMsg("y")))
	if m.confirm.active || cmd == nil {
		t.Error("'y' should run the drop")
	}
}

func TestPendingDueAndReorder(t *testing.T) {
	m := pendingModel()
	// 'd' arms a confirm; 'y' opens the due prompt targeting the selected id
	dc, _ := mustModel(m.handleKey(keyMsg("d")))
	if !dc.confirm.active || !dc.confirm.input.active {
		t.Fatalf("'d' should arm a due confirm, got %+v", dc.confirm)
	}
	d, _ := mustModel(dc.handleKey(keyMsg("y")))
	if !d.input.active || d.input.action != "pdue" || d.input.taskID != "P1" {
		t.Fatalf("confirming 'd' should open a due prompt for P1, got %+v", d.input)
	}
	// ']' lowers and follows the item
	m.selPend = 0
	low, cmd := mustModel(m.handleKey(keyMsg("]")))
	if cmd == nil || low.selPend != 1 {
		t.Errorf("']' should lower P1 and follow it: sel=%d cmd=%v", low.selPend, cmd)
	}
}

func TestDueBadge(t *testing.T) {
	m := Model{today: "2026-06-02"}
	cases := []struct {
		due, wantGlyph, wantLabel string
	}{
		{"", " ", "—"},
		{"2026-06-01", "!", "-1d"}, // overdue
		{"2026-06-02", "!", "today"},
		{"2026-06-03", "!", "1d"},    // due soon
		{"2026-06-20", " ", "06-20"}, // far out -> plain date
	}
	for _, c := range cases {
		g, _, label := m.dueBadge(c.due)
		if g != c.wantGlyph || label != c.wantLabel {
			t.Errorf("dueBadge(%q) = (%q,%q), want (%q,%q)", c.due, g, label, c.wantGlyph, c.wantLabel)
		}
	}
}

func TestParsePendingInput(t *testing.T) {
	cases := []struct{ in, d, p, due string }{
		{"Fix invoice", "Fix invoice", "", ""},
		{"Fix invoice @acme !2026-06-10", "Fix invoice", "acme", "2026-06-10"},
		{"!2026-06-10 Call client @ventes", "Call client", "ventes", "2026-06-10"},
		{"   spaced   out  ", "spaced out", "", ""},
	}
	for _, c := range cases {
		d, p, due := parsePendingInput(c.in)
		if d != c.d || p != c.p || due != c.due {
			t.Errorf("parse(%q) = (%q,%q,%q), want (%q,%q,%q)", c.in, d, p, due, c.d, c.p, c.due)
		}
	}
}

func TestParseStartInput(t *testing.T) {
	cases := []struct{ in, d, p, at string }{
		{"Refactor auth", "Refactor auth", "", ""},
		{"Refactor auth @backend", "Refactor auth", "backend", ""},
		{"@backend Refactor auth", "Refactor auth", "backend", ""},
		{"Fix the bug! now", "Fix the bug! now", "", ""},  // a trailing-! word stays in the desc
		{"Ship v2 @a @backend", "Ship v2", "backend", ""}, // last @token wins
		{"   spaced   out  ", "spaced out", "", ""},
		{"Refactor auth %9:30", "Refactor auth", "", "9:30"}, // inline start time
		{"Fix login @backend %9pm", "Fix login", "backend", "9pm"},
		{"Deploy %9 %10:15", "Deploy", "", "10:15"},    // last %token wins
		{"50% done report", "50% done report", "", ""}, // a mid-word % stays in the desc
	}
	for _, c := range cases {
		d, p, at := parseStartInput(c.in)
		if d != c.d || p != c.p || at != c.at {
			t.Errorf("parseStartInput(%q) = (%q,%q,%q), want (%q,%q,%q)", c.in, d, p, at, c.d, c.p, c.at)
		}
	}
}

func TestLeadingTaskID(t *testing.T) {
	cases := map[string]string{
		"T3  09:30  [proj]  started T3 — desc": "T3",
		"T12 10:00 [a] started T12 — x":        "T12",
		"T1  already completed":                "T1",
		"":                                     "",
		"started a thing":                      "", // no leading id
		"Tx not a number":                      "",
		"committed locally (no remote)":        "",
	}
	for in, want := range cases {
		if got := leadingTaskID(in); got != want {
			t.Errorf("leadingTaskID(%q) = %q, want %q", in, got, want)
		}
	}
}

// A start armed with focusNewTask must turn the CLI's confirmation line into a
// jumpTaskID, then select that task once the reloaded grid lands.
func TestStartFocusesNewTask(t *testing.T) {
	m := sampleModel()
	m.focusNewTask = true
	m, _ = mustModel(m.Update(mutationMsg{note: "T2  09:30  [backend]  started T2 — new work"}))
	if m.jumpTaskID != "T2" {
		t.Fatalf("a start note should arm jumpTaskID=T2, got %q", m.jumpTaskID)
	}
	if m.focusNewTask {
		t.Error("focusNewTask must be cleared after the mutation is handled")
	}
	// the freshly reloaded day's grid lands → the new task becomes the selection
	day := m.currentDay()
	m, _ = mustModel(m.Update(gridMsg{day: day, g: &wj.Grid{Date: day,
		Tasks: []wj.GridTask{{ID: "T1", Project: "backend"}, {ID: "T2", Project: "backend"}}}}))
	if m.jumpTaskID != "" {
		t.Error("jumpTaskID should clear after landing")
	}
	if m.selectedTaskID() != "T2" {
		t.Errorf("grid landing should select the new task, got %q", m.selectedTaskID())
	}
}

// A mutation that wasn't armed (focusNewTask false) must not hijack the
// selection, even if its note happens to start with a task id.
func TestUnarmedMutationDoesNotJump(t *testing.T) {
	m := sampleModel()
	m, _ = mustModel(m.Update(mutationMsg{note: "T1  09:30  paused"}))
	if m.jumpTaskID != "" {
		t.Errorf("an unarmed mutation must not arm a jump, got %q", m.jumpTaskID)
	}
}

func TestLayoutNeverOverflowsShortTerminal(t *testing.T) {
	m := drilled()
	for i := 0; i < 30; i++ {
		m.grid.Tasks = append(m.grid.Tasks, wj.GridTask{ID: fmt.Sprintf("T%d", i+2), Project: "p", Minutes: 30})
	}
	// even pathologically short/narrow terminals must not render past the screen
	for _, H := range []int{6, 8, 10, 14, 18, 22} {
		for _, W := range []int{40, 60, 100} {
			u, _ := m.Update(tea.WindowSizeMsg{Width: W, Height: H})
			out := u.(Model).View()
			lines := strings.Count(out, "\n") + 1
			if lines > H {
				t.Errorf("W=%d H=%d: rendered %d lines (overflow)", W, H, lines)
			}
			maxw := 0
			for _, ln := range strings.Split(out, "\n") {
				if x := lipgloss.Width(ln); x > maxw {
					maxw = x
				}
			}
			if maxw > W {
				t.Errorf("W=%d H=%d: a line is %d cols wide (overflow)", W, H, maxw)
			}
		}
	}
}

func TestFocusedRowFollowsProjectAcrossReload(t *testing.T) {
	m := sampleModel() // rows: backend, meetings
	m.focusedRow = 2   // "meetings" (index 0 = All, 1 = backend, 2 = meetings)
	if m.projectFilter() != "meetings" {
		t.Fatalf("setup: filter = %q, want meetings", m.projectFilter())
	}
	// reload with the rows in a different order; selection must follow by name
	reordered := &wj.Gantt{From: m.from, To: m.to, Days: m.g.Days, Rows: []wj.GanttRow{
		{Key: "meetings", Label: "meetings", Project: "meetings", PerDay: map[string]int{}},
		{Key: "backend", Label: "backend", Project: "backend", PerDay: map[string]int{}},
	}}
	u, _ := m.Update(ganttMsg{g: reordered})
	if got := u.(Model).projectFilter(); got != "meetings" {
		t.Errorf("after reorder, filter = %q, want meetings (followed by identity)", got)
	}
}

func TestLayoutSplit(t *testing.T) {
	sum := func(s [3]int) int { return s[0] + s[1] + s[2] }
	// split() drives the two-column topology's vertical division; it must sum and
	// keep the focused panel largest for every two-column profile.
	for _, lp := range layouts {
		if lp.kind != topoTwoCol {
			continue
		}
		got := lp.split(100, 1)
		if sum(got) != 100 {
			t.Errorf("%s: split sum = %d, want 100 (%v)", lp.name, sum(got), got)
		}
		if got[1] < got[0] || got[1] < got[2] {
			t.Errorf("%s: focused panel (idx1) should be the largest: %v", lp.name, got)
		}
	}
	// a too-short column or no-focus sidebar falls back to thirds, still summing
	if s := layouts[0].split(9, 1); sum(s) != 9 {
		t.Errorf("short split sum = %d (%v)", sum(s), s)
	}
	if s := layouts[0].sidebarSplit(30, -1); s != [3]int{10, 10, 10} {
		t.Errorf("sidebar with no focus should be equal thirds, got %v", s)
	}
}

// splitN/splitW must partition a total exactly (no under/over-fill), which is
// what keeps every multi-panel topology filling the body without overflow.
func TestSplitHelpers(t *testing.T) {
	tot := func(xs []int) int {
		s := 0
		for _, x := range xs {
			s += x
		}
		return s
	}
	for _, total := range []int{0, 1, 7, 50, 99, 100, 201} {
		for _, n := range []int{1, 2, 3, 4} {
			if got := splitN(total, n); tot(got) != total || len(got) != n {
				t.Errorf("splitN(%d,%d) = %v (sum %d)", total, n, got, tot(got))
			}
		}
		if got := splitW(total, 3, 2); tot(got) != total {
			t.Errorf("splitW(%d,3,2) = %v (sum %d, want %d)", total, got, tot(got), total)
		}
		if got := splitW(total, 0, 0); tot(got) != total { // zero weights → equal parts
			t.Errorf("splitW(%d,0,0) = %v (sum %d)", total, got, tot(got))
		}
	}
}

// Every topology must fill the terminal width exactly (no overflow, no gap) and
// keep all six panels reachable: at a generous size each layout renders every
// panel title. This is the structural guarantee behind "drastically different
// yet still shows all the information."
func TestEveryLayoutRenders(t *testing.T) {
	titles := []string{"Projects", "Tasks", "Pending", "Range", "Day", "Timeline"}
	for i, lp := range layouts {
		// size comfortably above every topology's minSize so none falls back
		const W, H = 140, 48
		m := drilled()
		u, _ := m.Update(tea.WindowSizeMsg{Width: W, Height: H})
		mm := u.(Model)
		mm.layout = i
		if mm.activeLayout().name != lp.name {
			t.Fatalf("%s: fell back to %s at %dx%d", lp.name, mm.activeLayout().name, W, H)
		}
		out := mm.View()
		maxw := 0
		for _, ln := range strings.Split(out, "\n") {
			if w := lipgloss.Width(ln); w > maxw {
				maxw = w
			}
		}
		if maxw != W {
			t.Errorf("%s: fills %d cols, want exactly %d", lp.name, maxw, W)
		}
		// the rail intentionally foregrounds one panel (others reachable via focus),
		// so it need not show Range/Day titles at once; the rest show all six.
		want := titles
		if lp.kind == topoRail {
			want = []string{"Projects", "Tasks", "Pending", "Timeline"}
		}
		for _, w := range want {
			if !strings.Contains(out, w) {
				t.Errorf("%s: view missing %q panel", lp.name, w)
			}
		}
	}
}

// No layout may overflow the terminal — in width (a line wider than W) or height
// (more rows than H) — across a sweep of sizes and both sidebar sides, including
// tiny terminals that trip the fallback and the stackBig/dashboard min-band
// clamps. This guards the integer split arithmetic in every topology.
func TestEveryLayoutNoOverflow(t *testing.T) {
	defer func() { sidebarRight = false }()
	sizes := []struct{ W, H int }{
		{40, 12}, {52, 16}, {64, 18}, {80, 22}, {104, 22}, {120, 30}, {160, 50}, {200, 60},
	}
	for _, right := range []bool{false, true} {
		sidebarRight = right
		for i, lp := range layouts {
			for _, s := range sizes {
				m := drilled()
				u, _ := m.Update(tea.WindowSizeMsg{Width: s.W, Height: s.H})
				mm := u.(Model)
				mm.layout = i
				// exercise every focus state — the rail's big area and companion
				// split depend on m.pane.
				for p := pane(0); p < paneCount; p++ {
					mm.pane = p
					out := mm.View()
					lines := strings.Split(out, "\n")
					if len(lines) > s.H {
						t.Errorf("%s right=%v %dx%d pane=%d: %d rows > %d", lp.name, right, s.W, s.H, p, len(lines), s.H)
					}
					for _, ln := range lines {
						if w := lipgloss.Width(ln); w > s.W {
							t.Errorf("%s right=%v %dx%d pane=%d: line overflows to %d", lp.name, right, s.W, s.H, p, w)
						}
					}
				}
			}
		}
	}
}

// sidebar=right must fill the width exactly in every topology (the lists column
// moves but nothing under/over-fills).
func TestEveryLayoutFillsWidthSidebarRight(t *testing.T) {
	defer func() { sidebarRight = false }()
	sidebarRight = true
	const W, H = 140, 48
	for i, lp := range layouts {
		m := drilled()
		u, _ := m.Update(tea.WindowSizeMsg{Width: W, Height: H})
		mm := u.(Model)
		mm.layout = i
		maxw := 0
		for _, ln := range strings.Split(mm.View(), "\n") {
			if w := lipgloss.Width(ln); w > maxw {
				maxw = w
			}
		}
		if maxw != W {
			t.Errorf("%s (sidebar=right): fills %d cols, want exactly %d", lp.name, maxw, W)
		}
	}
}

// collapseEmptyPending shrinks the Pending slot to the slim strip and hands the
// reclaimed rows to its stack-mates, always preserving the total.
func TestCollapseEmptyPending(t *testing.T) {
	sum := func(xs []int) int {
		s := 0
		for _, x := range xs {
			s += x
		}
		return s
	}
	// Pending (idx 2) collapses to 3; the other two absorb the reclaimed 17 rows.
	got := collapseEmptyPending([]int{10, 10, 20}, 2)
	if got[2] != 3 {
		t.Errorf("pending should collapse to 3, got %d (%v)", got[2], got)
	}
	if sum(got) != 40 {
		t.Errorf("sum must be preserved: got %d (%v)", sum(got), got)
	}
	if got[0] <= 10 || got[1] <= 10 {
		t.Errorf("stack-mates should grow: %v", got)
	}
	// last-position Pending (the rail) and already-slim panels are handled too.
	if r := collapseEmptyPending([]int{8, 8, 6, 9}, 3); r[3] != 3 || sum(r) != 31 {
		t.Errorf("rail-position collapse: %v (sum %d)", r, sum(r))
	}
	if r := collapseEmptyPending([]int{10, 3}, 1); r[1] != 3 || r[0] != 10 {
		t.Errorf("already-slim pending is left alone: %v", r)
	}
	// out-of-range / degenerate indices are no-ops, not panics.
	if r := collapseEmptyPending([]int{5, 5}, -1); r[0] != 5 || r[1] != 5 {
		t.Errorf("bad index should be a no-op: %v", r)
	}
}

// With an empty backlog, every layout collapses the Pending panel: its title
// still shows (reachable), but it occupies far fewer rows than when populated,
// so the freed space goes to the other panels.
func TestEmptyPendingCollapsesEveryLayout(t *testing.T) {
	const W, H = 130, 44
	// rows of the rendered Pending panel: its title line plus the body lines up to
	// the next panel border. A crude but stable proxy for how tall it is drawn.
	pendingRows := func(out string) int {
		lines := strings.Split(out, "\n")
		start := -1
		for i, ln := range lines {
			if strings.Contains(ln, "Pending") {
				start = i
				break
			}
		}
		if start < 0 {
			return 0
		}
		n := 1
		for i := start + 1; i < len(lines); i++ {
			// stop at the panel's bottom border (── run) on the Pending column
			if strings.Contains(lines[i], "╰") || strings.Contains(lines[i], "╭") {
				break
			}
			n++
		}
		return n
	}
	for i, lp := range layouts {
		full := drilled()
		full.pending = []wj.Pending{ // a populated backlog
			{ID: "P1", Project: "Acme", Due: "2026-06-01", Desc: "Fix invoice"},
			{ID: "P2", Due: "2026-06-02", Desc: "Call client"},
			{ID: "P3", Project: "Ideas", Desc: "Write blog"},
		}
		empty := drilled() // drilled() has an empty backlog
		uf, _ := full.Update(tea.WindowSizeMsg{Width: W, Height: H})
		ue, _ := empty.Update(tea.WindowSizeMsg{Width: W, Height: H})
		mf, me := uf.(Model), ue.(Model)
		mf.layout, me.layout = i, i
		of, oe := mf.View(), me.View()
		if !strings.Contains(oe, "Pending") {
			t.Errorf("%s: collapsed Pending must still show its title", lp.name)
		}
		if pendingRows(oe) >= pendingRows(of) {
			t.Errorf("%s: empty Pending (%d rows) should be slimmer than populated (%d rows)",
				lp.name, pendingRows(oe), pendingRows(of))
		}
	}
}

// Below its minSize a topology falls back to balanced so no panel gets crushed;
// balanced itself is always honored.
func TestLayoutFallsBackWhenTiny(t *testing.T) {
	m := sampleModel()
	for i, lp := range layouts {
		m.layout = i
		m.width, m.height = 40, 12 // smaller than any topology's minSize
		if got := m.activeLayout().name; got != "balanced" {
			t.Errorf("%s at 40x12 should fall back to balanced, got %s", lp.name, got)
		}
	}
	m.layout = 0
	m.width, m.height = 40, 12
	if got := m.activeLayout().name; got != "balanced" {
		t.Errorf("balanced must always be honored, got %s", got)
	}
}

// The descriptive aliases resolve to their back-compat preset names.
func TestLayoutAliases(t *testing.T) {
	if layoutIndex("rail") != layoutIndex("spotlight") {
		t.Error("rail should alias spotlight")
	}
	if layoutIndex("dashboard") != layoutIndex("golden") {
		t.Error("dashboard should alias golden")
	}
}

func TestLayoutCycle(t *testing.T) {
	m := sampleModel()
	m.layout = 0
	n, _ := m.handleKey(keyMsg("L"))
	nm := n.(Model)
	if nm.layout != 1 {
		t.Fatalf("Shift+L should advance layout 0->1, got %d", nm.layout)
	}
	if !strings.Contains(nm.notice, layouts[1].name) {
		t.Errorf("cycling should announce the layout, notice = %q", nm.notice)
	}
	nm.layout = len(layouts) - 1 // wraps back to 0
	if n2, _ := nm.handleKey(keyMsg("L")); n2.(Model).layout != 0 {
		t.Errorf("Shift+L should wrap to 0, got %d", n2.(Model).layout)
	}
}

func TestSetLayout(t *testing.T) {
	orig := defaultLayout
	defer func() { defaultLayout = orig }()

	SetLayout("spotlight")
	want := layoutIndex("spotlight")
	if defaultLayout != want {
		t.Fatalf("SetLayout(spotlight): defaultLayout = %d, want %d", defaultLayout, want)
	}
	SetLayout("nonsense") // unknown name is a no-op
	SetLayout("")         // empty is a no-op
	if defaultLayout != want {
		t.Errorf("unknown/empty SetLayout should not change defaultLayout (= %d)", defaultLayout)
	}
	if got := New(wj.Client{}, "", "", "", "", 0).layout; got != want {
		t.Errorf("New should adopt defaultLayout, got %d want %d", got, want)
	}
}

func TestSetSidebar(t *testing.T) {
	orig := sidebarRight
	defer func() { sidebarRight = orig }()
	sidebarRight = false
	SetSidebar("left")
	if sidebarRight {
		t.Error("'left' should keep the sidebar on the left")
	}
	SetSidebar("")
	if sidebarRight {
		t.Error("empty is a no-op")
	}
	SetSidebar("right")
	if !sidebarRight {
		t.Error("'right' should move the sidebar to the right")
	}
}

func TestSetLayoutRatiosCustom(t *testing.T) {
	orig := layouts
	defer func() { layouts = orig }()
	SetLayoutRatios("28", "60,25,15")
	i := layoutIndex("custom")
	if i < 0 {
		t.Fatal("a custom layout should be registered from the ratios")
	}
	lp := layouts[i]
	if lp.sidePct != 28 || lp.focusNum != 60 || lp.focusDen != 100 || lp.restHi != 25 || lp.restLo != 15 {
		t.Errorf("custom profile parsed wrong: %+v", lp)
	}
	if s := lp.split(100, 0); s[0]+s[1]+s[2] != 100 {
		t.Errorf("custom split should sum to 100, got %v", s)
	}
}

func TestSpotlightMinHeight(t *testing.T) {
	// spotlight on a shortish column would give thin strips; the floor keeps the
	// non-focused panels at >= 4 rows, borrowing from the focused one.
	sp := layouts[1].split(20, 1) // spotlight, focus index 1
	if sp[0] < 4 || sp[2] < 4 {
		t.Errorf("non-focused panels should be >= 4 rows: %v", sp)
	}
	if sp[0]+sp[1]+sp[2] != 20 {
		t.Errorf("split should still sum to 20: %v", sp)
	}
	if sp[1] < sp[0] || sp[1] < sp[2] {
		t.Errorf("focused panel should still be the largest: %v", sp)
	}
}

func TestActiveLayoutFallback(t *testing.T) {
	m := sampleModel()
	m.layout = layoutIndex("spotlight")
	m.width, m.height = 100, 40 // roomy: keep the chosen layout
	if got := m.activeLayout().name; got != "spotlight" {
		t.Errorf("roomy terminal should keep spotlight, got %s", got)
	}
	m.height = 14 // too short: fall back to balanced so nothing is crushed
	if got := m.activeLayout().name; got != "balanced" {
		t.Errorf("short terminal should fall back to balanced, got %s", got)
	}
	m.layout = 0 // balanced never falls back
	m.height = 5
	if got := m.activeLayout().name; got != "balanced" {
		t.Errorf("balanced should stay balanced, got %s", got)
	}
}

func TestZoomToggle(t *testing.T) {
	m := sampleModel()
	if m.zoomed {
		t.Fatal("zoom should be off by default")
	}
	n, _ := m.handleKey(keyMsg("z"))
	if m = n.(Model); !m.zoomed {
		t.Fatal("z should enter zoom")
	}
	n, _ = m.handleKey(keyMsg("z"))
	if m = n.(Model); m.zoomed {
		t.Fatal("z should exit zoom")
	}
	// esc leaves zoom first, without also resetting the pane focus
	m.zoomed = true
	m.pane = paneTimeline
	n, _ = m.handleKey(keyMsg("esc"))
	m = n.(Model)
	if m.zoomed {
		t.Error("esc should exit zoom")
	}
	if m.pane != paneTimeline {
		t.Error("the esc that exits zoom should not also reset the pane")
	}
}

func TestZoomRendersSinglePanel(t *testing.T) {
	m := drilled() // pane = paneDay, has grid data
	m.ready, m.width, m.height = true, 80, 24
	m.zoomed = true
	out := m.View()
	if !strings.Contains(out, "Day — ") {
		t.Errorf("zoom on paneDay should show the Day panel:\n%s", out)
	}
	if strings.Contains(out, "Projects") {
		t.Errorf("zoom should hide the other panels (found Projects):\n%s", out)
	}
}

func TestSidebarSideRender(t *testing.T) {
	orig := sidebarRight
	defer func() { sidebarRight = orig }()
	m := sampleModel()
	m.ready, m.width, m.height = true, 100, 24
	titleRow := func(s string) string {
		for _, ln := range strings.Split(s, "\n") {
			if strings.Contains(ln, "Projects") && strings.Contains(ln, "Range") {
				return ln
			}
		}
		return ""
	}
	sidebarRight = false
	left := titleRow(m.View())
	sidebarRight = true
	right := titleRow(m.View())
	if left == "" || right == "" {
		t.Fatal("could not find the Projects/Range title row")
	}
	if strings.Index(left, "Projects") > strings.Index(left, "Range") {
		t.Errorf("sidebar=left: Projects should come before Range")
	}
	if strings.Index(right, "Range") > strings.Index(right, "Projects") {
		t.Errorf("sidebar=right: Range should come before Projects")
	}
}

// liveModel is a sampleModel with today's live status populated, so the Projects
// panel shows both the Today and Window sections. today (2026-06-01) is the last
// day of the gantt range, so a Today selection can jump to it.
func liveModel() Model {
	m := sampleModel()
	m.today = "2026-06-01"
	m.live = &wj.Status{Date: "2026-06-01", Tasks: []wj.Task{
		{ID: "T1", Project: "backend", Status: "in-progress", Minutes: 120},
		{ID: "T2", Project: "frontend", Status: "completed", Minutes: 45},
		{ID: "T3", Project: "backend", Status: "paused", Minutes: 30},
	}}
	return m
}

func TestProjectsTwoSections(t *testing.T) {
	m := liveModel()
	rows := m.projRows()
	// Today section (aggregated by project, first-seen order) leads, then the
	// Window section ("All" + gantt rows).
	if len(rows) != 5 {
		t.Fatalf("projRows = %d rows, want 5: %+v", len(rows), rows)
	}
	if !rows[0].today || rows[0].project != "backend" || rows[0].minutes != 150 {
		t.Errorf("row0 = %+v, want today backend 150 (120+30)", rows[0])
	}
	if !rows[0].running {
		t.Error("backend has the in-progress task → should be flagged running")
	}
	if !rows[1].today || rows[1].project != "frontend" || rows[1].minutes != 45 {
		t.Errorf("row1 = %+v, want today frontend 45", rows[1])
	}
	if rows[2].today || !rows[2].isAll {
		t.Errorf("row2 = %+v, want the Window All entry", rows[2])
	}
	if m.allRow() != 2 {
		t.Errorf("allRow = %d, want 2", m.allRow())
	}
	// both section headers render
	out := m.renderProjects(30, 100)
	for _, want := range []string{"Today", "Window"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderProjects missing %q header:\n%s", want, out)
		}
	}
}

func TestNoLiveNoTodaySection(t *testing.T) {
	// With no live status, the panel collapses to the Window list and shows no
	// section headers (unchanged from the single-section behavior).
	out := sampleModel().renderProjects(30, 100)
	if strings.Contains(out, "Today") {
		t.Errorf("no live status should mean no Today header:\n%s", out)
	}
}

func TestProjectsEmptyState(t *testing.T) {
	// no live status and no range rows → the panel shows the empty-state message,
	// not a stray synthetic "All" row.
	m := sampleModel()
	m.g.Rows = nil
	if out := m.renderProjects(30, 100); !strings.Contains(out, "no tracked time") {
		t.Errorf("empty dataset should render the no-tracked-time message:\n%s", out)
	}
	// but with today work present, the panel still renders even when the range is empty.
	m2 := liveModel()
	m2.g.Rows = nil
	if out := m2.renderProjects(30, 100); strings.Contains(out, "no tracked time") {
		t.Errorf("today work should render rows even with an empty range:\n%s", out)
	}
}

func TestProjectFilterTodayRow(t *testing.T) {
	m := liveModel()
	m.focusedRow = 1 // today frontend
	if !m.selectedToday() {
		t.Error("row 1 should be in the Today section")
	}
	if got := m.projectFilter(); got != "frontend" {
		t.Errorf("projectFilter = %q, want frontend", got)
	}
}

func TestSectionToggleKey(t *testing.T) {
	// T from a Today row jumps to the Window "All" entry…
	m := liveModel()
	m.focusedRow = 0 // today backend
	u, _ := m.handleKey(keyMsg("T"))
	if got := u.(Model).focusedRow; got != m.allRow() {
		t.Errorf("T from Today should land on All (%d), got %d", m.allRow(), got)
	}
	// …and from a Window row back to the first Today row.
	m2 := liveModel()
	m2.focusedRow = 3 // window backend
	u2, _ := m2.handleKey(keyMsg("T"))
	if got := u2.(Model).focusedRow; got != 0 {
		t.Errorf("T from Window should land on the first Today row (0), got %d", got)
	}
}

func TestTodaySelectionJumpsToToday(t *testing.T) {
	m := liveModel()
	m.focusedDay = 0          // 2026-05-28, a past day
	m.focusedRow = m.allRow() // start on the Window All entry (index 2)
	// k moves up into the Today section, which should jump the day view to today.
	u, _ := m.handleKey(keyMsg("k"))
	nm := u.(Model)
	if !nm.selectedToday() {
		t.Fatalf("k should move into the Today section, focusedRow = %d", nm.focusedRow)
	}
	if nm.focusedDay != 4 { // index of 2026-06-01 (today) in g.Days
		t.Errorf("selecting a Today row should jump focusedDay to today (4), got %d", nm.focusedDay)
	}
}

func TestWindowSelectionSurvivesLiveReload(t *testing.T) {
	m := liveModel()
	m.focusedRow = 4 // window meetings
	if m.projectFilter() != "meetings" {
		t.Fatalf("setup: filter = %q, want meetings", m.projectFilter())
	}
	// a live refresh that grows the Today section must not shift the window
	// selection (it is re-anchored by identity, not raw index).
	u, _ := m.Update(liveMsg{s: &wj.Status{Date: "2026-06-01", Tasks: []wj.Task{
		{ID: "T9", Project: "newproj", Status: "in-progress", Minutes: 10},
		{ID: "T1", Project: "backend", Minutes: 150},
		{ID: "T2", Project: "frontend", Minutes: 45},
	}}})
	if got := u.(Model).projectFilter(); got != "meetings" {
		t.Errorf("window selection should survive a live reload, got %q", got)
	}
}

func TestParseConfirmLevel(t *testing.T) {
	cases := map[string]confirmLevel{
		"all": confirmAll, "destructive": confirmDestructive, "off": confirmOff,
		"none": confirmOff, "": confirmDestructive, "bogus": confirmDestructive,
	}
	for in, want := range cases {
		if got := parseConfirmLevel(in); got != want {
			t.Errorf("parseConfirmLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestConfirmLevels(t *testing.T) {
	// off: nothing confirms — even the destructive cancel runs straight away
	off := drilled()
	off.confirmLevel = confirmOff
	off.today = off.currentDay() // so issueMutation runs immediately
	if u, cmd := mustModel(off.handleKey(keyMsg("p"))); u.confirm.active || cmd == nil {
		t.Errorf("off: 'p' should run immediately, confirm=%v cmd=%v", u.confirm.active, cmd)
	}
	if u, cmd := mustModel(off.handleKey(keyMsg("x"))); u.confirm.active || cmd == nil {
		t.Errorf("off: 'x' (cancel) should run immediately, confirm=%v cmd=%v", u.confirm.active, cmd)
	}

	// destructive: only the void/drop actions confirm; pause runs immediately
	des := drilled()
	des.confirmLevel = confirmDestructive
	des.today = des.currentDay()
	if u, cmd := mustModel(des.handleKey(keyMsg("p"))); u.confirm.active || cmd == nil {
		t.Errorf("destructive: 'p' should not confirm")
	}
	if u, _ := mustModel(des.handleKey(keyMsg("x"))); !u.confirm.active || u.confirm.verb != "cancel" {
		t.Errorf("destructive: 'x' should arm a cancel confirm")
	}

	// all: even pause confirms
	all := drilled()
	all.confirmLevel = confirmAll
	if u, _ := mustModel(all.handleKey(keyMsg("p"))); !u.confirm.active || u.confirm.verb != "pause" {
		t.Errorf("all: 'p' should arm a pause confirm")
	}
}

func TestProjectsSectionSubtotals(t *testing.T) {
	// today total = backend 150 + frontend 45 = 195m = 3h15m, shown in the header
	out := liveModel().renderProjects(40, 100)
	if !strings.Contains(out, "3h15m") {
		t.Errorf("Today header should show its subtotal 3h15m:\n%s", out)
	}
}

func TestTodayLiveCounter(t *testing.T) {
	m := liveModel()
	m.liveAt = time.Now().Add(-3 * time.Minute) // status fetched 3m ago
	var backend int
	for _, r := range m.todayRows() {
		if r.project == "backend" {
			backend = r.minutes
		}
	}
	// backend has the in-progress task (T1), so it counts up past the stored 150
	if backend < 152 {
		t.Errorf("running project should count up since liveAt: backend = %d, want >= ~153", backend)
	}
}

func TestUndoKey(t *testing.T) {
	if _, cmd := drilled().handleKey(keyMsg("u")); cmd == nil {
		t.Error("'u' should issue an undo command on the focused day")
	}
}

func TestRuneDeleteWordBefore(t *testing.T) {
	cases := []struct {
		s       string
		i       int
		wantS   string
		wantCur int
	}{
		{"hello world", 11, "hello ", 6},  // delete the last word
		{"hello world", 6, "world", 0},    // cursor after "hello " eats the space + "hello"
		{"foo bar baz", 7, "foo  baz", 4}, // delete a middle word, keep the tail
		{"one", 3, "", 0},                 // single word -> empty
		{"", 0, "", 0},                    // empty is a no-op
		{"trailing   ", 11, "", 0},        // eats whitespace then the word
	}
	for _, c := range cases {
		gotS, gotCur := runeDeleteWordBefore(c.s, c.i)
		if gotS != c.wantS || gotCur != c.wantCur {
			t.Errorf("runeDeleteWordBefore(%q,%d) = (%q,%d), want (%q,%d)",
				c.s, c.i, gotS, gotCur, c.wantS, c.wantCur)
		}
	}
}

func TestRuneWordMotion(t *testing.T) {
	left := []struct {
		s    string
		i    int
		want int
	}{
		{"hello world", 11, 6}, // from end -> start of "world"
		{"hello world", 6, 0},  // from start of "world" -> start of "hello"
		{"foo  bar", 8, 5},     // skip the run of spaces, land on "bar"
		{"one", 0, 0},          // already at start
		{"", 0, 0},
	}
	for _, c := range left {
		if got := runeWordLeft(c.s, c.i); got != c.want {
			t.Errorf("runeWordLeft(%q,%d) = %d, want %d", c.s, c.i, got, c.want)
		}
	}
	right := []struct {
		s    string
		i    int
		want int
	}{
		{"hello world", 0, 5},  // end of "hello"
		{"hello world", 5, 11}, // skip space, end of "world"
		{"foo  bar", 0, 3},     // end of "foo"
		{"one", 3, 3},          // already at end
		{"", 0, 0},
	}
	for _, c := range right {
		if got := runeWordRight(c.s, c.i); got != c.want {
			t.Errorf("runeWordRight(%q,%d) = %d, want %d", c.s, c.i, got, c.want)
		}
	}
}

func TestSmartInsertPairing(t *testing.T) {
	cases := []struct {
		name    string
		s       string
		i       int
		c       rune
		wantS   string
		wantCur int
	}{
		{"open bracket pairs", "", 0, '[', "[]", 1},
		{"open brace pairs", "", 0, '{', "{}", 1},
		{"quote pairs at boundary", "say ", 4, '"', `say ""`, 5},
		{"step over closer", "[]", 1, ']', "[]", 2},
		{"step over quote", `""`, 1, '"', `""`, 2},
		{"no pair before word char", "abc", 0, '(', "(abc", 1}, // would split a word -> plain insert
		{"quote not doubled in contraction", "don", 3, '\'', "don'", 4},
		{"plain char inserts", "ab", 1, 'x', "axb", 2},
	}
	for _, c := range cases {
		gotS, gotCur := smartInsert(c.s, c.i, c.c)
		if gotS != c.wantS || gotCur != c.wantCur {
			t.Errorf("%s: smartInsert(%q,%d,%q) = (%q,%d), want (%q,%d)",
				c.name, c.s, c.i, c.c, gotS, gotCur, c.wantS, c.wantCur)
		}
	}
}

func TestSmartDeleteBeforePair(t *testing.T) {
	cases := []struct {
		s       string
		i       int
		wantS   string
		wantCur int
	}{
		{"()", 1, "", 0},    // inside an empty pair -> delete both
		{`""`, 1, "", 0},    // empty quotes too
		{"(x)", 1, "x)", 0}, // non-empty pair -> only the opener goes
		{"ab", 2, "a", 1},   // ordinary backspace
		{"[x", 1, "x", 0},   // opener whose next char isn't its closer -> plain delete
	}
	for _, c := range cases {
		gotS, gotCur := smartDeleteBefore(c.s, c.i)
		if gotS != c.wantS || gotCur != c.wantCur {
			t.Errorf("smartDeleteBefore(%q,%d) = (%q,%d), want (%q,%d)",
				c.s, c.i, gotS, gotCur, c.wantS, c.wantCur)
		}
	}
}

func TestCtrlWDeletesWordInPrompt(t *testing.T) {
	m := sampleModel()
	m.input = inputMode{active: true, action: "start", value: "fix the bug", cursor: 11}
	u, _ := mustModel(m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlW}))
	if u.input.value != "fix the " || u.input.cursor != 8 {
		t.Errorf("Ctrl+W = (%q,%d), want (%q,8)", u.input.value, u.input.cursor, "fix the ")
	}
	// Ctrl+Backspace arrives as Ctrl+H on most terminals
	u2, _ := mustModel(u.handleKey(tea.KeyMsg{Type: tea.KeyCtrlH}))
	if u2.input.value != "fix " {
		t.Errorf("Ctrl+H = %q, want %q", u2.input.value, "fix ")
	}
}

func TestHelpOverlayDocumentsNewActions(t *testing.T) {
	out := sampleModel().helpOverlay()
	for _, want := range []string{
		"Today and Window", // T section toggle
		"undo the last",    // u undo
		"confirm`",         // confirm config note (rendered as `confirm`)
		"Ctrl+W",           // word delete in prompts
		"deletes a word",
		"Text prompts", // the editing section header
	} {
		if !strings.Contains(out, want) {
			t.Errorf("? help overlay missing %q", want)
		}
	}
}

func TestPendingDetailShowsFullText(t *testing.T) {
	m := pendingModel()
	m.pane = panePending
	m.selPend = 0 // P1: "Fix invoice", project Acme, due 2026-06-01 (overdue vs today 2026-06-02)
	body := m.renderPendingDetail(40, 100)
	for _, want := range []string{"P1", "Acme", "Fix invoice", "due"} {
		if !strings.Contains(body, want) {
			t.Errorf("pending detail missing %q:\n%s", want, body)
		}
	}
	// a long description must appear in full (wrapped), not truncated with an ellipsis
	m.pending[0].Desc = "fix the content blocks fonts so they take the right size on mobile and desktop"
	body = m.renderPendingDetail(30, 100)
	if !strings.Contains(stripANSI(body), "desktop") {
		t.Errorf("long description should wrap and show its tail:\n%s", body)
	}
	// the main column swaps the Timeline panel for the Pending detail when focused
	full := m.View()
	if !strings.Contains(full, "Pending · P1") {
		t.Errorf("focusing Pending should title the detail panel 'Pending · P1'")
	}
}

func TestIconToggle(t *testing.T) {
	defer SetIcons("off") // restore the universal default for other tests

	// default (off) keeps the ASCII markers
	SetIcons("off")
	if g, _ := statusGlyph("in-progress"); g != ">" {
		t.Errorf("icons off: running glyph = %q, want %q", g, ">")
	}
	if g, _ := statusGlyph("paused"); g != "=" {
		t.Errorf("icons off: paused glyph = %q, want %q", g, "=")
	}

	// on swaps in the Nerd-Font icons (PUA codepoints)
	SetIcons("on")
	if g, _ := statusGlyph("in-progress"); g != "" {
		t.Errorf("icons on: running glyph = %q (%U), want play \\uf04b", g, []rune(g))
	}
	if g, _ := statusGlyph("completed"); g != "" {
		t.Errorf("icons on: completed glyph = %q, want check \\uf00c", g)
	}
	// an unknown mode is treated as off
	SetIcons("garbage")
	if g, _ := statusGlyph("paused"); g != "=" {
		t.Errorf("unknown icons mode should fall back to ASCII, got %q", g)
	}

	// the choice flows through to actual rendering (drilled's T1 is completed)
	SetIcons("on")
	if out := drilled().renderTasks(40, 100); !strings.Contains(out, "") {
		t.Errorf("icons on: task list should render the Nerd-Font check glyph")
	}
	SetIcons("off")
	if out := drilled().renderTasks(40, 100); !strings.Contains(out, "x ") {
		t.Errorf("icons off: task list should render the ASCII 'x' glyph")
	}
}

func TestIndicatorsRespectIconToggle(t *testing.T) {
	defer SetIcons("off")

	m := sampleModel()
	m.autoPause = false
	// pause badge: ASCII when off, glyph when on
	SetIcons("off")
	if b := m.pauseBadge(); !strings.Contains(b, "|| parallel") {
		t.Errorf("icons off: pause badge = %q, want ASCII '|| parallel'", b)
	}
	SetIcons("on")
	if b := m.pauseBadge(); !strings.Contains(b, "∥ parallel") {
		t.Errorf("icons on: pause badge = %q, want '∥ parallel'", b)
	}

	// rollup: a space sits between the glyph and the count
	SetIcons("off")
	r := sampleModel()
	r.live = &wj.Status{Tasks: []wj.Task{
		{Status: "in-progress"}, {Status: "in-progress"}, {Status: "completed"},
	}, TotalMinutes: 120}
	if roll := r.todayRollup(); !strings.Contains(roll, "> 2") {
		t.Errorf("rollup should space the glyph from the count ('> 2'), got %q", roll)
	}
}

func TestPendingAddProjectAutocomplete(t *testing.T) {
	m := pendingModel()
	m.projects = []string{"backend", "backlog", "frontend"}
	m.input = inputMode{active: true, action: "add", value: "fix the bug @ba"}
	tab := func(mm Model) Model { n, _ := mm.handleKey(tea.KeyMsg{Type: tea.KeyTab}); return n.(Model) }
	m = tab(m)
	if m.input.value != "fix the bug @backend" {
		t.Fatalf("tab 1 -> %q, want 'fix the bug @backend'", m.input.value)
	}
	m = tab(m) // cycles to the next @ba* match, keeping the description intact
	if m.input.value != "fix the bug @backlog" {
		t.Fatalf("tab 2 -> %q, want 'fix the bug @backlog'", m.input.value)
	}
}

func TestParseTagInput(t *testing.T) {
	adds, removes := parseTagInput("billing -urgent #priority -")
	if strings.Join(adds, ",") != "billing,#priority" {
		t.Errorf("adds = %v, want [billing #priority]", adds)
	}
	if strings.Join(removes, ",") != "urgent" {
		t.Errorf("removes = %v, want [urgent]", removes)
	}
}

func TestTagEditorOpensAndSubmits(t *testing.T) {
	m := drilled() // confirmAll, so '#' arms a confirm first
	m, _ = mustModel(m.handleKey(keyMsg("#")))
	if !m.confirm.active || !m.confirm.input.active {
		t.Fatalf("'#' should arm a tags confirm, got %+v", m.confirm)
	}
	m, _ = mustModel(m.handleKey(keyMsg("y")))
	if !m.input.active || m.input.action != "tags" || m.input.taskID != "T1" {
		t.Fatalf("confirming '#' should open the tags editor for T1, got %+v", m.input)
	}
	m.input.value = "billing -urgent"
	if _, cmd := m.handleKey(keyMsg("enter")); cmd == nil {
		t.Error("submitting tags should issue tag/untag commands")
	}
}

func TestTagAutocomplete(t *testing.T) {
	tab := func(mm Model) Model { n, _ := mm.handleKey(tea.KeyMsg{Type: tea.KeyTab}); return n.(Model) }
	m := drilled()
	m.tags = []string{"billing", "backend-fix", "urgent"}
	m.input = inputMode{active: true, action: "tags", value: "fix it b"}
	m = tab(m)
	if m.input.value != "fix it billing" {
		t.Fatalf("tab1 -> %q, want 'fix it billing'", m.input.value)
	}
	m = tab(m)
	if m.input.value != "fix it backend-fix" {
		t.Fatalf("tab2 -> %q, want 'fix it backend-fix'", m.input.value)
	}
	// a removal token keeps its leading '-'
	m2 := drilled()
	m2.tags = []string{"urgent"}
	m2.input = inputMode{active: true, action: "tags", value: "-u"}
	if got := tab(m2).input.value; got != "-urgent" {
		t.Fatalf("removal autocomplete -> %q, want '-urgent'", got)
	}
}

func TestTimelineShowsTags(t *testing.T) {
	m := drilled()
	m.show.Tags = []string{"billing", "urgent"}
	out := m.renderTimeline(60, 100)
	if !strings.Contains(out, "#billing") || !strings.Contains(out, "#urgent") {
		t.Errorf("timeline should render tag chips:\n%s", out)
	}
}

func TestTaskOwned(t *testing.T) {
	m := Model{actor: "me"}
	if !m.taskOwned(wj.GridTask{Actor: "me"}) {
		t.Error("your own task should be owned")
	}
	if m.taskOwned(wj.GridTask{Actor: "alex"}) {
		t.Error("a teammate's task should NOT be owned")
	}
	if !m.taskOwned(wj.GridTask{Actor: ""}) {
		t.Error("empty actor (solo/legacy data) should count as owned")
	}
	m.actor = "" // actor not loaded yet -> don't block
	if !m.taskOwned(wj.GridTask{Actor: "alex"}) {
		t.Error("unloaded actor should not block actions")
	}
}

func TestTeammateTaskGating(t *testing.T) {
	m := drilled() // Day pane, confirmAll
	m.actor = "me"
	m.grid.Tasks = []wj.GridTask{
		{ID: "alex/T1", Actor: "alex", Project: "docs", Desc: "their task", Status: "in-progress"},
	}
	m.selTask = 0
	// every mutation key on a teammate's task is consumed with a notice, no command
	for _, k := range []string{"p", "r", "c", "d", "x", "a", "m", "#", "o"} {
		next, cmd, handled := m.keyMutation(keyMsg(k))
		nm := next.(Model)
		if !handled || cmd != nil {
			t.Errorf("%q on a teammate task: handled=%v cmd=%v — want consumed, no cmd", k, handled, cmd)
		}
		if nm.notice == "" || nm.confirm.active {
			t.Errorf("%q should set a read-only notice and NOT arm a confirm/mutation", k)
		}
	}
	// your own task is actionable again (arms a confirm in 'all' mode)
	m.grid.Tasks = []wj.GridTask{{ID: "T1", Actor: "me", Project: "backend", Status: "in-progress"}}
	next, _, handled := m.keyMutation(keyMsg("p"))
	if !handled || !next.(Model).confirm.active {
		t.Error("'p' on your own task should arm a confirm")
	}
}

func TestMyTasksFiltersToOwnActor(t *testing.T) {
	m := liveModel()
	m.actor = "me"
	m.live.Tasks = []wj.Task{
		{ID: "T1", Actor: "me", Project: "backend", Status: "in-progress", Minutes: 60},
		{ID: "alex/T1", Actor: "alex", Project: "ops", Status: "in-progress", Minutes: 30},
		{ID: "T2", Actor: "me", Project: "docs", Status: "completed", Minutes: 20},
	}
	if got := len(m.myTasks()); got != 2 {
		t.Fatalf("myTasks = %d, want 2 (your own only)", got)
	}
	// the header/active project is YOUR running project, not alex's ops
	if got := m.activeProject(); got != "backend" {
		t.Errorf("activeProject = %q, want backend (yours)", got)
	}
	// the Today section excludes the teammate's project
	for _, r := range m.todayRows() {
		if r.project == "ops" {
			t.Error("Today section should exclude a teammate's project (ops)")
		}
	}
	// unset actor (solo / pre-collab) -> all tasks (back-compat)
	m.actor = ""
	if got := len(m.myTasks()); got != 3 {
		t.Errorf("unset actor should return all %d tasks, got %d", 3, got)
	}
}

func TestTeammatePendingGating(t *testing.T) {
	m := pendingModel()
	m.pane = panePending
	m.actor = "me"
	m.pending = []wj.Pending{{ID: "alex/P1", Actor: "alex", Desc: "their backlog item"}}
	m.selPend = 0
	for _, k := range []string{"enter", "d", "x", "[", "]"} {
		next, _ := mustModel(m.handleKey(keyMsg(k)))
		if next.notice == "" {
			t.Errorf("%q on a teammate's pending item should set a read-only notice", k)
		}
		if next.input.active || next.confirm.active {
			t.Errorf("%q on a teammate's pending item must not open input/confirm", k)
		}
	}
	// your own pending item is still actionable
	m.pending = []wj.Pending{{ID: "P1", Actor: "me", Desc: "mine"}}
	next, _ := mustModel(m.handleKey(keyMsg("x")))
	if !next.confirm.active {
		t.Error("'x' on your own pending item should arm the drop confirm")
	}
}

func TestPendingAssignKey(t *testing.T) {
	m := pendingModel()
	m.actor = "me"
	m.actors = []string{"alex", "me", "sam"} // teammates exist, so push opens a prompt

	// a teammate's item: '@' claims it to you -> runs `assign alice/P1 me`
	m.confirmLevel = confirmAll // so the claim arms an inspectable confirm
	m.pending = []wj.Pending{{ID: "alice/P1", Actor: "alice", Desc: "theirs"}}
	m.selPend = 0
	next, _ := mustModel(m.handleKey(keyMsg("@")))
	if !next.confirm.active || next.confirm.verb != "assign" {
		t.Fatalf("@ on a teammate's item should arm an assign confirm, got %+v", next.confirm)
	}
	if got := next.confirm.valueArgs; len(got) != 2 || got[0] != "alice/P1" || got[1] != "me" {
		t.Errorf("claim args = %v, want [alice/P1 me]", got)
	}

	// your own item: '@' opens the assign prompt where you type the assignee
	m.confirmLevel = confirmOff
	m.pending = []wj.Pending{{ID: "P2", Actor: "me", Desc: "mine"}}
	m.selPend = 0
	next, _ = mustModel(m.handleKey(keyMsg("@")))
	if !next.input.active || next.input.action != "assign" || next.input.taskID != "P2" {
		t.Fatalf("@ on your own item should open the assign prompt for P2, got %+v", next.input)
	}
	// submitting the assignee issues `assign P2 <who>` (no error path)
	next.input.value = "bob"
	out, _ := mustModel(next.handleKey(keyMsg("enter")))
	if out.input.active {
		t.Error("submitting the assignee should close the prompt")
	}
}

func TestAssignActorAutocomplete(t *testing.T) {
	m := pendingModel()
	m.actor = "me"
	m.actors = []string{"alex", "me", "sam"} // includes self; completion skips it
	// open the assign prompt as if @ was pressed on your own item
	m.input = inputMode{active: true, action: "assign", taskID: "P2", value: ""}
	// Tab cycles through teammate handles (not yourself), alphabetical
	next, _ := mustModel(m.handleKey(keyMsg("tab")))
	if next.input.value != "alex" {
		t.Fatalf("first Tab = %q, want alex", next.input.value)
	}
	next, _ = mustModel(next.handleKey(keyMsg("tab")))
	if next.input.value != "sam" {
		t.Fatalf("second Tab = %q, want sam (skipping yourself)", next.input.value)
	}
	// actorMatches never offers yourself
	for _, a := range m.actorMatches("") {
		if a == "me" {
			t.Error("actorMatches should exclude yourself")
		}
	}
}

func TestVisiblePendingOrdersMineFirst(t *testing.T) {
	m := pendingModel()
	m.actor = "me"
	m.pending = []wj.Pending{
		{ID: "alex/P1", Actor: "alex", Desc: "a"},
		{ID: "P1", Actor: "me", Desc: "mine-1"},
		{ID: "sam/P1", Actor: "sam", Desc: "s"},
		{ID: "P2", Actor: "me", Desc: "mine-2"},
	}
	// default (everyone): your own first (stable), teammates' below (stable)
	got := m.visiblePending()
	want := []string{"P1", "P2", "alex/P1", "sam/P1"}
	for i, w := range want {
		if got[i].ID != w {
			t.Fatalf("order[%d] = %q, want %q (full: %v)", i, got[i].ID, w, ids(got))
		}
	}
	// filtering to your own handle (M) hides teammates entirely
	m.filterActor = "me"
	got = m.visiblePending()
	if len(got) != 2 || got[0].ID != "P1" || got[1].ID != "P2" {
		t.Fatalf("mine-only pending = %v, want [P1 P2]", ids(got))
	}
	// filtering to a teammate (F) shows only their items
	m.filterActor = "alex"
	if got := m.visiblePending(); len(got) != 1 || got[0].ID != "alex/P1" {
		t.Fatalf("alex-only pending = %v, want [alex/P1]", ids(got))
	}
	// solo (no actor) is untouched, original order preserved
	m.filterActor = ""
	m.actor = ""
	if got := m.visiblePending(); len(got) != 4 || got[0].ID != "alex/P1" {
		t.Errorf("solo should pass through unchanged, got %v", ids(got))
	}
}

func ids(ps []wj.Pending) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

func TestFilteredTasksOrderMineFirst(t *testing.T) {
	m := drilled()
	m.actor = "me"
	m.grid.Tasks = []wj.GridTask{
		{ID: "alex/T1", Actor: "alex", Project: "ops", Status: "in-progress"},
		{ID: "T1", Actor: "me", Project: "backend", Status: "in-progress"},
		{ID: "T2", Actor: "me", Project: "docs", Status: "completed"},
	}
	got := m.filteredTasks()
	if len(got) != 3 || got[0].ID != "T1" || got[1].ID != "T2" || got[2].ID != "alex/T1" {
		t.Fatalf("tasks should be mine-first, got %v", []string{got[0].ID, got[1].ID, got[2].ID})
	}
}

func TestFilteredTasksActorFilter(t *testing.T) {
	m := drilled()
	m.actor = "me"
	m.grid.Tasks = []wj.GridTask{
		{ID: "T1", Actor: "me", Project: "backend", Status: "in-progress"},
		{ID: "alex/T1", Actor: "alex", Project: "ops", Status: "in-progress"},
	}
	if got := len(m.filteredTasks()); got != 2 {
		t.Fatalf("default (everyone) = %d, want 2", got)
	}
	// M: filter to your own handle
	m.filterActor = "me"
	got := m.filteredTasks()
	if len(got) != 1 || got[0].ID != "T1" {
		t.Fatalf("mine-only should keep only your own task, got %v", got)
	}
	// F: filter to a teammate
	m.filterActor = "alex"
	got = m.filteredTasks()
	if len(got) != 1 || got[0].ID != "alex/T1" {
		t.Fatalf("alex-only should keep only alex's task, got %v", got)
	}
	// solo/pre-collab (no actor) is unaffected by the filter
	m.actor = ""
	if len(m.filteredTasks()) != 2 {
		t.Error("actor filter with no actor should show all tasks (back-compat)")
	}
}

// TestActorFilterPicker drives the F picker: open it, jump to a teammate, and
// confirm the whole-view filter is set; then M toggles mine on and off.
func TestActorFilterPicker(t *testing.T) {
	m := drilled()
	m.actor = "me"
	m.actors = []string{"me", "alex", "sam"}
	m.grid.Tasks = []wj.GridTask{
		{ID: "T1", Actor: "me", Project: "backend", Status: "in-progress"},
		{ID: "alex/T1", Actor: "alex", Project: "ops", Status: "in-progress"},
	}

	// F opens the picker, starting on the current ("" = everyone) row
	m, _ = mustModel(m.handleKey(keyMsg("F")))
	if !m.actorPick.active {
		t.Fatal("F should open the author picker")
	}
	opts := m.actorPickOptions() // ["", "me", "alex", "sam"]
	if len(opts) != 4 || opts[0] != "" || opts[2] != "alex" {
		t.Fatalf("picker options = %v", opts)
	}

	// picking alex (label "3.": "" then me then alex) filters the whole view
	m, _ = mustModel(m.handleKey(keyMsg("3")))
	if m.actorPick.active {
		t.Error("choosing a row should close the picker")
	}
	if m.filterActor != "alex" {
		t.Fatalf("filterActor = %q, want alex", m.filterActor)
	}
	if got := m.filteredTasks(); len(got) != 1 || got[0].ID != "alex/T1" {
		t.Fatalf("after F→alex, tasks = %v, want [alex/T1]", taskIDs(got))
	}
	if m.filterLabel() != "alex" {
		t.Errorf("filterLabel = %q, want alex", m.filterLabel())
	}

	// M from a teammate filter switches to mine
	m, _ = mustModel(m.handleKey(keyMsg("M")))
	if m.filterActor != "me" {
		t.Fatalf("M should set filterActor to me, got %q", m.filterActor)
	}
	if m.filterLabel() != "mine" {
		t.Errorf("filterLabel = %q, want mine", m.filterLabel())
	}
	// M again toggles back to everyone
	m, _ = mustModel(m.handleKey(keyMsg("M")))
	if m.filterActor != "" {
		t.Fatalf("second M should clear the filter, got %q", m.filterActor)
	}
}

// TestActorFilterPickerSolo: in a non-shared log F and M are inert no-ops.
func TestActorFilterPickerSolo(t *testing.T) {
	m := drilled()
	m.actor = "" // solo / pre-collab
	m, _ = mustModel(m.handleKey(keyMsg("F")))
	if m.actorPick.active {
		t.Error("F should not open the picker in a solo log")
	}
	m, _ = mustModel(m.handleKey(keyMsg("M")))
	if m.filterActor != "" {
		t.Errorf("M should be a no-op in a solo log, got %q", m.filterActor)
	}
}

func taskIDs(ts []wj.GridTask) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return out
}

func TestByCyclesThroughPerson(t *testing.T) {
	m := sampleModel()
	m.pane = paneRange
	m.by = "project"
	for _, want := range []string{"task", "person", "project"} {
		next, _ := mustModel(m.handleKey(keyMsg("b")))
		if next.by != want {
			t.Fatalf("b cycle: got %q, want %q", next.by, want)
		}
		m = next
	}
}

func TestTeamOverlayRender(t *testing.T) {
	m := sampleModel()
	m.actor = "me"
	m.showTeam = true
	m.team = []wj.Member{
		{Actor: "me", Running: true, Desc: "invoice flow", Project: "backend", Minutes: 30, TotalMinutes: 90},
		{Actor: "alex", Running: false, TotalMinutes: 120},
	}
	out := m.teamOverlay()
	for _, want := range []string{"me (you)", "invoice flow", "alex", "idle"} {
		if !strings.Contains(out, want) {
			t.Errorf("team overlay missing %q in:\n%s", want, out)
		}
	}
}

// pickModel is a model on the Day pane with a few tasks, for the J picker tests.
func pickModel() Model {
	m := sampleModel()
	m.pane = paneDay
	m.grid = &wj.Grid{
		Date: "2026-05-28",
		Tasks: []wj.GridTask{
			{ID: "T1", Project: "backend", Desc: "Refactor auth", Status: "completed", Minutes: 180},
			{ID: "T2", Project: "backend", Desc: "Wire up cache", Status: "in-progress", Minutes: 42},
			{ID: "T3", Project: "meetings", Desc: "Standup", Status: "completed", Minutes: 15},
		},
	}
	return m
}

func TestPickerJumpsByDigit(t *testing.T) {
	m := pickModel()
	// J opens the picker without changing the selection yet
	m, _ = mustModel(m.handleKey(keyMsg("J")))
	if !m.pick.active {
		t.Fatal("J should open the task picker")
	}
	// pressing 3 jumps straight to the third listed task and focuses Tasks
	m, _ = mustModel(m.handleKey(keyMsg("3")))
	if m.pick.active {
		t.Error("picker should close after a digit jump")
	}
	if m.selTask != 2 {
		t.Errorf("selTask = %d, want 2 (the third task)", m.selTask)
	}
	if m.pane != paneDay {
		t.Errorf("pane = %d, want paneDay so the highlight shows", m.pane)
	}
	if id := m.selectedTaskID(); id != "T3" {
		t.Errorf("selected = %q, want T3", id)
	}
}

func TestPickerJumpsByLetter(t *testing.T) {
	// build a model with more rows than digits so letters are exercised
	m := sampleModel()
	m.pane = paneDay
	tasks := make([]wj.GridTask, 11)
	for i := range tasks {
		tasks[i] = wj.GridTask{ID: fmt.Sprintf("T%d", i+1), Project: "backend", Status: "completed", Minutes: 10}
	}
	m.grid = &wj.Grid{Date: "2026-05-28", Tasks: tasks}
	m, _ = mustModel(m.handleKey(keyMsg("J")))
	// the 11th row's label is the 11th entry in pickKeys: "1".."9" then "a","b"
	if pickKeys[10] != "b" {
		t.Fatalf("pickKeys[10] = %q, want b", pickKeys[10])
	}
	m, _ = mustModel(m.handleKey(keyMsg("b")))
	if m.pick.active || m.selTask != 10 {
		t.Errorf("'b' should jump to row 10: active=%v selTask=%d", m.pick.active, m.selTask)
	}
}

func TestPickerLetterKeysSkipNavBindings(t *testing.T) {
	// j/k/g/q must stay navigation/cancel, never become row labels
	for _, reserved := range []string{"j", "k", "g", "q"} {
		if indexOf(pickKeys, reserved) >= 0 {
			t.Errorf("%q must not be a pick label (it's a nav/cancel key)", reserved)
		}
	}
}

func TestPickerNavigateAndEnter(t *testing.T) {
	m := pickModel()
	m, _ = mustModel(m.handleKey(keyMsg("J")))
	m, _ = mustModel(m.handleKey(keyMsg("j"))) // 0 -> 1
	m, _ = mustModel(m.handleKey(keyMsg("j"))) // 1 -> 2
	m, _ = mustModel(m.handleKey(keyMsg("k"))) // 2 -> 1
	if m.pick.sel != 1 {
		t.Fatalf("pick.sel = %d, want 1 after jjk", m.pick.sel)
	}
	m, _ = mustModel(m.handleKey(keyMsg("enter")))
	if m.pick.active || m.selTask != 1 {
		t.Errorf("enter should select row 1: active=%v selTask=%d", m.pick.active, m.selTask)
	}
}

func TestPickerDigitOutOfRangeIgnored(t *testing.T) {
	m := pickModel() // 3 tasks
	m, _ = mustModel(m.handleKey(keyMsg("J")))
	m, _ = mustModel(m.handleKey(keyMsg("9"))) // no 9th task
	if !m.pick.active {
		t.Error("an out-of-range digit should be a no-op, leaving the picker open")
	}
	if m.selTask != 0 {
		t.Errorf("selTask moved to %d on an out-of-range digit", m.selTask)
	}
	// esc closes without changing the selection
	m, _ = mustModel(m.handleKey(keyMsg("esc")))
	if m.pick.active {
		t.Error("esc should close the picker")
	}
}

func TestPickerNoTasksDoesNotOpen(t *testing.T) {
	m := sampleModel()
	m.pane = paneDay
	m.grid = &wj.Grid{Date: "2026-05-28"} // no tasks
	m, _ = mustModel(m.handleKey(keyMsg("J")))
	if m.pick.active {
		t.Error("J on an empty day should not open the picker")
	}
	if m.notice == "" {
		t.Error("J on an empty day should explain there's nothing to jump to")
	}
}

func TestTimedCarryOverPromptsForTime(t *testing.T) {
	m := pickModel()
	m.today = "2026-06-01"      // the focused day (2026-05-28) is in the past
	m.confirmLevel = confirmOff // skip the y/n guard so the prompt opens directly
	m.selTask = 0               // T1
	m, _ = mustModel(m.handleKey(keyMsg("O")))
	if !m.input.active || m.input.action != "at" {
		t.Fatalf("O should open an at-time prompt: active=%v action=%q", m.input.active, m.input.action)
	}
	// the pending argv carries `continue T1 ... --date <source day>`; the prompt
	// later appends --at <time>.
	joined := strings.Join(m.input.pending, " ")
	for _, want := range []string{"continue", "T1", "--date", "2026-05-28"} {
		if !strings.Contains(joined, want) {
			t.Errorf("pending argv %q missing %q", joined, want)
		}
	}
	if strings.Contains(joined, "--at") {
		t.Errorf("pending should not carry --at yet: %q", joined)
	}
}

func TestTimedCarryOverRejectsToday(t *testing.T) {
	m := pickModel()
	m.today = "2026-05-28" // same as the focused day — nothing to carry over
	m.confirmLevel = confirmOff
	m.selTask = 0
	m, _ = mustModel(m.handleKey(keyMsg("O")))
	if m.input.active {
		t.Error("O on today should not open a prompt")
	}
	if m.notice == "" {
		t.Error("O on today should explain it copies from a past day")
	}
}
