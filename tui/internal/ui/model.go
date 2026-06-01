// Package ui implements the wj-tui front-end. The headline view is a multi-day
// range Gantt; selecting a day drills into that day's intraday Gantt, and
// selecting a task shows its timeline. This is the step-4 scaffold: three
// read-only panels with focus cycling and lazy loading. Mutation keybindings
// arrive in step 5.
package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Katestheimeno/wj/tui/internal/wj"
)

const (
	dateLayout     = "2006-01-02"
	labelW         = 18              // project/task label column width
	refresh        = 1 * time.Second // display tick (running clock)
	dataEveryTicks = 30              // re-shell the CLI for fresh data every N ticks
)

// pane identifies which panel currently has keyboard focus.
type pane int

const (
	paneRange    pane = iota // the multi-day Gantt (navigate days/rows)
	paneDay                  // the focused day's intraday Gantt (navigate tasks)
	paneTimeline             // the selected task's event timeline
	paneCount
)

// messages. grid/show messages carry their request key so a stale async result
// (from a day/task the user already navigated away from) can be discarded.
type ganttMsg struct {
	g   *wj.Gantt
	err error
}
type gridMsg struct {
	day string
	g   *wj.Grid
	err error
}
type showMsg struct {
	id, day string
	s       *wj.Show
	err     error
}
type tickMsg struct{}

// liveMsg carries today's status (for the running-task clock in the header).
type liveMsg struct {
	s   *wj.Status
	err error
}

// projectsMsg carries the list of known projects (for move autocomplete).
type projectsMsg struct{ names []string }

// mutationMsg is the result of a state-changing `wj` invocation.
type mutationMsg struct{ err error }

// inputMode is an inline text prompt. Two flavors:
//   - value entry for start/amend/move (action = the verb), then issued via
//     issueMutation (which may chain into a time prompt on a past day);
//   - time entry (action = "at") for a mutation on a non-today day, carrying the
//     fully-built argv in `pending`, to which "--at <value>" is appended.
type inputMode struct {
	active   bool
	action   string // "start" | "amend" | "move" | "log" | "at"
	prompt   string
	value    string
	taskID   string   // target task for amend/move
	pending  []string // for action "at": the wj argv awaiting an --at suffix
	acPrefix string   // move autocomplete: the prefix Tab cycles matches for
}

// confirmMode is a y/n guard for destructive mutations (cancel).
type confirmMode struct {
	active    bool
	prompt    string
	verb      string   // wj verb to run on confirmation
	valueArgs []string // args between the verb and --date (e.g. the task id)
}

// Model is the root Bubble Tea model.
type Model struct {
	cli wj.Client

	today    string // YYYY-MM-DD; mutations on other days require an explicit --at
	from, to string // current range (YYYY-MM-DD); empty until first load
	by       string // "project" | "task"

	g          *wj.Gantt
	focusedDay int // index into g.Days
	focusedRow int // index into g.Rows (range-pane highlight)

	grid    *wj.Grid // intraday data for the focused day
	selTask int      // index into grid.Tasks

	show *wj.Show // timeline of the selected task

	pane          pane
	input         inputMode
	confirm       confirmMode
	showHelp      bool
	err           string
	width, height int
	ready         bool
	focusInit     bool // whether focusedDay has been defaulted yet

	live     *wj.Status // today's status, for the running-task header clock
	liveAt   time.Time  // wall-clock time m.live was fetched
	projects []string   // known project names (move autocomplete)
	tickN    int        // 1s ticks since start; data reloads every dataEveryTicks
	tlOffset int        // timeline scroll position
}

// New builds the initial model. from/to may be empty to use the CLI default
// range; by defaults to "project".
func New(cli wj.Client, from, to, by string) Model {
	if by == "" {
		by = "project"
	}
	return Model{cli: cli, from: from, to: to, by: by, today: time.Now().Format(dateLayout)}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadGantt(), m.loadLive(), m.loadProjects(), tickCmd())
}

// currentDay is the YYYY-MM-DD of the focused day column ("" if none).
func (m Model) currentDay() string {
	if m.g == nil || m.focusedDay < 0 || m.focusedDay >= len(m.g.Days) {
		return ""
	}
	return m.g.Days[m.focusedDay]
}

// selectedTaskID is the id of the selected intraday task ("" if none).
func (m Model) selectedTaskID() string {
	if m.grid == nil || m.selTask < 0 || m.selTask >= len(m.grid.Tasks) {
		return ""
	}
	return m.grid.Tasks[m.selTask].ID
}

// loaders ---------------------------------------------------------------------

func (m Model) loadGantt() tea.Cmd {
	cli, from, to, by := m.cli, m.from, m.to, m.by
	return func() tea.Msg {
		g, err := cli.Gantt(from, to, by)
		return ganttMsg{g: g, err: err}
	}
}

func (m Model) loadGrid(day string) tea.Cmd {
	if day == "" {
		return nil
	}
	cli := m.cli
	return func() tea.Msg {
		g, err := cli.Grid(day)
		return gridMsg{day: day, g: g, err: err}
	}
}

func (m Model) loadShow(id, day string) tea.Cmd {
	if id == "" || day == "" {
		return nil
	}
	cli := m.cli
	return func() tea.Msg {
		s, err := cli.Show(id, day)
		return showMsg{id: id, day: day, s: s, err: err}
	}
}

func (m Model) loadLive() tea.Cmd {
	cli, today := m.cli, m.today
	if today == "" {
		return nil
	}
	return func() tea.Msg {
		s, err := cli.Status(today)
		return liveMsg{s: s, err: err}
	}
}

func (m Model) loadProjects() tea.Cmd {
	cli := m.cli
	return func() tea.Msg {
		names, err := cli.Projects()
		if err != nil {
			return projectsMsg{}
		}
		return projectsMsg{names: names}
	}
}

func tickCmd() tea.Cmd {
	return tea.Every(refresh, func(time.Time) tea.Msg { return tickMsg{} })
}

// mutate runs a state-changing `wj` command asynchronously.
func (m Model) mutate(args ...string) tea.Cmd {
	cli := m.cli
	return func() tea.Msg {
		return mutationMsg{err: cli.Mutate(args...)}
	}
}

// reloadAll refreshes every panel from the CLI (used after a mutation).
func (m Model) reloadAll() tea.Cmd {
	return tea.Batch(m.loadGantt(), m.loadGrid(m.currentDay()),
		m.loadShow(m.selectedTaskID(), m.currentDay()), m.loadLive())
}

// Update ----------------------------------------------------------------------

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		return m, nil

	case tickMsg:
		// every tick re-renders (advancing the header clock); data is re-shelled
		// only every dataEveryTicks to keep open-task durations fresh cheaply.
		m.tickN++
		cmds := []tea.Cmd{tickCmd()}
		if m.tickN%dataEveryTicks == 0 {
			cmds = append(cmds, m.loadGantt(), m.loadGrid(m.currentDay()),
				m.loadShow(m.selectedTaskID(), m.currentDay()), m.loadLive())
		}
		return m, tea.Batch(cmds...)

	case liveMsg:
		if msg.err == nil {
			m.live = msg.s
			m.liveAt = time.Now()
		}
		return m, nil

	case projectsMsg:
		m.projects = msg.names
		return m, nil

	case ganttMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		// NB: do not clear m.err here — a background reload must not erase a
		// mutation/load error before the user has seen it (cleared on keypress).
		m.g = msg.g
		m.from, m.to = msg.g.From, msg.g.To
		if !m.focusInit && len(m.g.Days) > 0 {
			m.focusedDay = len(m.g.Days) - 1 // start on the most recent day (today)
			m.focusInit = true
		}
		m.focusedDay = clamp(m.focusedDay, 0, len(m.g.Days)-1)
		m.focusedRow = clamp(m.focusedRow, 0, len(m.g.Rows)-1)
		return m, m.loadGrid(m.currentDay()) // refresh the drill-down too

	case gridMsg:
		if msg.day != m.currentDay() {
			return m, nil // stale
		}
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.grid = msg.g
		m.selTask = clamp(m.selTask, 0, len(m.grid.Tasks)-1)
		if m.selectedTaskID() == "" {
			m.show = nil // day has no tasks — drop any stale timeline
		}
		return m, m.loadShow(m.selectedTaskID(), m.currentDay())

	case showMsg:
		if msg.day != m.currentDay() || msg.id != m.selectedTaskID() {
			return m, nil // stale
		}
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.show = msg.s
		m.tlOffset = 0 // reset timeline scroll for the newly-loaded task
		return m, nil

	case mutationMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.err = ""
		}
		// reload regardless: even on a CLI error the log may have changed
		return m, m.reloadAll()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// active overlays capture all input
	if m.input.active {
		return m.handleInput(msg)
	}
	if m.confirm.active {
		return m.handleConfirm(msg)
	}
	if m.showHelp {
		switch msg.String() {
		case "?", "esc", "q", "enter", "ctrl+c":
			m.showHelp = false
		}
		return m, nil
	}

	m.err = "" // any keypress dismisses a stale error/notice

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "ctrl+r":
		return m, m.reloadAll()
	case "esc":
		if m.pane != paneRange {
			m.pane = paneRange
		}
		return m, nil
	case "tab":
		m.pane = (m.pane + 1) % paneCount
		return m, nil
	case "shift+tab":
		m.pane = (m.pane + paneCount - 1) % paneCount
		return m, nil
	case "s":
		// start a new task — global, prompts for a description
		m.input = inputMode{active: true, action: "start", prompt: "start (description)"}
		return m, nil
	}

	// task-targeted mutations, only when a task is selected in a detail pane
	if (m.pane == paneDay || m.pane == paneTimeline) && m.selectedTaskID() != "" {
		if next, cmd, handled := m.keyMutation(msg); handled {
			return next, cmd
		}
	}

	switch m.pane {
	case paneRange:
		return m.keyRange(msg)
	case paneDay:
		return m.keyDay(msg)
	case paneTimeline:
		return m.keyTimeline(msg)
	default:
		return m, nil
	}
}

func (m Model) keyTimeline(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := 0
	if m.show != nil {
		n = len(m.show.Events)
	}
	switch msg.String() {
	case "down", "j":
		if m.tlOffset < n-1 {
			m.tlOffset++
		}
	case "up", "k":
		if m.tlOffset > 0 {
			m.tlOffset--
		}
	case "left", "h":
		m.pane = paneDay
	}
	return m, nil
}

// keyMutation handles mutation keys against the selected task. The bool reports
// whether the key was consumed (so navigation doesn't also see it).
func (m Model) keyMutation(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	id := m.selectedTaskID()
	switch msg.String() {
	case "p":
		next, cmd := m.issueMutation("pause", []string{id})
		return next, cmd, true
	case "r":
		next, cmd := m.issueMutation("resume", []string{id})
		return next, cmd, true
	case "c":
		next, cmd := m.issueMutation("complete", []string{id})
		return next, cmd, true
	case "d":
		next, cmd := m.issueMutation("defer", []string{id})
		return next, cmd, true
	case "a":
		m.input = inputMode{active: true, action: "amend",
			prompt: "amend " + id + " (new description)", taskID: id}
		return m, nil, true
	case "m":
		m.input = inputMode{active: true, action: "move",
			prompt: "move " + id + " (target project; ⇥ completes)", taskID: id}
		return m, nil, true
	case "l":
		m.input = inputMode{active: true, action: "log",
			prompt: "log (note on the running task)"}
		return m, nil, true
	case "x":
		m.confirm = confirmMode{active: true, prompt: "cancel (void) " + id + "?",
			verb: "cancel", valueArgs: []string{id}}
		return m, nil, true
	}
	return m, nil, false
}

// issueMutation runs `wj <verb> <valueArgs...> --date <day>`. For today it runs
// immediately; for any other day it first opens a time prompt (since the CLI
// would otherwise infer the time from the day's last event), so the user gives
// an explicit --at and the action can't collapse to a zero-length interval.
func (m Model) issueMutation(verb string, valueArgs []string) (tea.Model, tea.Cmd) {
	day := m.currentDay()
	args := baseArgs(verb, valueArgs, day)
	if day == m.today || m.today == "" {
		return m, m.mutate(args...)
	}
	m.input = inputMode{active: true, action: "at", pending: args,
		prompt: verb + " on " + day + " — time (e.g. 14:30)"}
	return m, nil
}

// baseArgs assembles `verb <valueArgs...> --date <day>` (pure, for testing).
func baseArgs(verb string, valueArgs []string, day string) []string {
	args := append([]string{verb}, valueArgs...)
	return append(args, "--date", day)
}

// handleInput feeds keystrokes to the active text prompt.
func (m Model) handleInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		in := m.input
		m.input = inputMode{}
		m.err = ""
		val := strings.TrimSpace(in.value)
		switch in.action {
		case "at":
			if val == "" {
				return m, nil // no time -> cancel
			}
			return m, m.mutate(append(in.pending, "--at", val)...)
		case "start":
			if val == "" {
				return m, nil
			}
			return m.issueMutation("start", []string{val})
		case "amend":
			if val == "" {
				return m, nil
			}
			return m.issueMutation("amend", []string{in.taskID, val})
		case "move":
			if val == "" {
				return m, nil
			}
			return m.issueMutation("move", []string{in.taskID, val})
		case "log":
			if val == "" {
				return m, nil
			}
			return m.issueMutation("log", []string{val})
		}
		return m, nil
	case tea.KeyTab:
		// project autocomplete in the move prompt: cycle matches of the prefix
		if m.input.action == "move" {
			if m.input.acPrefix == "" {
				m.input.acPrefix = m.input.value
			}
			m.input.value = m.cycleProject(m.input.acPrefix, m.input.value)
		}
		return m, nil
	case tea.KeyEsc, tea.KeyCtrlC:
		m.input = inputMode{}
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if r := []rune(m.input.value); len(r) > 0 {
			m.input.value = string(r[:len(r)-1])
		}
		m.input.acPrefix = "" // editing restarts autocomplete
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		m.input.value += string(msg.Runes)
		m.input.acPrefix = ""
		return m, nil
	}
	return m, nil
}

// projectMatches returns known projects whose name has prefix (case-insensitive).
func (m Model) projectMatches(prefix string) []string {
	lc := strings.ToLower(prefix)
	var out []string
	for _, p := range m.projects {
		if strings.HasPrefix(strings.ToLower(p), lc) {
			out = append(out, p)
		}
	}
	return out
}

// cycleProject returns the project after cur among those matching prefix (Tab
// behavior); wraps around, and starts at the first match when cur isn't one.
func (m Model) cycleProject(prefix, cur string) string {
	matches := m.projectMatches(prefix)
	if len(matches) == 0 {
		return cur
	}
	for i, p := range matches {
		if p == cur {
			return matches[(i+1)%len(matches)]
		}
	}
	return matches[0]
}

// handleConfirm resolves a y/n destructive-action prompt.
func (m Model) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		c := m.confirm
		m.confirm = confirmMode{}
		m.err = ""
		return m.issueMutation(c.verb, c.valueArgs)
	case "n", "N", "esc", "q":
		m.confirm = confirmMode{}
		return m, nil
	}
	return m, nil
}

func (m Model) keyRange(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "left", "h":
		if m.focusedDay > 0 {
			m.focusedDay--
			return m.afterDayChange()
		}
	case "right", "l":
		if m.g != nil && m.focusedDay < len(m.g.Days)-1 {
			m.focusedDay++
			return m.afterDayChange()
		}
	case "up", "k":
		if m.focusedRow > 0 {
			m.focusedRow--
		}
	case "down", "j":
		if m.g != nil && m.focusedRow < len(m.g.Rows)-1 {
			m.focusedRow++
		}
	case "[":
		return m.shiftRange(-1)
	case "]":
		return m.shiftRange(+1)
	case "b":
		if m.by == "project" {
			m.by = "task"
		} else {
			m.by = "project"
		}
		return m, m.loadGantt()
	case "t":
		// jump focus to today if it's in range, else recenter on a 7-day window
		if m.g != nil {
			if i := indexOf(m.g.Days, m.today); i >= 0 {
				m.focusedDay = i
				return m.afterDayChange()
			}
		}
		return m.setSpan(7)
	case "1":
		return m.setSpan(1)
	case "7":
		return m.setSpan(7)
	case "3":
		return m.setSpan(30)
	case "enter":
		m.pane = paneDay
	}
	return m, nil
}

// setSpan resets the date window to the last n days ending today, and refocuses
// today.
func (m Model) setSpan(n int) (tea.Model, tea.Cmd) {
	if m.today == "" {
		return m, nil
	}
	t, err := time.Parse(dateLayout, m.today)
	if err != nil {
		return m, nil
	}
	m.to = m.today
	m.from = t.AddDate(0, 0, -(n - 1)).Format(dateLayout)
	m.focusInit = false // re-default focus to today (the last day) after reload
	return m, m.loadGantt()
}

func (m Model) keyDay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.selTask > 0 {
			m.selTask--
			return m, m.loadShow(m.selectedTaskID(), m.currentDay())
		}
	case "down", "j":
		if m.grid != nil && m.selTask < len(m.grid.Tasks)-1 {
			m.selTask++
			return m, m.loadShow(m.selectedTaskID(), m.currentDay())
		}
	case "left", "h":
		m.pane = paneRange
	case "enter":
		m.pane = paneTimeline
	}
	return m, nil
}

// afterDayChange resets the task selection and reloads the drill-down panels.
func (m Model) afterDayChange() (tea.Model, tea.Cmd) {
	m.selTask = 0
	m.grid = nil
	m.show = nil
	return m, m.loadGrid(m.currentDay())
}

// shiftRange slides the window by whole windows (dir = -1 earlier, +1 later).
func (m Model) shiftRange(dir int) (tea.Model, tea.Cmd) {
	if m.from == "" || m.to == "" {
		return m, nil
	}
	from, err1 := time.Parse(dateLayout, m.from)
	to, err2 := time.Parse(dateLayout, m.to)
	if err1 != nil || err2 != nil {
		return m, nil
	}
	span := int(to.Sub(from).Hours()/24) + 1
	if span < 1 {
		span = 1
	}
	m.from = from.AddDate(0, 0, dir*span).Format(dateLayout)
	m.to = to.AddDate(0, 0, dir*span).Format(dateLayout)
	return m, m.loadGantt()
}

// View ------------------------------------------------------------------------

func (m Model) View() string {
	if !m.ready {
		return "loading…"
	}
	w := m.width
	if w < 20 {
		w = 80
	}
	innerW := w - 4 // box = border(2) + padding(2) + content
	if innerW < 16 {
		innerW = 16
	}

	title := titleStyle.Render(truncate(fmt.Sprintf("wj — %s .. %s · by %s", m.from, m.to, m.by), w))
	title += "  " + m.runningHeader()

	if m.showHelp {
		return title + "\n" +
			panel("Help", m.helpOverlay(), true, w) + "\n" +
			footerStyle.Render("press ? or esc to close")
	}

	rMax, dMax, tMax := m.bodyBudgets()

	rangeBody := m.renderRange(innerW, rMax)
	if m.g != nil && len(m.g.Rows) == 0 {
		rangeBody = dimStyle.Render("(no tracked time in this range)")
	}

	panels := lipgloss.JoinVertical(lipgloss.Left,
		panel("Range", rangeBody, m.pane == paneRange, w),
		panel("Day — "+m.currentDay(), m.renderDay(innerW, dMax), m.pane == paneDay, w),
		panel("Timeline", m.renderTimeline(tMax), m.pane == paneTimeline, w),
	)

	var foot strings.Builder
	if m.err != "" {
		foot.WriteString(errStyle.Render(truncate("⚠ "+m.err, w)) + "\n")
	}
	switch {
	case m.input.active:
		foot.WriteString(inputStyle.Render(truncate(m.input.prompt+": "+m.input.value+"▏", w)) + "\n")
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
				hint = "⇥ " + strings.Join(ms, " ") + "   [esc] cancel"
			}
		}
		foot.WriteString(footerStyle.Render(truncate(hint, w)))
	case m.confirm.active:
		foot.WriteString(inputStyle.Render(m.confirm.prompt+"  ") + footerStyle.Render("[y/n]"))
	default:
		foot.WriteString(footerStyle.Render(truncate(m.footerLine(), w)))
	}

	return title + "\n" + panels + "\n" + foot.String()
}

// bodyBudgets returns the max body lines for the Range/Day/Timeline panels so
// the three stacked panels fit the terminal height. The focused panel gets the
// most room. A non-positive height (not yet sized / tests) disables windowing.
func (m Model) bodyBudgets() (int, int, int) {
	const big = 1 << 30
	if m.height <= 0 {
		return big, big, big // not yet sized (e.g. tests): never window
	}
	// chrome: title(1) + footer(2) + 3 panels × (2 borders + 1 heading) = 12
	avail := m.height - 12
	if avail < 6 {
		avail = 6 // pathologically short terminal: window hard, may still clip a little
	}
	focused := max2(3, avail/2)
	other := max2(2, (avail-focused)/2)
	switch m.pane {
	case paneRange:
		return focused, other, other
	case paneDay:
		return other, focused, other
	default:
		return other, other, focused
	}
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
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
		out[0] = dimStyle.Render(fmt.Sprintf("  ↑ %d more", start))
	}
	if end < n {
		out[len(out)-1] = dimStyle.Render(fmt.Sprintf("  ↓ %d more", n-end))
	}
	return out
}

// runningHeader shows the currently-running task with a live-ticking elapsed
// time (today's status as of liveAt, plus wall-clock seconds since). Empty when
// nothing is running.
func (m Model) runningHeader() string {
	if m.live == nil || m.liveAt.IsZero() {
		return ""
	}
	for _, t := range m.live.Tasks {
		if t.Status != "in-progress" {
			continue
		}
		mins := t.Minutes + int(time.Since(m.liveAt).Minutes())
		color := ProjectColor(t.Project)
		run := lipgloss.NewStyle().Foreground(color).Render("▶ " + t.ID + " [" + t.Project + "]")
		return run + " " + t.Desc + dimStyle.Render(" · "+fmtDur(mins))
	}
	return dimStyle.Render("◦ idle")
}

// footerLine is a short, context-sensitive hint that fits on one line; the full
// keymap lives in the ? overlay.
func (m Model) footerLine() string {
	switch m.pane {
	case paneRange:
		return "←→ day · ↑↓ row · [ ] shift · t today · 1/7/3 span · b proj/task · enter drill · ? help · q quit"
	case paneDay:
		return "↑↓ task · p/r/c/d/a/m/l/x act · s start · enter timeline · esc back · ? help · q quit"
	default:
		return "esc back · tab pane · s start · ? help · q quit"
	}
}

// helpOverlay is the full keymap, shown when ? is pressed.
func (m Model) helpOverlay() string {
	rows := [][2]string{
		{"~Navigation", ""},
		{"Tab / Shift+Tab", "cycle panels: Range → Day → Timeline"},
		{"← →", "Range: previous / next day"},
		{"↑ ↓ (or k j)", "Range: select project row · Day: select task"},
		{"[ / ]", "shift the date window earlier / later"},
		{"t", "jump to today / recenter the window"},
		{"1 / 7 / 3", "set window span: 1 / 7 / 30 days"},
		{"Enter", "Range: drill into day · Day: open timeline"},
		{"Esc", "return to the Range panel"},
		{"~View", ""},
		{"b", "toggle Range rows between project and task"},
		{"Ctrl+R", "reload everything from disk"},
		{"~Actions (on the selected task)", ""},
		{"s", "start a new task"},
		{"p / r / c / d", "pause / resume / complete / defer"},
		{"a / m", "amend description / move (⇥ completes project)"},
		{"l", "add a note (log) to the running task"},
		{"x", "cancel (void) — asks to confirm"},
		{"", "on a past day, actions prompt for a time first"},
		{"~General", ""},
		{"? ", "toggle this help"},
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
	const totalW = 9 // "  10h01m"
	dw := clamp((innerW-labelW-totalW)/n, 5, 24)
	max := m.maxCell()

	header := strings.Repeat(" ", labelW)
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
		label := padRight(row.Label, labelW)
		if ri == m.focusedRow {
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

	legend := rangeLegend(m.g.Rows)
	reserved := 1 // header
	if legend != "" {
		reserved++
	}
	rows = windowRows(rows, m.focusedRow, maxBody-reserved)

	out := header + "\n" + strings.Join(rows, "\n")
	if legend != "" {
		out += "\n" + legend
	}
	return out
}

// rangeLegend renders a colored swatch per distinct project across the gantt
// rows, so the bar colors are decodable (especially in --by task mode).
func rangeLegend(rows []wj.GanttRow) string {
	seen := map[string]bool{}
	var parts []string
	for _, r := range rows {
		p := rowProject(r)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		sw := lipgloss.NewStyle().Foreground(ProjectColor(p)).Render("█")
		parts = append(parts, sw+" "+p)
	}
	if len(parts) < 2 { // a single project needs no key
		return ""
	}
	return dimStyle.Render("legend: ") + strings.Join(parts, "  ")
}

// renderDay draws the focused day's intraday Gantt: one row per task, a time
// axis from shift_start to shift_end, colored bars for active segments.
func (m Model) renderDay(innerW, maxBody int) string {
	if m.grid == nil {
		return dimStyle.Render("(loading…)")
	}
	if len(m.grid.Tasks) == 0 {
		return dimStyle.Render("(no tasks on this day)")
	}
	start, end := hm(m.grid.ShiftStart), hm(m.grid.ShiftEnd)
	span := end - start
	if span <= 0 {
		span = 1
	}
	const metaW = 9 // " 10h01m" + slack
	axisW := innerW - labelW - metaW
	if axisW < 20 {
		axisW = 20
	}
	col := func(minute int) int { return clamp((minute-start)*axisW/span, 0, axisW-1) }
	gutter := strings.Repeat(" ", labelW)

	// hour-tick axis: a label every two hours across the width
	ticks := []rune(strings.Repeat(" ", axisW))
	for h := (start + 59) / 60 * 60; h <= end; h += 120 {
		lbl := []rune(fmt.Sprintf("%02d", h/60))
		at := col(h)
		for i, r := range lbl {
			if at+i < axisW {
				ticks[at+i] = r
			}
		}
	}
	axis := gutter + dimStyle.Render(string(ticks))

	rows := make([]string, len(m.grid.Tasks))
	for ti, t := range m.grid.Tasks {
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
		label := fmt.Sprintf("%-4s %-12.12s", t.ID, t.Project)
		if ti == m.selTask {
			label = selStyle.Render(padRight(label, labelW))
		} else {
			label = lipgloss.NewStyle().Foreground(color).Render(padRight(label, labelW))
		}
		rows[ti] = label + barStr + " " + fmtDur(t.Minutes)
	}

	// fixed lines: axis + now-marker + legend; window task rows into the rest
	var nowLine, legend string
	if n := hm(m.grid.Now); n >= start && n <= end {
		marker := []rune(strings.Repeat(" ", axisW))
		marker[col(n)] = '▲'
		nowLine = gutter + dimStyle.Render(string(marker))
	}
	legend = m.projectLegend()
	reserved := 1 // axis
	if nowLine != "" {
		reserved++
	}
	if legend != "" {
		reserved++
	}
	rows = windowRows(rows, m.selTask, maxBody-reserved)

	out := axis + "\n" + strings.Join(rows, "\n")
	if nowLine != "" {
		out += "\n" + nowLine
	}
	if legend != "" {
		out += "\n" + legend
	}
	return out
}

// projectLegend renders a colored swatch per distinct project in the focused
// day, so the intraday bar colors are decodable.
func (m Model) projectLegend() string {
	if m.grid == nil {
		return ""
	}
	seen := map[string]bool{}
	var parts []string
	for _, t := range m.grid.Tasks {
		if seen[t.Project] {
			continue
		}
		seen[t.Project] = true
		sw := lipgloss.NewStyle().Foreground(ProjectColor(t.Project)).Render("█")
		parts = append(parts, sw+" "+t.Project)
	}
	if len(parts) == 0 {
		return ""
	}
	return dimStyle.Render("legend: ") + strings.Join(parts, "  ")
}

// renderTimeline lists the selected task's events, scrollable via tlOffset.
func (m Model) renderTimeline(maxBody int) string {
	if m.show == nil {
		return dimStyle.Render("(select a task)")
	}
	s := m.show
	head := lipgloss.NewStyle().Foreground(ProjectColor(s.Project)).
		Render(fmt.Sprintf("%s  [%s]  %s", s.ID, s.Project, s.Desc))
	sub := dimStyle.Render(fmt.Sprintf("%s · %s · %s", s.Date, s.Status, fmtDur(s.Minutes)))

	rows := make([]string, len(s.Events))
	for i, e := range s.Events {
		label, extra := timelineLabel(e)
		rows[i] = fmt.Sprintf("  %s  %-9s  %s", e.Time, label, extra)
	}
	rows = windowRows(rows, m.tlOffset, maxBody-2) // 2 head lines
	return head + "\n" + sub + "\n" + strings.Join(rows, "\n")
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
	case "commit":
		return "commit", e.Note
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
func panel(title, body string, active bool, width int) string {
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	if active {
		st = st.BorderForeground(lipgloss.Color("39"))
	} else {
		st = st.BorderForeground(lipgloss.Color("240"))
	}
	if width > 6 {
		st = st.Width(width - 2)
	}
	heading := titleStyle.Render(title)
	if !active {
		heading = dimStyle.Render(title)
	}
	return st.Render(heading + "\n" + body)
}

// styles
var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	focusStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Underline(true)
	todayStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("84")) // green = today
	selStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("238"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	inputStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
)

// helpers ---------------------------------------------------------------------

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
