package wj

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Client shells out to the `wj` CLI. It only ever invokes subcommands (never a
// bare `wj`), so it cannot recurse back into the UI even when interface=ui.
type Client struct {
	// Bin is the wj executable to invoke; defaults to "wj" on PATH.
	Bin string
}

func (c Client) bin() string {
	if c.Bin != "" {
		return c.Bin
	}
	return "wj"
}

// run executes `wj <args...>` and returns stdout, wrapping failures with the
// captured stderr so the UI can surface a meaningful message.
func (c Client) run(args ...string) ([]byte, error) {
	cmd := exec.Command(c.bin(), args...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("wj %s: %s", strings.Join(args, " "), msg)
	}
	return out.Bytes(), nil
}

// readJSON runs a read subcommand with --json and decodes it into v.
func (c Client) readJSON(v any, args ...string) error {
	b, err := c.run(append(args, "--json")...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("wj %s: decode: %w", strings.Join(args, " "), err)
	}
	return nil
}

// Projects lists every distinct project name ever recorded (for autocomplete).
func (c Client) Projects() ([]string, error) {
	out, err := c.run("_projects")
	if err != nil {
		return nil, err
	}
	var ps []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			ps = append(ps, line)
		}
	}
	return ps, nil
}

// Mutate runs a state-changing subcommand (start, pause, complete, …) and
// discards its stdout. The bash CLI owns all the mutation logic; this just
// invokes it as a user would.
func (c Client) Mutate(args ...string) error {
	_, err := c.run(args...)
	return err
}

// Gantt fetches the multi-day range overview. Empty from/to let the CLI apply
// its default range (last 7 days through today); by is "project" or "task".
func (c Client) Gantt(from, to, by string) (*Gantt, error) {
	args := []string{"gantt", "--by", by}
	if from != "" {
		args = append(args, "--from", from)
	}
	if to != "" {
		args = append(args, "--to", to)
	}
	var g Gantt
	if err := c.readJSON(&g, args...); err != nil {
		return nil, err
	}
	return &g, nil
}

// Status fetches a day's per-task totals. Empty date means today.
func (c Client) Status(date string) (*Status, error) {
	args := []string{"status"}
	if date != "" {
		args = append(args, "--date", date)
	}
	var s Status
	if err := c.readJSON(&s, args...); err != nil {
		return nil, err
	}
	return &s, nil
}

// Grid fetches a day's intraday segments. Empty date means today.
func (c Client) Grid(date string) (*Grid, error) {
	args := []string{"grid"}
	if date != "" {
		args = append(args, "--date", date)
	}
	var g Grid
	if err := c.readJSON(&g, args...); err != nil {
		return nil, err
	}
	return &g, nil
}

// Pending lists the backlog of not-yet-started tasks, in manual (pinned) order.
func (c Client) Pending() ([]Pending, error) {
	var out []Pending
	if err := c.readJSON(&out, "pending"); err != nil {
		return nil, err
	}
	return out, nil
}

// Search finds tasks all-time by an id/project/description substring. An empty
// query returns every recorded task (most-recent day first).
func (c Client) Search(query string) ([]Found, error) {
	var out []Found
	if err := c.readJSON(&out, "search", query); err != nil {
		return nil, err
	}
	return out, nil
}

// Show fetches one task's full timeline on a given day. Empty date means today.
func (c Client) Show(id, date string) (*Show, error) {
	args := []string{"show", id}
	if date != "" {
		args = append(args, "--date", date)
	}
	var s Show
	if err := c.readJSON(&s, args...); err != nil {
		return nil, err
	}
	return &s, nil
}
