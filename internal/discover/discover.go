// Package discover finds running Claude Code instances by reading the
// session registry that Claude Code maintains under ~/.claude. Everything
// here is read-only and tolerant of schema drift: unknown fields are
// ignored, a single bad file never fails the whole scan, and the schemas
// (undocumented Claude Code internals) are confined to this package.
package discover

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"cctop/internal/instance"
)

// sessionFile mirrors ~/.claude/sessions/<PID>.json (Claude Code v2.1.x).
type sessionFile struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
	StartedAt int64  `json:"startedAt"` // unix ms
	ProcStart string `json:"procStart"` // ANSIC, e.g. "Wed Jun 10 09:47:09 2026"
	Version   string `json:"version"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updatedAt"` // unix ms
	Name      string `json:"name"`
}

// staleCutoff is how long a dead instance's registry file is still shown as
// an error row before being dropped entirely.
const staleCutoff = time.Hour

// Scanner reads the Claude Code data directory. It caches per-session
// transcript lookups across Scan calls; create one and reuse it.
type Scanner struct {
	Root string // Claude Code data dir, normally ~/.claude

	mu          sync.Mutex
	transcripts map[string]*transcriptCache
}

// New returns a Scanner over the given Claude Code data directory.
func New(root string) *Scanner {
	return &Scanner{Root: root, transcripts: make(map[string]*transcriptCache)}
}

// Scan returns the current set of instances. A missing sessions directory is
// not an error (Claude Code was never run); only an unreadable one is.
func (s *Scanner) Scan() ([]instance.Instance, error) {
	dir := filepath.Join(s.Root, "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	user := loadSettings(filepath.Join(s.Root, "settings.json"))
	var out []instance.Instance
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if inst := s.load(filepath.Join(dir, e.Name()), user); inst != nil {
			out = append(out, *inst)
		}
	}
	return out, nil
}

func (s *Scanner) load(path string, user settings) *instance.Instance {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil // raced with removal
	}
	var sf sessionFile
	if err := json.Unmarshal(data, &sf); err != nil {
		pid, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(path), ".json"))
		return &instance.Instance{PID: pid, Status: instance.Error, Detail: "unreadable session file"}
	}

	inst := instance.Instance{
		PID:       sf.PID,
		SessionID: sf.SessionID,
		Name:      sf.Name,
		CWD:       sf.CWD,
		RawStatus: sf.Status,
		Version:   sf.Version,
		StartedAt: time.UnixMilli(sf.StartedAt),
		UpdatedAt: time.UnixMilli(sf.UpdatedAt),
	}

	switch {
	case !alive(sf.PID) || !sameProcess(sf.PID, sf.ProcStart):
		if time.Since(inst.UpdatedAt) > staleCutoff {
			return nil // ancient leftover, not worth a row
		}
		inst.Status = instance.Error
		inst.Detail = "process not running"
	default:
		inst.Status = mapStatus(sf.Status)
	}

	eff := effectiveSettings(user, sf.CWD)
	inst.Effort = eff.EffortLevel
	if m := s.model(sf.SessionID); m != "" {
		inst.Model = m
	} else {
		inst.Model = eff.Model
	}
	return &inst
}

// mapStatus folds the registry's status field (an open enum) into the four
// user-facing states. Unknown values are surfaced as Ask — "look at this" —
// and the UI shows the raw text.
func mapStatus(raw string) instance.Status {
	switch raw {
	case "idle", "":
		return instance.Idle
	case "busy", "shell":
		return instance.Auto
	default:
		return instance.Ask
	}
}

// sameProcess guards against PID reuse: the registry records the process
// start time, which must match the live process. The registry's procStart
// string carries no timezone and has been observed in UTC, so a match under
// either UTC or local interpretation counts. Best-effort — if either side
// can't be determined, the kill(0) liveness check stands alone.
func sameProcess(pid int, procStart string) bool {
	raw := strings.TrimSpace(procStart)
	wantUTC, errUTC := time.Parse(time.ANSIC, raw)
	wantLocal, errLocal := time.ParseInLocation(time.ANSIC, raw, time.Local)
	if errUTC != nil && errLocal != nil {
		return true
	}
	got, err := procStartTime(pid)
	if err != nil {
		return true
	}
	const tolerance = 10 * time.Second
	return (errUTC == nil && got.Sub(wantUTC).Abs() <= tolerance) ||
		(errLocal == nil && got.Sub(wantLocal).Abs() <= tolerance)
}
