//go:build windows

package server

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

const processSuspendResume = 0x0800

var (
	ntdll            = windows.NewLazySystemDLL("ntdll.dll")
	ntSuspendProcess = ntdll.NewProc("NtSuspendProcess")
	ntResumeProcess  = ntdll.NewProc("NtResumeProcess")
)

func suspendProcess(process *os.Process) error {
	return callProcessControl(process, ntSuspendProcess, "suspend")
}

func resumeProcess(process *os.Process) error {
	return callProcessControl(process, ntResumeProcess, "resume")
}

func callProcessControl(process *os.Process, procedure *windows.LazyProc, action string) error {
	if process == nil || process.Pid <= 0 {
		return fmt.Errorf("%s process is unavailable", action)
	}
	handle, err := windows.OpenProcess(processSuspendResume, false, uint32(process.Pid))
	if err != nil {
		return fmt.Errorf("open process to %s: %w", action, err)
	}
	defer windows.CloseHandle(handle)
	status, _, callErr := procedure.Call(uintptr(handle))
	if status != 0 {
		return fmt.Errorf("%s process: NTSTATUS 0x%x: %v", action, status, callErr)
	}
	return nil
}
