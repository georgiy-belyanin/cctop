//go:build darwin

package discover

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// procStartTime returns the start time of a live process via ps(1), which
// prints lstart in ANSIC format ("Wed Jun 10 09:47:09 2026").
func procStartTime(pid int) (time.Time, error) {
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return time.Time{}, err
	}
	return time.ParseInLocation(time.ANSIC, strings.TrimSpace(string(out)), time.Local)
}
