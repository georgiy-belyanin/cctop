// Package instance defines the model of a running Claude Code session as
// shown in the cctop table.
package instance

import (
	"slices"
	"time"
)

// Status is the user-facing state of an instance.
type Status string

const (
	Idle  Status = "idle"
	Auto  Status = "auto" // agent is actively working (registry: busy/shell)
	Ask   Status = "ask"  // needs user attention
	Error Status = "error"
)

// Instance is one running (or stale) Claude Code session.
type Instance struct {
	PID       int
	SessionID string
	Name      string // optional session title
	CWD       string
	RawStatus string // verbatim status from the session registry
	Status    Status
	Model     string
	Effort    string
	Version   string
	StartedAt time.Time
	UpdatedAt time.Time
	Detail    string // human-readable note, set for error rows
}

func order(s Status) int {
	switch s {
	case Error:
		return 0
	case Ask:
		return 1
	case Auto:
		return 2
	default:
		return 3
	}
}

// Sort orders instances by urgency (error, ask, auto, idle), most recently
// updated first within each group.
func Sort(list []Instance) {
	slices.SortStableFunc(list, func(a, b Instance) int {
		if d := order(a.Status) - order(b.Status); d != 0 {
			return d
		}
		switch {
		case a.UpdatedAt.After(b.UpdatedAt):
			return -1
		case b.UpdatedAt.After(a.UpdatedAt):
			return 1
		}
		return a.PID - b.PID
	})
}
