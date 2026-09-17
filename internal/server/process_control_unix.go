//go:build !windows

package server

import (
	"fmt"
	"os"
	"syscall"
)

func suspendProcess(process *os.Process) error {
	if process == nil {
		return fmt.Errorf("suspend process is unavailable")
	}
	return process.Signal(syscall.SIGSTOP)
}

func resumeProcess(process *os.Process) error {
	if process == nil {
		return fmt.Errorf("resume process is unavailable")
	}
	return process.Signal(syscall.SIGCONT)
}
