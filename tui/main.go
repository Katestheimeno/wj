// Command wj-tui is the optional graphical front-end for wj. It renders the
// event log via the `wj` CLI's --json read contract; the bash CLI remains the
// single source of all task/time logic. Launched by `wj ui` (or bare `wj` when
// interface=ui), it is never required for wj to function.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Katestheimeno/wj/tui/internal/ui"
	"github.com/Katestheimeno/wj/tui/internal/wj"
)

// version is the wj-tui version, overridable at build time via
// -ldflags "-X main.version=<v>".
var version = "dev"

func main() {
	var (
		from    = flag.String("from", "", "range start YYYY-MM-DD (default: CLI default range)")
		to      = flag.String("to", "", "range end YYYY-MM-DD (default: today)")
		by      = flag.String("by", "project", "gantt rows: project | task")
		bin     = flag.String("wj", "", "path to the wj binary (default: wj on PATH)")
		accent  = flag.String("accent", "", "border/header color: 256-color code, hex (#rrggbb), or name (default: purple)")
		colors  = flag.String("colors", "", "per-panel title colors, e.g. \"projects=39,timeline=#888888\"")
		layout  = flag.String("layout", "", "panel layout: balanced | spotlight | golden (default: balanced)")
		showVer = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("wj-tui %s\n", version)
		return
	}

	ui.SetAccent(*accent)
	ui.SetPanelColors(*colors)
	ui.SetLayout(*layout)
	cli := wj.Client{Bin: *bin}
	model := ui.New(cli, *from, *to, *by)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "wj-tui: %v\n", err)
		os.Exit(1)
	}
}
