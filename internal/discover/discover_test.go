package discover

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/georgiy-belyanin/cctop/internal/instance"
)

func TestMapStatus(t *testing.T) {
	cases := map[string]instance.Status{
		"idle":       instance.Idle,
		"":           instance.Idle,
		"busy":       instance.Auto,
		"shell":      instance.Auto,
		"compacting": instance.Ask, // unknown values surface as Ask
	}
	for raw, want := range cases {
		if got := mapStatus(raw); got != want {
			t.Errorf("mapStatus(%q) = %v, want %v", raw, got, want)
		}
	}
}

// deadPID returns a PID that is guaranteed not to be running: a child that
// has already been reaped.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("run true: %v", err)
	}
	return cmd.Process.Pid
}

// writeSession writes a registry file into root/sessions.
func writeSession(t *testing.T, root string, sf sessionFile) {
	t.Helper()
	dir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(sf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.json", sf.PID)), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "proj")
	now := time.Now()

	// Our own process plays the live instance; its real start time makes the
	// PID-reuse guard pass.
	self := os.Getpid()
	started, err := procStartTime(self)
	if err != nil {
		t.Fatalf("procStartTime(self): %v", err)
	}

	writeFile(t, filepath.Join(root, "settings.json"),
		`{"effortLevel":"high","model":"claude-fable-5[1m]","otherKey":1}`)
	writeFile(t, filepath.Join(cwd, ".claude", "settings.json"),
		`{"effortLevel":"medium"}`)

	const sid = "11111111-2222-3333-4444-555555555555"
	writeFile(t, filepath.Join(root, "projects", "-munged-proj", sid+".jsonl"),
		`{"type":"assistant","message":{"model":"claude-opus-4-7"}}`+"\n"+
			`{"type":"user","message":{"content":"hi"}}`+"\n"+
			`{"type":"assistant","message":{"model":"claude-opus-4-8"}}`+"\n")

	writeSession(t, root, sessionFile{
		PID: self, SessionID: sid, CWD: cwd, Status: "busy", Name: "live-one",
		ProcStart: started.Format(time.ANSIC),
		StartedAt: now.Add(-time.Hour).UnixMilli(), UpdatedAt: now.UnixMilli(),
	})

	dead := deadPID(t)
	writeSession(t, root, sessionFile{
		PID: dead, SessionID: "dead-session", CWD: cwd, Status: "busy",
		StartedAt: now.Add(-time.Minute).UnixMilli(), UpdatedAt: now.UnixMilli(),
	})

	ancient := deadPID(t)
	writeSession(t, root, sessionFile{
		PID: ancient, SessionID: "ancient", Status: "idle",
		UpdatedAt: now.Add(-2 * time.Hour).UnixMilli(),
	})

	writeFile(t, filepath.Join(root, "sessions", "999.json"), "{not json")

	list, err := New(root).Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	byPID := map[int]instance.Instance{}
	for _, in := range list {
		byPID[in.PID] = in
	}
	if len(list) != 3 {
		t.Fatalf("got %d instances (%v), want 3", len(list), byPID)
	}

	live := byPID[self]
	if live.Status != instance.Auto {
		t.Errorf("live status = %v, want auto", live.Status)
	}
	if live.Model != "claude-opus-4-8" {
		t.Errorf("live model = %q, want newest transcript model", live.Model)
	}
	if live.Effort != "medium" {
		t.Errorf("live effort = %q, want project override %q", live.Effort, "medium")
	}
	if live.Name != "live-one" || live.CWD != cwd {
		t.Errorf("live name/cwd = %q/%q", live.Name, live.CWD)
	}

	if got := byPID[dead]; got.Status != instance.Error {
		t.Errorf("dead-pid status = %v, want error", got.Status)
	} else if got.Model != "claude-fable-5[1m]" {
		t.Errorf("dead-pid model = %q, want settings fallback", got.Model)
	}

	if _, ok := byPID[ancient]; ok {
		t.Error("ancient stale entry should be dropped")
	}

	if got := byPID[999]; got.Status != instance.Error {
		t.Errorf("unparsable file status = %v, want error", got.Status)
	}
}

func TestScanMissingDir(t *testing.T) {
	list, err := New(filepath.Join(t.TempDir(), "nope")).Scan()
	if err != nil || list != nil {
		t.Fatalf("missing root: got %v, %v; want nil, nil", list, err)
	}
}

func TestLastModelTailWindow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	var b strings.Builder
	// An old model entry that ends up outside the tail window…
	b.WriteString(`{"type":"assistant","message":{"model":"claude-ancient"}}` + "\n")
	pad := `{"type":"user","message":{"content":"` + strings.Repeat("x", 1024) + `"}}` + "\n"
	for b.Len() < tailWindow+4096 {
		b.WriteString(pad)
	}
	// …and the current one inside it.
	b.WriteString(`{"type":"assistant","message":{"model":"claude-current"}}` + "\n")
	writeFile(t, path, b.String())

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := lastModel(path, fi.Size()); got != "claude-current" {
		t.Errorf("lastModel = %q, want claude-current", got)
	}
}

func TestSameProcessTimezones(t *testing.T) {
	self := os.Getpid()
	started, err := procStartTime(self)
	if err != nil {
		t.Fatalf("procStartTime: %v", err)
	}
	// Claude Code has been observed writing procStart in UTC; local must
	// also be accepted.
	for _, s := range []string{
		started.UTC().Format(time.ANSIC),
		started.Local().Format(time.ANSIC),
	} {
		if !sameProcess(self, s) {
			t.Errorf("sameProcess(self, %q) = false, want true", s)
		}
	}
	if sameProcess(self, started.Add(-time.Hour).UTC().Format(time.ANSIC)) {
		t.Error("sameProcess accepted a start time an hour off — PID-reuse guard broken")
	}
	if !sameProcess(self, "garbage") {
		t.Error("unparsable procStart must be best-effort accepted")
	}
}
