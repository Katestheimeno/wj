// Package wj models the machine-readable JSON contract emitted by the `wj`
// CLI (the `--json` read views) and provides a thin client that shells out to
// it. All task/time derivation lives in the bash CLI; this package only reads
// and renders, so the two can never disagree.
package wj

// Status mirrors `wj status --json`: per-task totals for a single day.
type Status struct {
	Date         string `json:"date"`
	Now          string `json:"now"`
	Tasks        []Task `json:"tasks"`
	TotalMinutes int    `json:"total_minutes"`
}

// Task is one row of a day's status table.
type Task struct {
	ID      string   `json:"id"`
	Actor   string   `json:"actor"` // owning author (collaborative log); "" = pre-collab data
	Project string   `json:"project"`
	Status  string   `json:"status"`
	Minutes int      `json:"minutes"`
	Desc    string   `json:"desc"`
	Tags    []string `json:"tags"`
}

// Show mirrors `wj show <id> --json`: the full event timeline of one task.
type Show struct {
	ID      string   `json:"id"`
	Actor   string   `json:"actor"`
	Date    string   `json:"date"`
	Project string   `json:"project"`
	Status  string   `json:"status"`
	Desc    string   `json:"desc"`
	Minutes int      `json:"minutes"`
	Tags    []string `json:"tags"`
	Events  []Event  `json:"events"`
}

// Event is one entry in a task's timeline.
type Event struct {
	Time    string `json:"time"`
	Event   string `json:"event"`
	Project string `json:"project"`
	Note    string `json:"note"`
}

// Grid mirrors `wj grid --json`: intraday active segments per task, used by the
// single-day drill-down view.
type Grid struct {
	Date        string     `json:"date"`
	ShiftStart  string     `json:"shift_start"`
	ShiftEnd    string     `json:"shift_end"`
	SlotMinutes int        `json:"slot_minutes"`
	Now         string     `json:"now"`
	Tasks       []GridTask `json:"tasks"`
}

// GridTask is one task's intraday timeline.
type GridTask struct {
	ID       string    `json:"id"`
	Actor    string    `json:"actor"`
	Project  string    `json:"project"`
	Desc     string    `json:"desc"`
	Status   string    `json:"status"`
	Minutes  int       `json:"minutes"`
	Tags     []string  `json:"tags"`
	Segments []Segment `json:"segments"`
}

// Segment is one active interval. State records how it ended:
// "active" (ongoing), "pause", "complete", or "defer".
type Segment struct {
	From  string `json:"from"`
	To    string `json:"to"`
	State string `json:"state"`
}

// Gantt mirrors `wj gantt --json`: the multi-day range overview (rows x days).
type Gantt struct {
	From string     `json:"from"`
	To   string     `json:"to"`
	By   string     `json:"by"`
	Days []string   `json:"days"`
	Rows []GanttRow `json:"rows"`
}

// Pending is one not-yet-started backlog task from `wj pending --json`. It has
// no tracked time until promoted via `wj start P#`; Due is "" when unset.
type Pending struct {
	ID      string `json:"id"`
	Actor   string `json:"actor"` // owning author in a shared backlog ("" = solo)
	Created string `json:"created"`
	Project string `json:"project"`
	Due     string `json:"due"`
	Desc    string `json:"desc"`
}

// Found is one hit from `wj search --json`: a task located by id/project/desc
// substring, on a specific day. The (ID, Date) pair is enough to jump to it.
type Found struct {
	ID      string   `json:"id"`
	Actor   string   `json:"actor"`
	Date    string   `json:"date"`
	Project string   `json:"project"`
	Desc    string   `json:"desc"`
	Status  string   `json:"status"`
	Minutes int      `json:"minutes"`
	Tags    []string `json:"tags"`
}

// Team mirrors `wj team --json`: who is working on what right now across every
// author in the shared log, for a given day.
type Team struct {
	Date    string   `json:"date"`
	Members []Member `json:"members"`
}

// Member is one author's standup line: their currently-running task (if
// Running) and their tracked total for the day.
type Member struct {
	Actor        string `json:"actor"`
	Running      bool   `json:"running"`
	ID           string `json:"id"`
	Desc         string `json:"desc"`
	Project      string `json:"project"`
	Minutes      int    `json:"minutes"`
	TotalMinutes int    `json:"total_minutes"`
}

// GanttRow is one project (or task) row. PerDay maps a date (YYYY-MM-DD) to
// minutes worked; days with no work are absent (treat as zero).
type GanttRow struct {
	Key          string         `json:"key"`
	Label        string         `json:"label"`
	Project      string         `json:"project"` // the row's project (== Key when --by project)
	TotalMinutes int            `json:"total_minutes"`
	PerDay       map[string]int `json:"perDay"`
}
