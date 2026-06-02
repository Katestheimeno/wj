package wj

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoWJ returns the absolute path to the bash `wj` script at the repo root,
// or "" if it can't be located.
func repoWJ(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd() // .../tui/internal/wj
	if err != nil {
		return ""
	}
	p := filepath.Clean(filepath.Join(wd, "..", "..", "..", "wj"))
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// TestClientRoundTrip seeds a day through the real CLI and reads it back via the
// JSON contract, asserting the bash and Go sides agree end-to-end.
func TestClientRoundTrip(t *testing.T) {
	wjBin := repoWJ(t)
	if wjBin == "" {
		t.Skip("wj script not found relative to module; skipping integration test")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	data := t.TempDir()
	cfg := filepath.Join(t.TempDir(), "config")
	t.Setenv("WJ_DATA_DIR", data)
	t.Setenv("WJ_CONFIG", cfg)

	cli := Client{Bin: wjBin}

	// seed: one completed backend task on a past day
	seed := [][]string{
		{"start", "Refactor auth", "--project", "backend", "--date", "2026-05-28", "--at", "09:00"},
		{"complete", "T1", "--project", "backend", "--date", "2026-05-28", "--at", "10:30"},
	}
	for _, args := range seed {
		if _, err := cli.run(args...); err != nil {
			t.Fatalf("seed %v: %v", args, err)
		}
	}

	// status round-trips
	st, err := cli.Status("2026-05-28")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(st.Tasks) != 1 || st.Tasks[0].Project != "backend" || st.Tasks[0].Minutes != 90 {
		t.Fatalf("status mismatch: %+v", st)
	}

	// grid segments round-trip
	g, err := cli.Grid("2026-05-28")
	if err != nil {
		t.Fatalf("Grid: %v", err)
	}
	if len(g.Tasks) != 1 || len(g.Tasks[0].Segments) != 1 {
		t.Fatalf("grid mismatch: %+v", g)
	}
	if seg := g.Tasks[0].Segments[0]; seg.From != "09:00" || seg.To != "10:30" || seg.State != "complete" {
		t.Fatalf("segment mismatch: %+v", seg)
	}

	// gantt round-trips
	ga, err := cli.Gantt("2026-05-28", "2026-05-28", "project")
	if err != nil {
		t.Fatalf("Gantt: %v", err)
	}
	if len(ga.Rows) != 1 || ga.Rows[0].Key != "backend" || ga.Rows[0].TotalMinutes != 90 {
		t.Fatalf("gantt mismatch: %+v", ga)
	}
	if ga.Rows[0].PerDay["2026-05-28"] != 90 {
		t.Fatalf("gantt perDay mismatch: %+v", ga.Rows[0].PerDay)
	}
}

// TestClientMutate drives the exported Mutate path the TUI uses, then reads the
// result back — exercising the full key->mutate->wj->reload chain end to end.
func TestClientMutate(t *testing.T) {
	wjBin := repoWJ(t)
	if wjBin == "" {
		t.Skip("wj script not found relative to module; skipping integration test")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	t.Setenv("WJ_DATA_DIR", t.TempDir())
	t.Setenv("WJ_CONFIG", filepath.Join(t.TempDir(), "config"))
	cli := Client{Bin: wjBin}

	day := "2026-05-30"
	if _, err := cli.Mutate("start", "Write docs", "--project", "backend", "--date", day, "--at", "09:00"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := cli.Mutate("pause", "T1", "--date", day, "--at", "09:30"); err != nil {
		t.Fatalf("pause: %v", err)
	}
	if _, err := cli.Mutate("amend", "T1", "Write the docs", "--date", day, "--at", "09:31"); err != nil {
		t.Fatalf("amend: %v", err)
	}

	st, err := cli.Status(day)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(st.Tasks) != 1 {
		t.Fatalf("want 1 task, got %d", len(st.Tasks))
	}
	if got := st.Tasks[0]; got.Status != "paused" || got.Desc != "Write the docs" || got.Minutes != 30 {
		t.Fatalf("mutated task mismatch: %+v", got)
	}

	// re-pausing an already-paused task is an idempotent no-op: no error, and
	// the CLI echoes an "already paused" line so the UI can surface feedback.
	note, err := cli.Mutate("pause", "T1", "--date", day, "--at", "09:45")
	if err != nil {
		t.Fatalf("re-pause should not error: %v", err)
	}
	if !strings.Contains(note, "already paused") {
		t.Fatalf("re-pause note = %q, want it to contain %q", note, "already paused")
	}

	// a bad mutation surfaces an error (no such task)
	if _, err := cli.Mutate("complete", "T9", "--date", day); err == nil {
		t.Error("completing a nonexistent task should error")
	}
}
