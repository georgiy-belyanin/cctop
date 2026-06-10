// cctop — Claude Code top: a live table of all running Claude Code
// instances on this machine (status, model, effort, working directory).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/term"

	"github.com/georgiy-belyanin/cctop/internal/discover"
	"github.com/georgiy-belyanin/cctop/internal/instance"
	"github.com/georgiy-belyanin/cctop/internal/tui"
)

func main() {
	interval := flag.Duration("interval", time.Second, "refresh interval")
	once := flag.Bool("once", false, "print the table once and exit (no TUI)")
	root := flag.String("root", "", "Claude Code data dir (default ~/.claude)")
	flag.Parse()

	if *root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fatal(err)
		}
		*root = filepath.Join(home, ".claude")
	}
	scanner := discover.New(*root)

	if *once || !term.IsTerminal(int(os.Stdout.Fd())) {
		list, err := scanner.Scan()
		if err != nil {
			fatal(err)
		}
		instance.Sort(list)
		for _, line := range tui.Table(list, tui.Options{Width: 120, Color: false, Now: time.Now()}) {
			fmt.Println(line)
		}
		return
	}

	if err := tui.Run(scanner.Scan, *interval); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "cctop:", err)
	os.Exit(1)
}
