package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) loadGantt() tea.Cmd {
	cli, from, to, by := m.cli, m.from, m.to, m.by
	mine := m.mineOnly && m.actor != "" // M scopes the Range rollup to your own time
	return func() tea.Msg {
		g, err := cli.Gantt(from, to, by, mine)
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

func (m Model) loadTags() tea.Cmd {
	cli := m.cli
	return func() tea.Msg {
		names, err := cli.Tags()
		if err != nil {
			return tagsMsg{}
		}
		return tagsMsg{names: names}
	}
}

func (m Model) loadActor() tea.Cmd {
	cli := m.cli
	return func() tea.Msg {
		name, err := cli.Actor()
		if err != nil {
			return actorMsg{}
		}
		return actorMsg{name: name}
	}
}

func (m Model) loadActors() tea.Cmd {
	cli := m.cli
	return func() tea.Msg {
		names, err := cli.Actors()
		if err != nil {
			return actorsMsg{}
		}
		return actorsMsg{names: names}
	}
}

// loadTeam fetches the per-author standup for the team overlay (today).
func (m Model) loadTeam() tea.Cmd {
	cli, today := m.cli, m.today
	return func() tea.Msg {
		t, err := cli.Team(today)
		if err != nil {
			return teamMsg{err: err}
		}
		return teamMsg{members: t.Members}
	}
}

// loadSyncable probes whether the data dir is a git sync repo, without doing a
// full sync. `wj sync status` prints "not a sync repo" (on stdout, exit 0) when
// it isn't — so check both the output and the wrapped error. The resulting
// syncMsg has an empty note, so it only sets m.syncable (no footer).
func (m Model) loadSyncable() tea.Cmd {
	cli := m.cli
	return func() tea.Msg {
		out, err := cli.Mutate("sync", "status")
		if strings.Contains(out, "not a sync repo") ||
			(err != nil && strings.Contains(err.Error(), "not a sync repo")) {
			return syncMsg{ok: false}
		}
		return syncMsg{ok: true}
	}
}

// runSync runs `wj sync` in the background. It never blocks the UI (the CLI is
// non-interactive with a timeout). A "not a sync repo" result is reported as
// ok=false so a non-shared journal silently disables auto-sync. On any other
// failure the reason (carried in err, since the CLI dies on stderr) is folded
// into note so the footer can surface it.
func (m Model) runSync() tea.Cmd {
	cli := m.cli
	return func() tea.Msg {
		note, err := cli.Mutate("sync")
		if err != nil && strings.Contains(err.Error(), "not a sync repo") {
			return syncMsg{ok: false}
		}
		if err != nil && note == "" {
			note = strings.TrimPrefix(err.Error(), "wj sync: ")
		}
		return syncMsg{ok: true, note: note, err: err}
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
		m.loadShow(m.selectedTaskID(), m.currentDay()), m.loadLive(), m.loadPending(), m.loadTags(), m.loadActors()}
	if m.search.active { // keep an open search overlay in sync with mutations
		cmds = append(cmds, m.runSearch(m.search.query))
	}
	return tea.Batch(cmds...)
}
