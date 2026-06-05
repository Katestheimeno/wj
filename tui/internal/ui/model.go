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

// tagsMsg carries every tag ever used (for the tag-editor autocomplete).
type tagsMsg struct{ names []string }

// actorMsg carries the current author handle (to distinguish your tasks from
// teammates' in a shared log).
type actorMsg struct{ name string }

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
	verb      string    // wj verb to run on confirmation
	valueArgs []string  // args between the verb and --date (e.g. the task id)
	raw       bool      // run as a plain mutate (no --date), e.g. for backlog drop
	atTime    bool      // after confirming, prompt for an explicit --at time
	input     inputMode // after confirming, open this text prompt instead (amend/move)
}

// confirmLevel controls which actions pop a y/n guard, set from the `confirm`
// config (all | destructive | off). "destructive" guards only the void/drop
// actions; "all" guards every mutating action; "off" guards nothing (undo is the
// safety net). The zero value is confirmOff, so test models opt in explicitly.
type confirmLevel int

const (
	confirmOff confirmLevel = iota
	confirmDestructive
	confirmAll
)

// parseConfirmLevel maps the config string to a level (default: destructive).
func parseConfirmLevel(s string) confirmLevel {
	switch s {
	case "all":
		return confirmAll
	case "off", "none":
		return confirmOff
	default: // "destructive", "", or anything unrecognised
		return confirmDestructive
	}
}

// Model is the root Bubble Tea model.
type Model struct {
	cli wj.Client

	today    string // YYYY-MM-DD; mutations on other days require an explicit --at
	from, to string // current range (YYYY-MM-DD); empty until first load
	by       string // "project" | "task"

	g          *wj.Gantt
	focusedDay int // index into g.Days
	focusedRow int // sidebar Projects index into projRows() (Today then Window)

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
	tags      []string   // known tag names (tag-editor autocomplete)
	actor     string     // this user's author handle (collaborative log; "" = solo)
	tickN     int        // 1s ticks since start; data reloads every dataEveryTicks
	tlOffset  int        // timeline scroll position
	autoPause bool       // when true, start/resume pause the project's other running task
	layout    int        // index into layouts (panel arrangement); cycled with Shift+L
	zoomed    bool       // when true, the focused pane fills the screen (toggled with z)

	confirmLevel confirmLevel // which actions pop a y/n guard (from the `confirm` config)
}

// needConfirm reports whether an action should pop a y/n guard. destructive marks
// the void/drop actions, which are guarded at the "destructive" level too.
func (m Model) needConfirm(destructive bool) bool {
	switch m.confirmLevel {
	case confirmAll:
		return true
	case confirmDestructive:
		return destructive
	default: // confirmOff
		return false
	}
}

// liveDelta is the whole minutes elapsed since today's status was fetched, added
// to an in-progress task's tracked time so the Today list counts up between the
// (coarser) data reloads. 0 until the first live status lands.
func (m Model) liveDelta() int {
	if m.liveAt.IsZero() {
		return 0
	}
	if d := int(time.Since(m.liveAt).Minutes()); d > 0 {
		return d
	}
	return 0
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
func New(cli wj.Client, from, to, by, confirm string) Model {
	if by == "" {
		by = "project"
	}
	return Model{cli: cli, from: from, to: to, by: by, today: time.Now().Format(dateLayout),
		layout: defaultLayout, confirmLevel: parseConfirmLevel(confirm)}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadGantt(), m.loadLive(), m.loadProjects(), m.loadTags(), m.loadActor(), m.loadPending(), tickCmd())
}

// currentDay is the YYYY-MM-DD of the focused day column ("" if none).
func (m Model) currentDay() string {
	if m.g == nil || m.focusedDay < 0 || m.focusedDay >= len(m.g.Days) {
		return ""
	}
	return m.g.Days[m.focusedDay]
}

// projRow is one selectable entry in the Projects panel. The panel stacks two
// sections: Today (derived from today's live status, so it is independent of the
// browsing window) on top, then Window (the range gantt rows, led by an "All"
// entry). focusedRow indexes the flattened list returned by projRows.
type projRow struct {
	project string // project to filter the day detail by; "" = All (no filter)
	label   string
	minutes int
	today   bool // a Today-section row
	isAll   bool // the Window "All" entry
	running bool // today's in-progress project (drives the ">" glyph)
}

// myTasks is today's live tasks that belong to you (or all of them when the
// actor is unset / solo / pre-collaborative data). The personal surfaces — the
// header, the today rollup, and the Today panel section — use this so they show
// your work, not the whole team's (team rollups live in the Range / Window).
func (m Model) myTasks() []wj.Task {
	if m.live == nil {
		return nil
	}
	if m.actor == "" {
		return m.live.Tasks
	}
	out := make([]wj.Task, 0, len(m.live.Tasks))
	for _, t := range m.live.Tasks {
		if t.Actor == "" || t.Actor == m.actor {
			out = append(out, t)
		}
	}
	return out
}

// todayRows aggregates today's tracked time (your live tasks) into Projects-panel
// rows, honouring the by-project/by-task grouping. Empty when you have no work
// today (or status hasn't loaded), which collapses the panel to the Window list.
func (m Model) todayRows() []projRow {
	mine := m.myTasks()
	if len(mine) == 0 {
		return nil
	}
	delta := m.liveDelta() // count an in-progress task up between data reloads
	if m.by == "task" {
		rows := make([]projRow, 0, len(mine))
		for _, t := range mine {
			label := t.ID + " " + t.Project
			if t.Desc != "" {
				label = t.ID + " " + t.Desc
			}
			running := t.Status == "in-progress"
			mins := t.Minutes
			if running {
				mins += delta
			}
			rows = append(rows, projRow{project: t.Project, label: label, minutes: mins,
				today: true, running: running})
		}
		return rows
	}
	active := m.activeProject()
	order := make([]string, 0, len(mine))
	sum := make(map[string]int, len(mine))
	for _, t := range mine {
		if _, seen := sum[t.Project]; !seen {
			order = append(order, t.Project)
		}
		sum[t.Project] += t.Minutes
		if t.Status == "in-progress" {
			sum[t.Project] += delta
		}
	}
	rows := make([]projRow, 0, len(order))
	for _, p := range order {
		label := p
		if label == "" {
			label = "(no project)"
		}
		rows = append(rows, projRow{project: p, label: label, minutes: sum[p],
			today: true, running: p != "" && p == active})
	}
	return rows
}

// windowSection is the range gantt as Projects-panel rows: an "All" entry then
// one row per gantt row (project or task).
func (m Model) windowSection() []projRow {
	if m.g == nil {
		return nil
	}
	total := 0
	for _, r := range m.g.Rows {
		total += r.TotalMinutes
	}
	active := m.activeProject()
	rows := make([]projRow, 0, len(m.g.Rows)+1)
	rows = append(rows, projRow{label: "All", minutes: total, isAll: true})
	for _, r := range m.g.Rows {
		p := rowProject(r)
		rows = append(rows, projRow{project: p, label: r.Label, minutes: r.TotalMinutes,
			running: p != "" && p == active})
	}
	return rows
}

// projRows is the full ordered list of selectable Projects-panel entries: the
// Today section first, then the Window section.
func (m Model) projRows() []projRow {
	return append(m.todayRows(), m.windowSection()...)
}

// allRow is the index of the Window "All" entry — the no-filter default.
func (m Model) allRow() int {
	for i, r := range m.projRows() {
		if r.isAll {
			return i
		}
	}
	return 0
}

// projectFilter is the project selected in the Projects panel ("" = All / no
// filter); it drives the master→detail filtering of the focused day's tasks.
func (m Model) projectFilter() string {
	rows := m.projRows()
	if m.focusedRow < 0 || m.focusedRow >= len(rows) {
		return ""
	}
	return rows[m.focusedRow].project
}

// selectedToday reports whether the focused Projects row is in the Today section.
func (m Model) selectedToday() bool {
	rows := m.projRows()
	return m.focusedRow >= 0 && m.focusedRow < len(rows) && rows[m.focusedRow].today
}

// projAnchor identifies a selected Projects row by identity (not raw index) so
// the selection survives reloads that resize the dynamic Today section.
type projAnchor struct {
	today   bool
	isAll   bool
	project string
	idx     int // ordinal fallback when no identity match remains
}

// currentAnchor snapshots the focused row's identity before a data swap.
func (m Model) currentAnchor() projAnchor {
	rows := m.projRows()
	if m.focusedRow < 0 || m.focusedRow >= len(rows) {
		return projAnchor{isAll: true}
	}
	r := rows[m.focusedRow]
	return projAnchor{today: r.today, isAll: r.isAll, project: r.project, idx: m.focusedRow}
}

// anchorIndex re-resolves an anchor against the current projRows, preferring an
// exact (section, project) match, then any project match, then the clamped idx.
func (m Model) anchorIndex(a projAnchor) int {
	rows := m.projRows()
	if a.isAll {
		return m.allRow()
	}
	for i, r := range rows {
		if !r.isAll && r.today == a.today && r.project == a.project {
			return i
		}
	}
	for i, r := range rows {
		if !r.isAll && r.project == a.project {
			return i
		}
	}
	return clamp(a.idx, 0, max2(0, len(rows)-1))
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

// selectedTask returns the focused day's selected task (ok=false if none).
func (m Model) selectedTask() (wj.GridTask, bool) {
	ts := m.filteredTasks()
	if m.selTask < 0 || m.selTask >= len(ts) {
		return wj.GridTask{}, false
	}
	return ts[m.selTask], true
}

// taskOwned reports whether a task belongs to this user — i.e. you can act on it.
// Empty actor (solo / pre-collaborative data, or before the actor has loaded)
// counts as owned, so single-user behaviour is unchanged.
func (m Model) taskOwned(t wj.GridTask) bool {
	return t.Actor == "" || m.actor == "" || t.Actor == m.actor
}

// selectedPendID is the id of the highlighted pending task ("" if none).
func (m Model) selectedPendID() string {
	if m.selPend < 0 || m.selPend >= len(m.pending) {
		return ""
	}
	return m.pending[m.selPend].ID
}
