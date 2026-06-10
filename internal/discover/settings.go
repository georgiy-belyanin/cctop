package discover

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// settings holds the few keys we care about from Claude Code settings files.
// All other keys are ignored.
type settings struct {
	EffortLevel string `json:"effortLevel"`
	Model       string `json:"model"`
}

// loadSettings reads one settings file; a missing or unparsable file yields
// the zero value.
func loadSettings(path string) settings {
	var s settings
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

// effectiveSettings layers project settings inside cwd over the user-level
// ones: user < project .claude/settings.json < .claude/settings.local.json.
func effectiveSettings(user settings, cwd string) settings {
	out := user
	if cwd == "" {
		return out
	}
	for _, p := range []string{
		filepath.Join(cwd, ".claude", "settings.json"),
		filepath.Join(cwd, ".claude", "settings.local.json"),
	} {
		s := loadSettings(p)
		if s.EffortLevel != "" {
			out.EffortLevel = s.EffortLevel
		}
		if s.Model != "" {
			out.Model = s.Model
		}
	}
	return out
}
