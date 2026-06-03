// Package ui implements the wj-tui front-end. The layout is lazygit-style: a
// narrow left sidebar of lists (Projects, then the focused day's Tasks) drives
// a wide main column of visualizations (the range Gantt, the focused day's
// intraday Gantt, and the selected task's Timeline). Selecting a project in the
// sidebar filters the day detail (master→detail). Navigation is vim-style:
// j/k move within a panel, l/h drill in/out, ←/→ step days.
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
	labelW         = 18              // project/task label column width (max)
	refresh        = 1 * time.Second // display tick (running clock)
	dataEveryTicks = 30              // re-shell the CLI for fresh data every N ticks
)

// pane identifies which panel currently has keyboard focus. The drill axis is
// Projects → Tasks → Timeline; the sidebar holds Projects/Tasks, the main
// column reflects the selection.
type pane int

const (
	paneRange    pane = iota // sidebar: Projects list (navigate project rows)
	paneDay                  // sidebar: the focused day's Tasks list
	paneTimeline             // the selected task's event timeline
	panePending              // sidebar: the not-yet-started backlog
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

// searchMsg carries the results of a global task search. The query is echoed
// back so a result for a query the user has since edited can be discarded.
type searchMsg struct {
	query   string
	results []wj.Found
	err     error
}

// pendingMsg carries the current backlog of not-yet-started tasks.
type pendingMsg struct {
	items []wj.Pending
	err   error
}

// mutationMsg is the result of a state-changing `wj` invocation. note carries
// the CLI's confirmation line (e.g. "T1  already completed") so the UI can
// echo it back as feedback.
type mutationMsg struct {
	note string
	err  error
}

// inputMode is an inline text prompt. Two flavors:
//   - value entry for start/amend/move (action = the verb), then issued via
//     issueMutation (which may chain into a time prompt on a past day);
//   - time entry (action = "at") for a mutation on a non-today day, carrying the
//     fully-built argv in `pending`, to which "--at <value>" is appended.
type inputMode struct {
	active   bool
	action   string // "start" | "amend" | "move" | "log" | "at" (at = run pending argv with --at <value>)
	prompt   string
	value    string
	taskID   string   // target task for amend/move
	pending  []string // for action "at": the wj argv awaiting an --at suffix
	acPrefix string   // move autocomplete: the prefix Tab cycles matches for
}

// searchMode is the global task-search overlay (opened with /). The query is
// re-run against the CLI on every edit; results carry the day + id needed to
// jump straight to a task.
type searchMode struct {
	active  bool
	query   string
	results []wj.Found
	sel     int
}

// confirmMode is a y/n guard for destructive mutations (cancel, drop).
type confirmMode struct {
	active    bool
	prompt    string
	verb      string   // wj verb to run on confirmation
	valueArgs []string // args between the verb and --date (e.g. the task id)
	raw       bool     // run as a plain mutate (no --date), e.g. for backlog drop
	atTime    bool     // after confirming, prompt for an explicit --at time
}

// Model is the root Bubble Tea model.
type Model struct {
	cli wj.Client

	today    string // YYYY-MM-DD; mutations on other days require an explicit --at
	from, to string // current range (YYYY-MM-DD); empty until first load
	by       string // "project" | "task"

	g          *wj.Gantt
	focusedDay int // index into g.Days
	focusedRow int // sidebar Projects index: 0 = "All", i = g.Rows[i-1]

	grid    *wj.Grid // intraday data for the focused day
	selTask int      // index into filteredTasks()

	show *wj.Show // timeline of the selected task

	pane          pane
	input         inputMode
	confirm       confirmMode
	search        searchMode
	jumpTaskID    string // a search result to select once its day's grid loads
	showHelp      bool
	err           string
	notice        string // transient confirmation/no-op feedback from a mutation
	width, height int
	ready         bool
	focusInit     bool // whether focusedDay has been defaulted yet

	pending []wj.Pending // the not-yet-started backlog (its own panel)
	selPend int          // index into pending

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
	return tea.Batch(m.loadGantt(), m.loadLive(), m.loadProjects(), m.loadPending(), tickCmd())
}

// currentDay is the YYYY-MM-DD of the focused day column ("" if none).
func (m Model) currentDay() string {
	if m.g == nil || m.focusedDay < 0 || m.focusedDay >= len(m.g.Days) {
		return ""
	}
	return m.g.Days[m.focusedDay]
}

// projectFilter is the project selected in the sidebar ("" = All / no filter).
func (m Model) projectFilter() string {
	if m.g == nil || m.focusedRow <= 0 || m.focusedRow-1 >= len(m.g.Rows) {
		return ""
	}
	return rowProject(m.g.Rows[m.focusedRow-1])
}

// filteredTasks is the focused day's tasks restricted to the selected project
// (all tasks when no project is selected).
func (m Model) filteredTasks() []wj.GridTask {
	if m.grid == nil {
		return nil
	}
	f := m.projectFilter()
	if f == "" {
		return m.grid.Tasks
	}
	out := make([]wj.GridTask, 0, len(m.grid.Tasks))
	for _, t := range m.grid.Tasks {
		if t.Project == f {
			out = append(out, t)
		}
	}
	return out
}

// selectedTaskID is the id of the selected (filtered) task ("" if none).
func (m Model) selectedTaskID() string {
	ts := m.filteredTasks()
	if m.selTask < 0 || m.selTask >= len(ts) {
		return ""
	}
	return ts[m.selTask].ID
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

func (m Model) loadPending() tea.Cmd {
	cli := m.cli
	return func() tea.Msg {
		items, err := cli.Pending()
		return pendingMsg{items: items, err: err}
	}
}

func (m Model) runSearch(query string) tea.Cmd {
	cli := m.cli
	return func() tea.Msg {
		res, err := cli.Search(query)
		return searchMsg{query: query, results: res, err: err}
	}
}

func tickCmd() tea.Cmd {
	return tea.Every(refresh, func(time.Time) tea.Msg { return tickMsg{} })
}

// mutate runs a state-changing `wj` command asynchronously.
func (m Model) mutate(args ...string) tea.Cmd {
	cli := m.cli
	return func() tea.Msg {
		note, err := cli.Mutate(args...)
		return mutationMsg{note: note, err: err}
	}
}

// reloadAll refreshes every panel from the CLI (used after a mutation).
func (m Model) reloadAll() tea.Cmd {
	cmds := []tea.Cmd{m.loadGantt(), m.loadGrid(m.currentDay()),
		m.loadShow(m.selectedTaskID(), m.currentDay()), m.loadLive(), m.loadPending()}
	if m.search.active { // keep an open search overlay in sync with mutations
		cmds = append(cmds, m.runSearch(m.search.query))
	}
	return tea.Batch(cmds...)
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
				m.loadShow(m.selectedTaskID(), m.currentDay()), m.loadLive(), m.loadPending())
			if m.search.active {
				cmds = append(cmds, m.runSearch(m.search.query))
			}
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

	case pendingMsg:
		if msg.err == nil {
			m.pending = msg.items
			m.selPend = clamp(m.selPend, 0, len(m.pending)-1)
		}
		return m, nil

	case searchMsg:
		if !m.search.active || msg.query != m.search.query {
			return m, nil // stale (overlay closed or query moved on)
		}
		if msg.err == nil {
			m.search.results = msg.results
			m.search.sel = clamp(m.search.sel, 0, len(msg.results)-1)
		}
		return m, nil

	case ganttMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		// NB: do not clear m.err here — a background reload must not erase a
		// mutation/load error before the user has seen it (cleared on keypress).
		prevProj := m.projectFilter() // remember the selected project (old rows)
		m.g = msg.g
		m.from, m.to = msg.g.From, msg.g.To
		if !m.focusInit && len(m.g.Days) > 0 {
			m.focusedDay = len(m.g.Days) - 1 // start on the most recent day (today)
			m.focusInit = true
		}
		m.focusedDay = clamp(m.focusedDay, 0, len(m.g.Days)-1)
		// follow the selection by project identity, not raw index, since the row
		// set can be reordered/re-keyed across a reload (or a by project↔task flip).
		if prevProj != "" {
			for i, r := range m.g.Rows {
				if rowProject(r) == prevProj {
					m.focusedRow = i + 1
					break
				}
			}
		}
		m.focusedRow = clamp(m.focusedRow, 0, len(m.g.Rows)) // 0 = All, len = last row
		return m, m.loadGrid(m.currentDay())                 // refresh the drill-down too

	case gridMsg:
		if msg.day != m.currentDay() {
			return m, nil // stale
		}
		if msg.err != nil {
			m.err = msg.err.Error()
			m.jumpTaskID = "" // abandon any pending search-jump rather than strand it
			return m, nil
		}
		m.grid = msg.g
		if m.jumpTaskID != "" { // a search jump is landing on this day
			for i, t := range m.grid.Tasks {
				if t.ID == m.jumpTaskID {
					m.selTask = i
					break
				}
			}
			m.jumpTaskID = ""
		}
		m.selTask = clamp(m.selTask, 0, len(m.filteredTasks())-1)
		if m.selectedTaskID() == "" {
			m.show = nil // day has no (matching) tasks — drop any stale timeline
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
			m.notice = ""
		} else {
			m.err = ""
			// echo the CLI's confirmation line (incl. idempotent no-ops like
			// "T1  already completed") so a re-click still gives feedback.
			// Collapse whitespace so a multi-line reply (e.g. complete's
			// "… completed\n  N commit(s) recorded") stays a single footer line.
			m.notice = strings.Join(strings.Fields(msg.note), " ")
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
	if m.search.active {
		return m.handleSearch(msg)
	}
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

	m.err = ""    // any keypress dismisses a stale error/notice
	m.notice = "" // ...and a stale mutation confirmation

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "/":
		m.search = searchMode{active: true}
		return m, m.runSearch("") // prime with everything (most recent first)
	case "ctrl+r":
		return m, m.reloadAll()
	case "left":
		return m.stepDay(-1) // ←/→ step the focused day from any panel
	case "right":
		return m.stepDay(+1)
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
		// start a new task — global. Description plus an optional inline
		// "@project" (⇥ cycles known projects, like the add/move prompts).
		m.input = inputMode{active: true, action: "start",
			prompt: "start: desc  (optional @project ⇥completes  %time, e.g. %9:30)"}
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
	case panePending:
		return m.keyPending(msg)
	default:
		return m, nil
	}
}

// selectedPendID is the id of the highlighted pending task ("" if none).
func (m Model) selectedPendID() string {
	if m.selPend < 0 || m.selPend >= len(m.pending) {
		return ""
	}
	return m.pending[m.selPend].ID
}

// keyPending drives the backlog panel: navigate, promote (start), add, set due,
// reorder, and drop. Add/due open the inline prompt; drop asks to confirm.
func (m Model) keyPending(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.pending)
	switch msg.String() {
	case "up", "k":
		if m.selPend > 0 {
			m.selPend--
		}
	case "down", "j":
		if m.selPend < n-1 {
			m.selPend++
		}
	case "g":
		m.selPend = 0
	case "G":
		if n > 0 {
			m.selPend = n - 1
		}
	case "h":
		m.pane = paneRange
	case "enter": // promote the selected backlog item into a tracked task
		if id := m.selectedPendID(); id != "" {
			return m, m.mutate("start", id)
		}
	case "a": // add a new pending task (inline @project / !due optional)
		m.input = inputMode{active: true, action: "add",
			prompt: "add pending: desc  (optional @project  !YYYY-MM-DD)"}
	case "d": // set / clear its deadline
		if id := m.selectedPendID(); id != "" {
			m.input = inputMode{active: true, action: "pdue", taskID: id,
				prompt: "due " + id + " (YYYY-MM-DD; empty clears)"}
		}
	case "x": // drop without starting (guarded)
		if id := m.selectedPendID(); id != "" {
			m.confirm = confirmMode{active: true, prompt: "drop pending " + id + "?",
				verb: "drop", valueArgs: []string{id}, raw: true}
		}
	case "[": // raise one step (and follow it)
		if id := m.selectedPendID(); id != "" {
			if m.selPend > 0 {
				m.selPend--
			}
			return m, m.mutate("raise", id)
		}
	case "]": // lower one step (and follow it)
		if id := m.selectedPendID(); id != "" {
			if m.selPend < n-1 {
				m.selPend++
			}
			return m, m.mutate("lower", id)
		}
	}
	return m, nil
}

// stepDay moves the focused day by dir (clamped), reloading the drill-down.
func (m Model) stepDay(dir int) (tea.Model, tea.Cmd) {
	if m.g == nil {
		return m, nil
	}
	nd := m.focusedDay + dir
	if nd < 0 || nd >= len(m.g.Days) {
		return m, nil
	}
	m.focusedDay = nd
	return m.afterDayChange()
}

// pageStep is the half-page jump for Ctrl+d / Ctrl+u, scaled to the terminal.
func (m Model) pageStep() int {
	if m.height > 10 {
		return (m.height - 10) / 2
	}
	return 4
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
	case "g":
		m.tlOffset = 0
	case "G":
		if n > 0 {
			m.tlOffset = n - 1
		}
	case "ctrl+d":
		m.tlOffset = clamp(m.tlOffset+m.pageStep(), 0, max2(0, n-1))
	case "ctrl+u":
		m.tlOffset = clamp(m.tlOffset-m.pageStep(), 0, max2(0, n-1))
	case "h":
		m.pane = paneDay
	}
	return m, nil
}

// keyMutation handles mutation keys against the selected task. The bool reports
// whether the key was consumed (so navigation doesn't also see it). Note: log
// is bound to "n" (note) so that "l" stays free for drill-in.
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
	// Shift+key = same action at an explicit --at time (a time prompt, then run).
	case "P":
		next, cmd := m.promptTimedMutation("pause", []string{id})
		return next, cmd, true
	case "R":
		next, cmd := m.promptTimedMutation("resume", []string{id})
		return next, cmd, true
	case "C":
		next, cmd := m.promptTimedMutation("complete", []string{id})
		return next, cmd, true
	case "D":
		next, cmd := m.promptTimedMutation("defer", []string{id})
		return next, cmd, true
	case "X": // timed void: confirm first (it's destructive), then prompt for time
		m.confirm = confirmMode{active: true, prompt: "cancel (void) " + id + " at a time?",
			verb: "cancel", valueArgs: []string{id}, atTime: true}
		return m, nil, true
	case "a":
		m.input = inputMode{active: true, action: "amend",
			prompt: "amend " + id + " (new description)", taskID: id}
		return m, nil, true
	case "m":
		m.input = inputMode{active: true, action: "move",
			prompt: "move " + id + " (target project; ⇥ completes)", taskID: id}
		return m, nil, true
	case "n":
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
	if day == m.today || m.today == "" || hasFlag(valueArgs, "--at") {
		return m, m.mutate(args...)
	}
	m.input = inputMode{active: true, action: "at", pending: args,
		prompt: verb + " on " + day + " — time (e.g. 14:30)"}
	return m, nil
}

// promptTimedMutation opens the time prompt for a Shift-key action so the user
// can run verb at an explicit --at time instead of "now". It reuses the "at"
// input handler (which appends --at <value> and runs). Works on any day, since
// baseArgs carries the --date; a blank time cancels.
func (m Model) promptTimedMutation(verb string, valueArgs []string) (tea.Model, tea.Cmd) {
	day := m.currentDay()
	label := strings.TrimSpace(verb + " " + strings.Join(valueArgs, " "))
	m.input = inputMode{active: true, action: "at", pending: baseArgs(verb, valueArgs, day),
		prompt: label + " — time (e.g. 14:30)"}
	return m, nil
}

// baseArgs assembles `verb <valueArgs...> --date <day>` (pure, for testing).
func baseArgs(verb string, valueArgs []string, day string) []string {
	args := append([]string{verb}, valueArgs...)
	return append(args, "--date", day)
}

// hasFlag reports whether args already contains flag (e.g. "--at"), so a
// caller-supplied value isn't re-prompted for.
func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
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
			desc, proj, at := parseStartInput(val)
			if desc == "" {
				return m, nil
			}
			args := []string{desc}
			if proj != "" {
				args = append(args, "--project", proj)
			}
			if at != "" {
				args = append(args, "--at", at)
			}
			return m.issueMutation("start", args)
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
		case "add": // new pending backlog task (not a dated mutation)
			desc, proj, due := parsePendingInput(val)
			if desc == "" {
				return m, nil
			}
			args := []string{"add", desc}
			if proj != "" {
				args = append(args, "--project", proj)
			}
			if due != "" {
				args = append(args, "--due", due)
			}
			return m, m.mutate(args...)
		case "pdue": // set or clear a pending task's deadline
			d := val
			if d == "" {
				d = "-"
			}
			return m, m.mutate("due", in.taskID, d)
		}
		return m, nil
	case tea.KeyTab:
		switch m.input.action {
		case "move":
			// the whole value is the project name
			if m.input.acPrefix == "" {
				m.input.acPrefix = m.input.value
			}
			m.input.value = m.cycleProject(m.input.acPrefix, m.input.value)
		case "start":
			// only the trailing "@token" is a project; the rest is the desc
			if at := strings.LastIndexByte(m.input.value, '@'); at >= 0 {
				head, proj := m.input.value[:at+1], m.input.value[at+1:]
				if m.input.acPrefix == "" {
					m.input.acPrefix = proj
				}
				m.input.value = head + m.cycleProject(m.input.acPrefix, proj)
			}
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

// handleSearch drives the global search overlay: runes edit the query (which
// re-runs the search), ↑/↓ (or Ctrl+p/Ctrl+n) move the selection, Enter jumps
// to the highlighted task, Esc closes.
func (m Model) handleSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		return m.jumpToResult()
	case tea.KeyEsc, tea.KeyCtrlC:
		m.search = searchMode{}
		return m, nil
	case tea.KeyUp:
		if m.search.sel > 0 {
			m.search.sel--
		}
		return m, nil
	case tea.KeyDown:
		if m.search.sel < len(m.search.results)-1 {
			m.search.sel++
		}
		return m, nil
	case tea.KeyBackspace, tea.KeyDelete:
		if r := []rune(m.search.query); len(r) > 0 {
			m.search.query = string(r[:len(r)-1])
			m.search.sel = 0
			return m, m.runSearch(m.search.query)
		}
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		m.search.query += string(msg.Runes)
		m.search.sel = 0
		return m, m.runSearch(m.search.query)
	}
	switch msg.String() {
	case "ctrl+n":
		if m.search.sel < len(m.search.results)-1 {
			m.search.sel++
		}
	case "ctrl+p":
		if m.search.sel > 0 {
			m.search.sel--
		}
	}
	return m, nil
}

// jumpToResult closes the overlay and navigates to the selected hit: it windows
// the range so the task's day is focused, clears the project filter, and marks
// the task to be selected once that day's grid loads.
func (m Model) jumpToResult() (tea.Model, tea.Cmd) {
	if m.search.sel < 0 || m.search.sel >= len(m.search.results) {
		m.search = searchMode{}
		return m, nil
	}
	r := m.search.results[m.search.sel]
	m.search = searchMode{}
	if t, err := time.Parse(dateLayout, r.Date); err == nil {
		m.from = t.AddDate(0, 0, -6).Format(dateLayout) // a 7-day window ending on the hit
		m.to = r.Date
	}
	m.focusInit = false // re-default focus to the last day (the hit's day)
	m.focusedRow = 0    // clear any project filter so the task is visible
	m.jumpTaskID = r.ID
	m.pane = paneTimeline
	return m, m.loadGantt()
}

// handleConfirm resolves a y/n destructive-action prompt.
func (m Model) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		c := m.confirm
		m.confirm = confirmMode{}
		m.err = ""
		if c.raw { // backlog ops aren't dated task mutations — run them plainly
			return m, m.mutate(append([]string{c.verb}, c.valueArgs...)...)
		}
		if c.atTime { // Shift+X: confirmed, now ask for the explicit --at time
			return m.promptTimedMutation(c.verb, c.valueArgs)
		}
		return m.issueMutation(c.verb, c.valueArgs)
	case "n", "N", "esc", "q":
		m.confirm = confirmMode{}
		return m, nil
	}
	return m, nil
}

func (m Model) keyRange(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := 0
	if m.g != nil {
		rows = len(m.g.Rows)
	}
	switch msg.String() {
	case "left":
		return m.stepDay(-1)
	case "right":
		return m.stepDay(+1)
	case "up", "k":
		if m.focusedRow > 0 {
			m.focusedRow--
			return m.afterProjectChange()
		}
	case "down", "j":
		if m.focusedRow < rows {
			m.focusedRow++
			return m.afterProjectChange()
		}
	case "g":
		m.focusedRow = 0
		return m.afterProjectChange()
	case "G":
		m.focusedRow = rows
		return m.afterProjectChange()
	case "ctrl+d":
		m.focusedRow = clamp(m.focusedRow+m.pageStep(), 0, rows)
		return m.afterProjectChange()
	case "ctrl+u":
		m.focusedRow = clamp(m.focusedRow-m.pageStep(), 0, rows)
		return m.afterProjectChange()
	case "l", "enter":
		m.pane = paneDay
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
		m.focusedRow = 0 // row set changes; reset to All
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
	n := len(m.filteredTasks())
	switch msg.String() {
	case "up", "k":
		if m.selTask > 0 {
			m.selTask--
			return m, m.loadShow(m.selectedTaskID(), m.currentDay())
		}
	case "down", "j":
		if m.selTask < n-1 {
			m.selTask++
			return m, m.loadShow(m.selectedTaskID(), m.currentDay())
		}
	case "g":
		m.selTask = 0
		return m, m.loadShow(m.selectedTaskID(), m.currentDay())
	case "G":
		if n > 0 {
			m.selTask = n - 1
			return m, m.loadShow(m.selectedTaskID(), m.currentDay())
		}
	case "ctrl+d":
		m.selTask = clamp(m.selTask+m.pageStep(), 0, n-1)
		return m, m.loadShow(m.selectedTaskID(), m.currentDay())
	case "ctrl+u":
		m.selTask = clamp(m.selTask-m.pageStep(), 0, n-1)
		return m, m.loadShow(m.selectedTaskID(), m.currentDay())
	case "h":
		m.pane = paneRange
	case "l", "enter":
		m.pane = paneTimeline
	}
	return m, nil
}

// afterDayChange resets the task selection and reloads the drill-down panels.
// The project filter (focusedRow) is preserved across days.
func (m Model) afterDayChange() (tea.Model, tea.Cmd) {
	m.selTask = 0
	m.grid = nil
	m.show = nil
	return m, m.loadGrid(m.currentDay())
}

// afterProjectChange re-selects the first matching task after the sidebar
// project (the master→detail filter) changes, reloading its timeline.
func (m Model) afterProjectChange() (tea.Model, tea.Cmd) {
	m.selTask = 0
	if m.selectedTaskID() == "" {
		m.show = nil
	}
	return m, m.loadShow(m.selectedTaskID(), m.currentDay())
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

	header := m.renderHeader(w)

	if m.showHelp {
		return header + "\n" +
			panel("Help", m.helpOverlay(), true, w, 0) + "\n" +
			footerStyle.Render("press ? or esc to close")
	}

	if m.search.active {
		return header + "\n" +
			panel("Search", m.searchOverlay(w), true, w, 0) + "\n" +
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

	sideW := clamp(w*24/100, 22, 32)
	if sideW > w-24 {
		sideW = w - 24
	}
	if sideW < 12 {
		sideW = 12
	}
	mainW := w - sideW

	body := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderSidebar(sideW, bodyH, fill),
		m.renderMain(mainW, bodyH, fill),
	)

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
		b.WriteString(errStyle.Render(truncate("⚠ "+m.err, w)) + "\n")
	} else if m.notice != "" {
		b.WriteString(noticeStyle.Render(truncate("✓ "+m.notice, w)) + "\n")
	}
	switch {
	case m.input.active:
		b.WriteString(inputStyle.Render(truncate(m.input.prompt+": "+m.input.value+"▏", w)) + "\n")
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
		b.WriteString(footerStyle.Render(truncate(hint, w)))
	case m.confirm.active:
		b.WriteString(inputStyle.Render(m.confirm.prompt+"  ") + footerStyle.Render("[y/n]"))
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
			panel("Projects", m.renderProjects(cw, 1<<30), m.pane == paneRange, w, 0),
			panel(taskTitle, m.renderTasks(cw, 1<<30), m.pane == paneDay, w, 0),
			panel(pendTitle, m.renderPending(cw, 1<<30), m.pane == panePending, w, 0),
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
	hs := sidebarSplit(h, fi)
	return lipgloss.JoinVertical(lipgloss.Left,
		panel("Projects", m.renderProjects(cw, hs[0]-3), m.pane == paneRange, w, hs[0]),
		panel(taskTitle, m.renderTasks(cw, hs[1]-3), m.pane == paneDay, w, hs[1]),
		panel(pendTitle, m.renderPending(cw, hs[2]-3), m.pane == panePending, w, hs[2]),
	)
}

// sidebarSplit divides the sidebar height among Projects/Tasks/Pending. The
// focused panel (0/1/2) gets the most; focused == -1 splits into equal thirds.
func sidebarSplit(h, focused int) [3]int {
	if focused < 0 || h < 12 {
		a := h / 3
		return [3]int{a, a, h - 2*a}
	}
	return split3(h, focused)
}

// renderPending lists the backlog: a deadline-urgency glyph + the description
// (project-colored when set), with the due date right-aligned.
func (m Model) renderPending(cw, maxRows int) string {
	if len(m.pending) == 0 {
		return dimStyle.Render("(empty — press a to add)")
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

// parsePendingInput splits the add-prompt text into a description plus optional
// inline tokens: "@project" sets the project, "!YYYY-MM-DD" the deadline.
// e.g. "Fix invoice @acme !2026-06-10" → ("Fix invoice", "acme", "2026-06-10").
func parsePendingInput(s string) (desc, project, due string) {
	var words []string
	for _, f := range strings.Fields(s) {
		switch {
		case len(f) > 1 && f[0] == '@':
			project = f[1:]
		case len(f) > 1 && f[0] == '!':
			due = f[1:]
		default:
			words = append(words, f)
		}
	}
	return strings.Join(words, " "), project, due
}

// parseStartInput splits the start-prompt text into a description plus optional
// inline tokens: "@project" sets the project, "%time" the start time (passed to
// the CLI as --at; blank → the CLI defaults to now). The last @/%token wins.
// Unlike parsePendingInput it has no "!due" notion, so a "!"-word stays part of
// the description.
// e.g. "Fix login bug @backend %9:30" → ("Fix login bug", "backend", "9:30").
func parseStartInput(s string) (desc, project, at string) {
	var words []string
	for _, f := range strings.Fields(s) {
		switch {
		case len(f) > 1 && f[0] == '@':
			project = f[1:]
		case len(f) > 1 && f[0] == '%':
			at = f[1:]
		default:
			words = append(words, f)
		}
	}
	return strings.Join(words, " "), project, at
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
	tlTitle := "Timeline"
	if m.show != nil {
		tlTitle = "Timeline · " + m.show.ID
	}
	if !fill {
		return lipgloss.JoinVertical(lipgloss.Left,
			panel("Range", m.rangeBody(innerW, 1<<30), m.pane == paneRange, w, 0),
			panel(dayTitle, m.renderDay(innerW, 1<<30), m.pane == paneDay, w, 0),
			panel(tlTitle, m.renderTimeline(1<<30), m.pane == paneTimeline, w, 0),
		)
	}
	hs := split3(h, int(m.pane))
	return lipgloss.JoinVertical(lipgloss.Left,
		panel("Range", m.rangeBody(innerW, hs[0]-3), m.pane == paneRange, w, hs[0]),
		panel(dayTitle, m.renderDay(innerW, hs[1]-3), m.pane == paneDay, w, hs[1]),
		panel(tlTitle, m.renderTimeline(hs[2]-3), m.pane == paneTimeline, w, hs[2]),
	)
}

// rangeBody renders the Range gantt, or a placeholder when the range is empty.
func (m Model) rangeBody(innerW, maxBody int) string {
	if m.g != nil && len(m.g.Rows) == 0 {
		return dimStyle.Render("(no tracked time in this range)")
	}
	return m.renderRange(innerW, maxBody)
}

// split2 divides a column height between two stacked panels; the focused one
// gets the larger share. The two always sum to h.
func split2(h int, firstFocused bool) (int, int) {
	if h < 8 {
		a := h / 2
		return a, h - a
	}
	f := h * 11 / 20
	s := h - f
	if firstFocused {
		return f, s
	}
	return s, f
}

// split3 divides a column height among three stacked panels; the focused one
// (by index) gets roughly half. The three always sum to h.
func split3(h, focused int) [3]int {
	if h < 12 {
		a := h / 3
		return [3]int{a, a, h - 2*a}
	}
	f := h / 2
	rest := h - f
	a := rest / 2
	b := rest - a
	switch focused {
	case 0:
		return [3]int{f, a, b}
	case 1:
		return [3]int{a, f, b}
	default:
		return [3]int{a, b, f}
	}
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
		return "j/k project · l drill · ←→ day · [ ] window · 1/7/3 span · b by · / search · s start · ? help · q quit"
	case paneDay:
		return "j/k task · l timeline · h back · p/r/c/d pause/resume/done/defer (⇧=at time) · a/m/n amend/move/note · / search · ? help"
	case panePending:
		return "j/k pick · enter start · a add · d due · [ ] reorder · x drop · h back · ? help"
	default:
		return "j/k scroll · ^d/^u page · h back · / search · s start · ? help · q quit"
	}
}

// searchOverlay renders the search prompt and its results list (one task per
// row: status glyph, id, description, project, day, duration), windowed to fit.
func (m Model) searchOverlay(w int) string {
	cw := w - 4 // panel content width
	var b strings.Builder
	b.WriteString(inputStyle.Render("/"+m.search.query+"▏") + "\n")
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
		{"h / l", "drill out / in: Projects → Tasks → Timeline"},
		{"j / k", "move the selection in the focused panel"},
		{"g / G", "jump to first / last"},
		{"Ctrl+d / Ctrl+u", "half-page down / up"},
		{"← / →", "previous / next day (from any panel)"},
		{"Tab / Shift+Tab", "cycle panels: Projects → Tasks → Timeline → Pending"},
		{"Enter", "drill in (alias for l) · Esc returns to Projects"},
		{"[ / ]", "shift the date window earlier / later"},
		{"t", "jump to today / recenter the window"},
		{"1 / 7 / 3", "set window span: 1 / 7 / 30 days"},
		{"~View", ""},
		{"/", "search all tasks (id / project / description); Enter jumps"},
		{"b", "toggle the Projects rows between project and task"},
		{"", "selecting a project filters the day's Tasks"},
		{"Ctrl+R", "reload everything from disk"},
		{"~Pending (backlog panel)", ""},
		{"a", "add: 'desc @project !YYYY-MM-DD' (project/due optional)"},
		{"d", "set / clear the selected task's deadline"},
		{"Enter", "start (promote) the selected pending task"},
		{"[ / ]", "move it up / down · x drop it"},
		{"~Actions (on the selected task)", ""},
		{"s", "start a new task: 'desc @project %time' (both optional; ⇥ completes)"},
		{"p / r / c / d", "pause / resume / complete / defer (now)"},
		{"P / R / C / D", "same, but prompt for an explicit time (--at)"},
		{"a / m", "amend description / move (⇥ completes project)"},
		{"n", "add a note (log) to the running task"},
		{"x / X", "cancel (void) — asks to confirm · X also prompts for a time"},
		{"", "on a past day, actions prompt for a time first"},
		{"~General", ""},
		{"?", "toggle this help"},
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

// renderProjects is the sidebar list: an "All" entry plus one row per gantt
// row (project or task), each showing its total. The selected row is the
// master→detail filter.
func (m Model) renderProjects(cw, maxRows int) string {
	if m.g == nil {
		return dimStyle.Render("(loading…)")
	}
	if len(m.g.Rows) == 0 {
		return dimStyle.Render("(no tracked time)")
	}
	total := 0
	for _, r := range m.g.Rows {
		total += r.TotalMinutes
	}
	active := m.activeProject()
	items := make([]string, 0, len(m.g.Rows)+1)
	items = append(items, listLine(" ", lipgloss.Color("244"), lipgloss.Color("250"),
		"All", fmtDur(total), m.focusedRow == 0, cw))
	for i, r := range m.g.Rows {
		p := rowProject(r)
		glyph, gc := " ", lipgloss.Color("78")
		if p != "" && p == active {
			glyph = ">" // this project has the running task
		}
		items = append(items, listLine(glyph, gc, ProjectColor(p),
			r.Label, fmtDur(r.TotalMinutes), m.focusedRow == i+1, cw))
	}
	items = windowRows(items, m.focusedRow, maxRows)
	return strings.Join(items, "\n")
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
		label := t.ID
		if t.Desc != "" {
			label = t.ID + " " + t.Desc
		} else {
			label = t.ID + " " + t.Project
		}
		glyph, gc := statusGlyph(t.Status)
		items[i] = listLine(glyph, gc, ProjectColor(t.Project), label, fmtDur(t.Minutes), i == m.selTask, cw)
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
	l := padRight(left, leftMax)
	if selected {
		return selStyle.Render(padRight(glyph+" "+l+" "+right, cw))
	}
	return lipgloss.NewStyle().Foreground(glyphColor).Render(glyph) + " " +
		lipgloss.NewStyle().Foreground(labelColor).Render(l) + " " + dimStyle.Render(right)
}

// statusGlyph maps a task status to its ASCII glyph and accent color. The same
// glyph set is used in the task lists, the header rollup, and the legend key.
func statusGlyph(status string) (string, lipgloss.Color) {
	switch status {
	case "in-progress":
		return ">", lipgloss.Color("78") // green — running now
	case "paused":
		return "=", lipgloss.Color("214") // amber
	case "deferred":
		return "»", lipgloss.Color("39") // blue
	case "completed":
		return "x", lipgloss.Color("244") // dim — done
	case "cancelled":
		return "x", lipgloss.Color("240")
	default:
		return " ", lipgloss.Color("244")
	}
}

// activeProject is the project of today's in-progress task ("" if none),
// used to flag the running project in the Projects list.
func (m Model) activeProject() string {
	if m.live == nil {
		return ""
	}
	for _, t := range m.live.Tasks {
		if t.Status == "in-progress" {
			return t.Project
		}
	}
	return ""
}

// todayRollup is a compact count of today's tasks by status plus the day total,
// e.g. ">1 =0 x4 · Σ16h44m" (empty until today's status has loaded).
func (m Model) todayRollup() string {
	if m.live == nil {
		return ""
	}
	var run, paused, deferred, done int
	for _, t := range m.live.Tasks {
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
		return lipgloss.NewStyle().Foreground(c).Render(g) + dimStyle.Render(fmt.Sprintf("%d", n))
	}
	counts := seg("in-progress", run) + " " + seg("paused", paused)
	if deferred > 0 {
		counts += " " + seg("deferred", deferred)
	}
	counts += " " + seg("completed", done)
	return counts + dimStyle.Render(" · Σ"+fmtDur(m.live.TotalMinutes))
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
// project filter), a time axis from shift_start to shift_end, colored bars for
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
		for i, r := range lbl {
			if at+i < axisW {
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
		marker[col(nm)] = '▲'
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
// A positive height forces the box to that total height (content top-aligned),
// so stacked panels fill the screen; height 0 leaves it content-sized.
func panel(title, body string, active bool, width, height int) string {
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

// styles
var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	footerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	focusStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Underline(true)
	todayStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("84")) // green = today
	selStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("238"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	noticeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("45")) // cyan: neutral feedback
	inputStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220"))
)

// helpers ---------------------------------------------------------------------

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
