package tui

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"cctop/internal/instance"
)

// emit writes straight to the terminal; a failed write to our own TTY has
// no useful recovery, so the error is deliberately dropped.
func emit(s string) {
	_, _ = os.Stdout.WriteString(s)
}

// Run owns the terminal: raw mode, alternate screen, hidden cursor — all
// restored on every exit path, including signals. scan is called once per
// tick; its result is sorted and repainted in full (no diffing).
func Run(scan func() ([]instance.Instance, error), interval time.Duration) error {
	in := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(in)
	if err != nil {
		return fmt.Errorf("enter raw mode: %w", err)
	}
	defer func() {
		emit("\x1b[?25h\x1b[?1049l") // cursor on, main screen
		_ = term.Restore(in, oldState)
	}()
	emit("\x1b[?1049h\x1b[?25l\x1b[2J") // alt screen, cursor off, clear

	keys := make(chan byte, 8)
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				close(keys)
				return
			}
			if n > 0 {
				keys <- buf[0]
			}
		}
	}()

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigc)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	draw := func() {
		// Re-query size every frame: cheap, and handles resize without
		// listening for SIGWINCH.
		w, _, err := term.GetSize(in)
		if err != nil || w <= 0 {
			w = 100
		}
		list, scanErr := scan()
		instance.Sort(list)
		lines := Table(list, Options{Width: w, Color: true, Now: time.Now()})
		var b strings.Builder
		b.WriteString("\x1b[H") // home; repaint over the old frame
		for _, l := range lines {
			b.WriteString(l)
			b.WriteString("\x1b[K\r\n")
		}
		if scanErr != nil {
			b.WriteString(red + "scan error: " + scanErr.Error() + reset + "\x1b[K\r\n")
		}
		b.WriteString("\x1b[J") // clear leftovers from a taller previous frame
		emit(b.String())
	}

	draw()
	for {
		select {
		case <-ticker.C:
			draw()
		case k, ok := <-keys:
			if !ok {
				return nil
			}
			switch k {
			case 'q', 'Q', 3 /* ^C */, 4 /* ^D */ :
				return nil
			}
		case <-sigc:
			return nil
		}
	}
}
