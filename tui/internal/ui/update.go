package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"strings"
	"time"
)

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
			// Collapse whitespace so any multi-line reply stays a single
			// footer line and can't break the fixed-height footer.
			m.notice = strings.Join(strings.Fields(msg.note), " ")
		}
		// reload regardless: even on a CLI error the log may have changed
		return m, m.reloadAll()

	case tea.ResumeMsg:
		// brought back to the foreground after a ctrl+z suspend — the log may
		// have changed in the meantime, so refresh every view.
		return m, m.reloadAll()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// ctrl+z suspends to the background (job control) from any mode — Bubble Tea
	// restores the terminal, and we refresh on the ResumeMsg when foregrounded.
	if msg.String() == "ctrl+z" {
		return m, tea.Suspend
	}

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
	case "tab", "l":
		// l (drill-in) and Tab both advance one panel, wrapping past the last
		// back to the first — so the cycle now includes Pending.
		m.pane = (m.pane + 1) % paneCount
		return m, nil
	case "shift+tab", "h":
		// h (drill-out) and Shift+Tab step back one panel, wrapping the first
		// back to the last.
		m.pane = (m.pane + paneCount - 1) % paneCount
		return m, nil
	case "1": // jump straight to a panel (works from any pane)
		m.pane = paneRange
		return m, nil
	case "2":
		m.pane = paneDay
		return m, nil
	case "3":
		m.pane = paneTimeline
		return m, nil
	case "4":
		m.pane = panePending
		return m, nil
	case "s":
		// start a new task — global. Description plus an optional inline
		// "@project" (⇥ cycles known projects, like the add/move prompts).
		m.input = inputMode{active: true, action: "start",
			prompt: "start: desc  (optional @project ⇥completes  %time, e.g. %9:30)"}
		return m, nil
	case "A":
		// toggle how start/resume treat another running task in the same project:
		// parallel (default) vs auto-pause the previous one.
		m.autoPause = !m.autoPause
		if m.autoPause {
			m.notice = "start/resume: auto-pause same-project task (one at a time)"
		} else {
			m.notice = "start/resume: run in parallel (same-project tasks coexist)"
		}
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
	args := baseArgs(verb, m.withPauseFlag(verb, valueArgs), day)
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
	m.input = inputMode{active: true, action: "at", pending: baseArgs(verb, m.withPauseFlag(verb, valueArgs), day),
		prompt: label + " — time (e.g. 14:30)"}
	return m, nil
}

// withPauseFlag appends the explicit --parallel / --auto-pause flag for the two
// verbs that auto-pause (start, resume), so the TUI's behaviour is independent of
// the auto_pause config and follows the in-session toggle (default: parallel).
// Other verbs pass through unchanged. A fresh slice is returned so the caller's
// valueArgs is never mutated.
func (m Model) withPauseFlag(verb string, valueArgs []string) []string {
	out := append([]string{}, valueArgs...)
	if verb != "start" && verb != "resume" {
		return out
	}
	if m.autoPause {
		return append(out, "--auto-pause")
	}
	return append(out, "--parallel")
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

// keepPrompt leaves the active input open and surfaces hint (rendered as the
// footer's ⚠ line), so an empty or incomplete submit guides the user instead of
// silently vanishing. Esc still aborts the prompt outright.
func (m Model) keepPrompt(hint string) (tea.Model, tea.Cmd) {
	m.err = hint
	return m, nil
}

// closeInput dismisses the active prompt and clears any lingering hint, called
// once a submit has been accepted.
func (m *Model) closeInput() {
	m.input = inputMode{}
	m.err = ""
}

// handleInput feeds keystrokes to the active text prompt.
func (m Model) handleInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		in := m.input
		val := strings.TrimSpace(in.value)
		switch in.action {
		case "at":
			if val == "" {
				return m.keepPrompt("time required: type a time like 14:30 — esc to abort")
			}
			m.closeInput()
			return m, m.mutate(append(in.pending, "--at", val)...)
		case "start":
			desc, proj, at := parseStartInput(val)
			if desc == "" {
				// the failing case is usually "@project" with no text — say so
				// plainly rather than echoing an @proj example that looks like
				// the input was accepted.
				hint := "description required: type what you're working on — esc to abort"
				if proj != "" {
					hint = "description required: @" + proj + " needs task text too — esc to abort"
				}
				return m.keepPrompt(hint)
			}
			m.closeInput()
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
				return m.keepPrompt("new description required — esc to abort")
			}
			m.closeInput()
			return m.issueMutation("amend", []string{in.taskID, val})
		case "move":
			if val == "" {
				return m.keepPrompt("project required — esc to abort")
			}
			m.closeInput()
			return m.issueMutation("move", []string{in.taskID, val})
		case "log":
			if val == "" {
				return m.keepPrompt("note required — esc to abort")
			}
			m.closeInput()
			return m.issueMutation("log", []string{val})
		case "add": // new pending backlog task (not a dated mutation)
			desc, proj, due := parsePendingInput(val)
			if desc == "" {
				// same trap as start: "@project" alone gives no description.
				hint := "description required: type what to add — esc to abort"
				if proj != "" {
					hint = "description required: @" + proj + " needs task text too — esc to abort"
				}
				return m.keepPrompt(hint)
			}
			m.closeInput()
			args := []string{"add", desc}
			if proj != "" {
				args = append(args, "--project", proj)
			}
			if due != "" {
				args = append(args, "--due", due)
			}
			return m, m.mutate(args...)
		case "pdue": // set or clear a pending task's deadline (empty clears it)
			m.closeInput()
			d := val
			if d == "" {
				d = "-"
			}
			return m, m.mutate("due", in.taskID, d)
		}
		m.closeInput()
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
		m.closeInput() // deliberate abort — silent, but clears any hint
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
	case "enter":
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
	// span presets moved off the bare digits (now panel-jump keys) onto their
	// shifted variants: ⇧1 / ⇧2 / ⇧3 → 1- / 7- / 30-day window.
	case "!":
		return m.setSpan(1)
	case "@":
		return m.setSpan(7)
	case "#":
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
	case "enter":
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
