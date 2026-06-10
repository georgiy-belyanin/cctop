package discover

import (
	"errors"
	"syscall"
)

// alive reports whether a process with the given PID exists. EPERM means the
// process exists but belongs to someone else — still alive.
func alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
