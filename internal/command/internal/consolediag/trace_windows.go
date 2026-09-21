// Package consolediag records allowlisted facts from the Windows test processes.
// It is used only by the console regression test and its GUI-subsystem launcher.
package consolediag

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const DirectoryEnv = "BIOMELAB_CONSOLE_TRACE_DIR"

type State struct {
	Schema           int       `json:"schema"`
	Time             time.Time `json:"time"`
	Role             string    `json:"role"`
	Phase            string    `json:"phase"`
	PID              int       `json:"pid"`
	PPID             int       `json:"ppid"`
	ChildPID         int       `json:"child_pid,omitempty"`
	Window           uintptr   `json:"console_window"`
	InputCPError     uint32    `json:"input_cp_error,omitempty"`
	OutputCPError    uint32    `json:"output_cp_error,omitempty"`
	ErrorCode        uint32    `json:"error_code,omitempty"`
	ForegroundWindow uintptr   `json:"foreground_window"`
	ForegroundPID    uint32    `json:"foreground_pid"`
	InputCP          uint32    `json:"input_codepage"`
	OutputCP         uint32    `json:"output_codepage"`
	ConsoleProcesses uintptr   `json:"console_process_count"`
	ProcessListError uint32    `json:"console_process_error,omitempty"`
	CreationFlags    uint32    `json:"creation_flags,omitempty"`
	ExitCode         int       `json:"exit_code"`
	DurationMS       int64     `json:"duration_ms,omitempty"`
	GoVersion        string    `json:"go_version"`
	Arch             string    `json:"arch"`
	WindowsBuild     uint32    `json:"windows_build"`
}

func Snapshot(role, phase string) State {
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	window, _, _ := kernel.NewProc("GetConsoleWindow").Call()
	input, inputErr := windows.GetConsoleCP()
	output, outputErr := windows.GetConsoleOutputCP()
	user := windows.NewLazySystemDLL("user32.dll")
	foreground, _, _ := user.NewProc("GetForegroundWindow").Call()
	var foregroundPID uint32
	if foreground != 0 {
		_, _, _ = user.NewProc("GetWindowThreadProcessId").Call(foreground, uintptr(unsafe.Pointer(&foregroundPID)))
	}
	var pid uint32
	count, _, listErr := kernel.NewProc("GetConsoleProcessList").Call(uintptr(unsafe.Pointer(&pid)), 1)
	var errorCode uint32
	if count == 0 {
		if errno, ok := listErr.(windows.Errno); ok {
			errorCode = uint32(errno)
		}
	}
	return State{
		Schema: 1, Time: time.Now().UTC(), Role: role, Phase: phase, PID: os.Getpid(), PPID: os.Getppid(),
		Window: window, InputCP: input, OutputCP: output, ConsoleProcesses: count, ProcessListError: errorCode,
		InputCPError: errorCodeOf(inputErr), OutputCPError: errorCodeOf(outputErr), ForegroundWindow: foreground, ForegroundPID: foregroundPID,
		GoVersion: runtime.Version(), Arch: runtime.GOARCH, WindowsBuild: windows.RtlGetVersion().BuildNumber,
	}
}

func errorCodeOf(err error) uint32 {
	var code windows.Errno
	if errors.As(err, &code) {
		return uint32(code)
	}
	return 0
}

// Write appends to a file owned by this process; there is no shared cross-process
// stream. No command arguments, paths, environment, titles or I/O are recorded.
func (s State) Write() error {
	dir := os.Getenv(DirectoryEnv)
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, fmt.Sprintf("process-%d.jsonl", os.Getpid())), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return json.NewEncoder(f).Encode(s)
}

// Emit also sends the snapshot through the helper's JSON-only stdout protocol.
func (s State) Emit(out io.Writer) error {
	if err := s.Write(); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(s)
}

// Run logs start/wait stages and the exact child creation flags. Failures retain
// their phase and numeric exit status without serializing executable paths.
func Run(role string, cmd *exec.Cmd) error {
	started := time.Now()
	state := Snapshot(role, "before_start")
	if cmd.SysProcAttr != nil {
		state.CreationFlags = cmd.SysProcAttr.CreationFlags
	}
	if err := state.Write(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		state.Phase, state.ExitCode, state.Time = "start_failed", -1, time.Now().UTC()
		state.ErrorCode = errorCodeOf(err)
		_ = state.Write()
		return err
	}
	state.Phase, state.ChildPID, state.Time = "started", cmd.Process.Pid, time.Now().UTC()
	traceErr := state.Write()
	err := cmd.Wait()
	final := Snapshot(role, "exited")
	final.ChildPID, final.CreationFlags = cmd.Process.Pid, state.CreationFlags
	final.ExitCode, final.DurationMS = -1, time.Since(started).Milliseconds()
	if cmd.ProcessState != nil {
		final.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		final.Phase = "wait_failed"
		final.ErrorCode = errorCodeOf(err)
	}
	if writeErr := final.Write(); traceErr == nil {
		traceErr = writeErr
	}
	if err != nil {
		return err
	}
	return traceErr
}
