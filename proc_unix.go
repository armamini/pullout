//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// isolate runs the tool in its own session. That detaches it from the
// controlling terminal (so progress bars drawn on /dev/tty cannot corrupt our
// output) and makes it a process-group leader, so cancellation kills any
// helpers it spawned (headless browsers, worker processes) with it.
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
