//go:build linux

package discover

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"time"
)

// clockTicksPerSec is USER_HZ, fixed at 100 on Linux regardless of the
// kernel's internal HZ.
const clockTicksPerSec = 100

// procStartTime returns the start time of a live process from
// /proc/<pid>/stat (field 22, clock ticks since boot) plus the boot time
// from /proc/stat.
func procStartTime(pid int) (time.Time, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return time.Time{}, err
	}
	// comm (field 2) may contain spaces; fields resume after the last ')'.
	i := bytes.LastIndexByte(data, ')')
	if i < 0 {
		return time.Time{}, fmt.Errorf("malformed stat for pid %d", pid)
	}
	fields := bytes.Fields(data[i+1:]) // fields[0] is stat field 3 ("state")
	const starttimeIdx = 22 - 3
	if len(fields) <= starttimeIdx {
		return time.Time{}, fmt.Errorf("short stat for pid %d", pid)
	}
	ticks, err := strconv.ParseInt(string(fields[starttimeIdx]), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	btime, err := bootTime()
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(btime+ticks/clockTicksPerSec, 0), nil
}

func bootTime() (int64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, err
	}
	for line := range bytes.SplitSeq(data, []byte("\n")) {
		if rest, ok := bytes.CutPrefix(line, []byte("btime ")); ok {
			return strconv.ParseInt(string(bytes.TrimSpace(rest)), 10, 64)
		}
	}
	return 0, fmt.Errorf("btime not found in /proc/stat")
}
