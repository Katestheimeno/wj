// Package ui implements the wj-tui front-end. The layout is lazygit-style: a
// narrow left sidebar of lists (Projects, then the focused day's Tasks) drives
// a wide main column of visualizations (the range Gantt, the focused day's
// intraday Gantt, and the selected task's Timeline). Selecting a project in the
// sidebar filters the day detail (master→detail). Navigation is vim-style:
// j/k move within a panel, h/l (and Tab/Shift+Tab) cycle the panels with
// wraparound, 1-4 jump straight to a panel, ←/→ step days.
package ui

import (
	"github.com/Katestheimeno/wj/tui/internal/wj"
	tea "github.com/charmbracelet/bubbletea"
	"time"
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
	cursor   int      // caret position within value, in runes (0..len)
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

	live      *wj.Status // today's status, for the running-task header clock
	liveAt    time.Time  // wall-clock time m.live was fetched
	projects  []string   // known project names (move autocomplete)
	tickN     int        // 1s ticks since start; data reloads every dataEveryTicks
	tlOffset  int        // timeline scroll position
	autoPause bool       // when true, start/resume pause the project's other running task
	layout    int        // index into layouts (panel arrangement); cycled with Shift+L
	zoomed    bool       // when true, the focused pane fills the screen (toggled with z)
}

// activeLayout is the current panel-arrangement profile (clamped defensively).
// Fallback-only auto-switch: on a terminal too small for an asymmetric layout to
// render well, it drops back to balanced so no panel gets crushed; the chosen
// layout still wins at normal sizes.
func (m Model) activeLayout() layoutProfile {
	lp := layouts[clamp(m.layout, 0, len(layouts)-1)]
	if lp.name != "balanced" && ((m.height > 0 && m.height < 18) || (m.width > 0 && m.width < 64)) {
		return layouts[0] // balanced
	}
	return lp
}

// New builds the initial model. from/to may be empty to use the CLI default
// range; by defaults to "project".
func New(cli wj.Client, from, to, by string) Model {
	if by == "" {
		by = "project"
	}
	return Model{cli: cli, from: from, to: to, by: by, today: time.Now().Format(dateLayout), layout: defaultLayout}
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

// selectedPendID is the id of the highlighted pending task ("" if none).
func (m Model) selectedPendID() string {
	if m.selPend < 0 || m.selPend >= len(m.pending) {
		return ""
	}
	return m.pending[m.selPend].ID
}
