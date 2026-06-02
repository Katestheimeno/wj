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
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func sampleModel() Model {
	return Model{
		by: "project", from: "2026-05-28", to: "2026-06-01", ready: true,
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
	if got := bar(0, 100, 7, "39"); strings.Contains(got, "█") {
		t.Errorf("zero minutes should render no block, got %q", got)
	}
	if got := bar(1, 100, 7, "39"); !strings.Contains(got, "█") {
		t.Errorf("tiny work should render a sliver, got %q", got)
	}
	// every cell is exactly width-wide once color codes are stripped
	if w := len([]rune(stripANSI(bar(50, 100, 7, "39")))); w != 7 {
		t.Errorf("bar width = %d, want 7", w)
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

func TestProjectColorStable(t *testing.T) {
	if ProjectColor("backend") != ProjectColor("backend") {
		t.Error("project color must be deterministic")
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
		Date: "2026-05-28", ShiftStart: "09:00", ShiftEnd: "19:00", SlotMinutes: 5, Now: "12:30",
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
	for _, want := range []string{"09:00", "T1", "backend", "Refactor auth", "started", "completed", "Day — 2026-05-28", "legend:", "▲"} {
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
	if m = step(m, "tab"); m.pane != paneRange {
		t.Fatalf("tab wraps -> %d, want paneRange", m.pane)
	}
	// esc from a non-range pane returns to range
	m.pane = paneTimeline
	if m = step(m, "esc"); m.pane != paneRange {
		t.Fatalf("esc -> %d, want paneRange", m.pane)
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
	m.input = inputMode{active: true, action: "amend", prompt: "amend T1 (new description)", value: "Refac"}
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
	next, cmd := m.handleKey(keyMsg("s"))
	nm := next.(Model)
	if !nm.input.active || nm.input.action != "start" {
		t.Fatalf("'s' should open the start prompt, got %+v", nm.input)
	}
	if cmd != nil {
		t.Error("opening the prompt should not issue a command yet")
	}
}

func TestInputTypingAndSubmit(t *testing.T) {
	m := drilled()
	// open amend on the selected task
	m, _ = mustModel(m.handleKey(keyMsg("a")))
	if !m.input.active || m.input.action != "amend" || m.input.taskID != "T1" {
		t.Fatalf("'a' should open amend for T1, got %+v", m.input)
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
	if !m.input.active {
		t.Fatal("'m' should open move prompt")
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
	next, cmd := mustModel(m.handleKey(keyMsg("p")))
	if next.input.active {
		t.Error("a today mutation must not open a time prompt")
	}
	if cmd == nil {
		t.Error("a today mutation should issue immediately")
	}
}

func TestPastDayMutationPromptsForTime(t *testing.T) {
	m := drilled()         // focused day 2026-05-28
	m.today = "2026-06-01" // …which is in the past
	next, cmd := mustModel(m.handleKey(keyMsg("p")))
	if !next.input.active || next.input.action != "at" {
		t.Fatalf("past-day pause should open a time prompt, got %+v", next.input)
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

func TestMutationKeyGatedToDetailPane(t *testing.T) {
	// in the range pane, 'c' is not a mutation (no selected task context there)
	m := sampleModel() // paneRange
	_, cmd := m.handleKey(keyMsg("c"))
	if cmd != nil {
		t.Error("'c' in range pane should not trigger a mutation")
	}
	// in the day pane with a selection, 'p' issues a command
	d := drilled()
	if _, cmd := d.handleKey(keyMsg("p")); cmd == nil {
		t.Error("'p' on a selected task should issue a mutation command")
	}
}

func mustModel(mod tea.Model, cmd tea.Cmd) (Model, tea.Cmd) {
	return mod.(Model), cmd
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
	// "1" => single-day window ending today
	n, cmd := mustModel(m.handleKey(keyMsg("1")))
	if n.from != "2026-06-01" || n.to != "2026-06-01" {
		t.Errorf("span 1: from=%q to=%q", n.from, n.to)
	}
	if cmd == nil {
		t.Error("span change should reload")
	}
	// "7" => last 7 days
	n, _ = mustModel(m.handleKey(keyMsg("7")))
	if n.from != "2026-05-26" || n.to != "2026-06-01" {
		t.Errorf("span 7: from=%q to=%q", n.from, n.to)
	}
}

func TestLogKeyOpensPrompt(t *testing.T) {
	m := drilled()
	m.today = m.currentDay()
	n, _ := mustModel(m.handleKey(keyMsg("n")))
	if !n.input.active || n.input.action != "log" {
		t.Fatalf("'n' should open a log (note) prompt, got %+v", n.input)
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
	if !strings.Contains(h, "▶") || !strings.Contains(h, "T1") || !strings.Contains(h, "Refactor auth") {
		t.Errorf("running header = %q", h)
	}
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
