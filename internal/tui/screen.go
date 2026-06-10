// Package tui renders the cctop table with raw ANSI escape sequences and
// runs the interactive refresh loop. Rendering is pure (strings in, strings
// out) so it is testable without a TTY.
package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"cctop/internal/instance"
)

const (
	reset   = "\x1b[0m"
	bold    = "\x1b[1m"
	dim     = "\x1b[2m"
	red     = "\x1b[31m"
	green   = "\x1b[32m"
	yellow  = "\x1b[33m"
	reverse = "\x1b[7m"
)

// Options controls table rendering.
type Options struct {
	Width int
	Color bool
	Now   time.Time
}

// Column layout: fixed widths for everything but CWD, which absorbs the rest.
const (
	pidW    = 6
	statusW = 8
	modelW  = 22
	effortW = 7
	upW     = 7
	nameW   = 22
	gap     = "  "
)

// Table renders one frame as a slice of lines (no trailing newlines, no
// cursor positioning — the caller owns screen control).
func Table(list []instance.Instance, o Options) []string {
	w := o.Width
	if w <= 0 {
		w = 100
	}
	cwdW := max(12, w-(pidW+statusW+modelW+effortW+upW+nameW+6*len(gap)))

	paint := func(code, s string) string {
		if !o.Color || code == "" {
			return s
		}
		return code + s + reset
	}

	lines := make([]string, 0, len(list)+3)

	left := fmt.Sprintf("cctop — %d session%s", len(list), plural(len(list)))
	right := o.Now.Format("15:04:05") + "  q quit"
	pad := max(1, w-len(left)-len(right))
	lines = append(lines, paint(bold, left)+strings.Repeat(" ", pad)+paint(dim, right))
	lines = append(lines, "")

	header := strings.Join([]string{
		fit("PID", pidW), fit("STATUS", statusW), fit("MODEL", modelW),
		fit("EFFORT", effortW), fit("UP", upW), fit("NAME", nameW), fit("CWD", cwdW),
	}, gap)
	lines = append(lines, paint(reverse, header))

	if len(list) == 0 {
		lines = append(lines, paint(dim, "no running Claude Code sessions"))
		return lines
	}

	for _, in := range list {
		name := in.Name
		if name == "" && in.Detail != "" {
			name = in.Detail
		}
		row := strings.Join([]string{
			fit(fmt.Sprintf("%d", in.PID), pidW),
			paint(statusColor(in.Status), fit(statusText(in), statusW)),
			fit(in.Model, modelW),
			fit(in.Effort, effortW),
			fit(uptime(o.Now, in.StartedAt), upW),
			fit(name, nameW),
			fitLeft(home(in.CWD), cwdW),
		}, gap)
		lines = append(lines, row)
	}
	return lines
}

func statusText(in instance.Instance) string {
	// Unknown registry values land in Ask; show them verbatim so new Claude
	// Code states are visible rather than mislabeled.
	if in.Status == instance.Ask && in.RawStatus != "" && in.RawStatus != "ask" {
		return in.RawStatus
	}
	return string(in.Status)
}

func statusColor(s instance.Status) string {
	switch s {
	case instance.Idle:
		return dim
	case instance.Auto:
		return green
	case instance.Ask:
		return yellow
	case instance.Error:
		return red
	}
	return ""
}

// fit pads or truncates (with …) to exactly w cells.
func fit(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		if w <= 1 {
			return string(r[:w])
		}
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

// fitLeft truncates from the left, keeping the tail — right for paths.
func fitLeft(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		if w <= 1 {
			return string(r[len(r)-w:])
		}
		return "…" + string(r[len(r)-w+1:])
	}
	return s + strings.Repeat(" ", w-len(r))
}

func uptime(now time.Time, started time.Time) string {
	if started.IsZero() || started.Unix() <= 0 {
		return "—"
	}
	d := now.Sub(started)
	switch {
	case d < 0:
		return "—"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

var homeDir, _ = os.UserHomeDir()

func home(path string) string {
	if homeDir != "" && strings.HasPrefix(path, homeDir) {
		return "~" + strings.TrimPrefix(path, homeDir)
	}
	return path
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
