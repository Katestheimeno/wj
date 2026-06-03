package ui

import (
	tea "github.com/charmbracelet/bubbletea"
	"time"
)

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
