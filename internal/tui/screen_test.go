package tui

import (
	"strings"
	"testing"
	"time"

	"cctop/internal/instance"
)

var renderTime = time.Date(2026, 6, 10, 15, 0, 0, 0, time.UTC)

func TestTableRows(t *testing.T) {
	list := []instance.Instance{
		{
			PID: 4242, Status: instance.Auto, Model: "claude-opus-4-8",
			Effort: "high", Name: "my-task", CWD: "/work/proj",
			StartedAt: renderTime.Add(-90 * time.Minute),
		},
		{
			PID: 7, Status: instance.Error, Detail: "process not running",
			CWD: "/work/other", StartedAt: renderTime.Add(-30 * time.Second),
		},
	}
	lines := Table(list, Options{Width: 120, Color: false, Now: renderTime})
	out := strings.Join(lines, "\n")

	for _, want := range []string{
		"cctop — 2 sessions",
		"PID", "STATUS", "MODEL", "EFFORT", "CWD",
		"4242", "auto", "claude-opus-4-8", "high", "1h30m", "my-task", "/work/proj",
		"error", "process not running", // detail fills the empty name cell
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("Color: false must not emit ANSI codes")
	}
}

func TestTableUnknownStatusShownVerbatim(t *testing.T) {
	list := []instance.Instance{{PID: 1, Status: instance.Ask, RawStatus: "compacting"}}
	out := strings.Join(Table(list, Options{Width: 120, Now: renderTime}), "\n")
	if !strings.Contains(out, "compact") {
		t.Errorf("unknown raw status not shown:\n%s", out)
	}
}

func TestTableEmpty(t *testing.T) {
	out := strings.Join(Table(nil, Options{Width: 80, Now: renderTime}), "\n")
	if !strings.Contains(out, "no running Claude Code sessions") {
		t.Errorf("empty-state line missing:\n%s", out)
	}
}

func TestTableNarrowWidthDoesNotPanic(t *testing.T) {
	list := []instance.Instance{{
		PID: 1, Status: instance.Idle,
		CWD: "/a/very/long/path/that/will/surely/not/fit/anywhere/at/all",
	}}
	for _, w := range []int{0, 1, 20, 60} {
		_ = Table(list, Options{Width: w, Color: true, Now: renderTime})
	}
}

func TestFitHelpers(t *testing.T) {
	if got := fit("abcdef", 4); got != "abc…" {
		t.Errorf("fit truncate = %q", got)
	}
	if got := fit("ab", 4); got != "ab  " {
		t.Errorf("fit pad = %q", got)
	}
	if got := fitLeft("/long/path/tail", 6); got != "…/tail" {
		t.Errorf("fitLeft = %q", got)
	}
}

func TestUptime(t *testing.T) {
	cases := map[time.Duration]string{
		42 * time.Second:             "42s",
		12 * time.Minute:             "12m",
		3*time.Hour + 12*time.Minute: "3h12m",
		50 * time.Hour:               "2d2h",
	}
	for d, want := range cases {
		if got := uptime(renderTime, renderTime.Add(-d)); got != want {
			t.Errorf("uptime(%v) = %q, want %q", d, got, want)
		}
	}
	if got := uptime(renderTime, time.Time{}); got != "—" {
		t.Errorf("zero start = %q, want —", got)
	}
}
