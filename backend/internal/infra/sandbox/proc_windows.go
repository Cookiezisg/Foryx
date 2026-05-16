//go:build windows

package sandbox

import (
	"fmt"
	"os/exec"
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	masterJobOnce sync.Once
	masterJobErr  error
	masterJob     windows.Handle
)

// EnsureMasterJob creates the per-process Job Object on first call (idempotent).
//
// EnsureMasterJob 首次调用创建 per-process Job Object（幂等）。
func EnsureMasterJob() error {
	masterJobOnce.Do(initMasterJob)
	return masterJobErr
}

func initMasterJob() {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		masterJobErr = fmt.Errorf("sandbox.initMasterJob: CreateJobObject: %w", err)
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	_, err = windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(h)
		masterJobErr = fmt.Errorf("sandbox.initMasterJob: SetInformationJobObject: %w", err)
		return
	}
	if err := windows.AssignProcessToJobObject(h, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(h)
		masterJobErr = fmt.Errorf("sandbox.initMasterJob: AssignProcessToJobObject(self): %w", err)
		return
	}
	masterJob = h
}

// setupProcessGroup ensures the master Job Object exists; child processes inherit it (best-effort).
//
// setupProcessGroup 确保 master Job Object 已就绪；child 自动继承（best-effort）。
func setupProcessGroup(cmd *exec.Cmd) {
	_ = EnsureMasterJob()
}

// killProcessGroup runs `taskkill /T /F /PID <pid>` to kill cmd and its descendants.
//
// killProcessGroup 跑 `taskkill /T /F /PID <pid>` 杀 cmd 及其所有后代。
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pid := strconv.Itoa(cmd.Process.Pid)
	out, err := exec.Command("taskkill", "/T", "/F", "/PID", pid).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sandbox.killProcessGroup: taskkill: %w (output: %s)", err, out)
	}
	return nil
}
