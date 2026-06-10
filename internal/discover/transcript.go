package discover

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

// tailWindow is how much of the end of a transcript is examined when looking
// for the session's model. Transcripts grow to many MB; the model appears on
// every assistant entry, so the tail is always enough.
const tailWindow = 64 << 10

// transcriptCache remembers, per session, where its transcript lives and the
// last model extracted from it, keyed by file size+mtime.
type transcriptCache struct {
	path  string
	size  int64
	mtime time.Time
	model string
}

// model returns the model used by the session's most recent assistant turn,
// or "" if it can't be determined. Results are cached until the transcript
// file changes.
func (s *Scanner) model(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	c := s.transcripts[sessionID]
	if c == nil {
		// Locating by glob avoids re-implementing Claude Code's cwd→dirname
		// munging (/ and . replaced by -).
		matches, _ := filepath.Glob(filepath.Join(s.Root, "projects", "*", sessionID+".jsonl"))
		if len(matches) == 0 {
			return ""
		}
		c = &transcriptCache{path: matches[0]}
		s.transcripts[sessionID] = c
	}

	fi, err := os.Stat(c.path)
	if err != nil {
		return c.model
	}
	if fi.Size() == c.size && fi.ModTime().Equal(c.mtime) {
		return c.model
	}
	if m := lastModel(c.path, fi.Size()); m != "" {
		c.model = m
	}
	c.size, c.mtime = fi.Size(), fi.ModTime()
	return c.model
}

// lastModel scans the final tailWindow bytes of a JSONL transcript backwards
// for the newest entry carrying message.model.
func lastModel(path string, size int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }() // read-only; close error is meaningless

	off := int64(0)
	if size > tailWindow {
		off = size - tailWindow
	}
	buf := make([]byte, size-off)
	if _, err := io.ReadFull(io.NewSectionReader(f, off, int64(len(buf))), buf); err != nil {
		return ""
	}

	lines := bytes.Split(buf, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if !bytes.Contains(lines[i], []byte(`"model":"`)) {
			continue
		}
		var entry struct {
			Message struct {
				Model string `json:"model"`
			} `json:"message"`
		}
		// A partial first line (window cut mid-line) simply fails to parse.
		if json.Unmarshal(lines[i], &entry) == nil && entry.Message.Model != "" {
			return entry.Message.Model
		}
	}
	return ""
}
