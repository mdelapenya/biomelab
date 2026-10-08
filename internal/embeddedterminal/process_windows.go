//go:build windows

// Package embeddedterminal owns processes attached to the in-app terminal.
package embeddedterminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Process owns a pseudoconsole and a job containing its child process tree.
// Wait reaps the direct child; WaitStopped waits for transport teardown.
type Process struct {
	input     *os.File
	output    *os.File
	console   windows.Handle
	job       windows.Handle
	child     windows.Handle
	mu        sync.Mutex // serializes Resize with pseudoconsole closure
	lifeMu    sync.Mutex // prevents Stop from using handles after teardown
	cleaned   bool
	force     sync.Once
	finish    sync.Once
	terminate sync.Once
	members   []windows.Handle // SYNCHRONIZE handles captured before the job is terminated
	readEnd   sync.Once
	done      chan struct{}
	stopped   chan struct{}
	forced    chan struct{}
	forceDone chan struct{}
	readEOF   chan struct{}
	err       error // published by closing done
}

// The SDK's JOBOBJECT_BASIC_ACCOUNTING_INFORMATION layout. x/sys/windows
// exposes the query API and information class, but not this result structure.
type jobBasicAccounting struct {
	totalUserTime, totalKernelTime            int64
	periodUserTime, periodKernelTime          int64
	totalPageFaults, totalProcesses           uint32
	activeProcesses, totalTerminatedProcesses uint32
}

func terminalSize(rows, cols uint16) windows.Coord {
	return windows.Coord{X: int16(min(max(cols, 1), 32767)), Y: int16(min(max(rows, 1), 32767))}
}

func shellCommand() []string {
	for _, name := range []string{"pwsh.exe", "powershell.exe"} {
		if path, err := exec.LookPath(name); err == nil {
			return []string{path, "-NoLogo"}
		}
	}
	if name := os.Getenv("COMSPEC"); name != "" {
		return []string{name}
	}
	return []string{"cmd.exe"}
}

func executable(argv0, dir string) (string, error) {
	if argv0 == "" {
		return "", errors.New("terminal executable is empty")
	}
	if strings.IndexByte(argv0, 0) >= 0 {
		return "", errors.New("terminal executable contains NUL")
	}
	if filepath.IsAbs(argv0) {
		return argv0, nil
	}
	if strings.ContainsAny(argv0, `\/`) {
		if dir != "" {
			return filepath.Abs(filepath.Join(dir, argv0))
		}
		return filepath.Abs(argv0)
	}
	return exec.LookPath(argv0)
}

// Start starts argv directly, preserving argument boundaries. Empty argv
// starts an interactive PowerShell (or the configured command processor).
func Start(ctx context.Context, dir string, argv []string, rows, cols uint16) (_ *Process, retErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(argv) == 0 {
		argv = shellCommand()
	}
	for _, arg := range argv {
		if strings.IndexByte(arg, 0) >= 0 {
			return nil, errors.New("terminal argument contains NUL")
		}
	}
	path, err := executable(argv[0], dir)
	if err != nil {
		return nil, err
	}
	if err := windows.NewLazySystemDLL("kernel32.dll").NewProc("CreatePseudoConsole").Find(); err != nil {
		return nil, fmt.Errorf("Windows ConPTY is unavailable; use Open in external terminal: %w", err)
	}
	if dir != "" {
		if _, err := os.Stat(dir); err != nil {
			return nil, fmt.Errorf("terminal directory: %w", err)
		}
	}

	var ptyInput, inputWrite, outputRead, ptyOutput windows.Handle
	var console, job windows.Handle
	var pi windows.ProcessInformation
	var attrs *windows.ProcThreadAttributeListContainer
	defer func() {
		if retErr == nil {
			return
		}
		// The child is still suspended on all startup failure paths.
		if pi.Process != 0 {
			_ = windows.TerminateProcess(pi.Process, 1)
			windows.CloseHandle(pi.Process)
		}
		if pi.Thread != 0 {
			windows.CloseHandle(pi.Thread)
		}
		if attrs != nil {
			attrs.Delete()
		}
		for _, h := range []windows.Handle{ptyInput, inputWrite, outputRead, ptyOutput} {
			if h != 0 {
				windows.CloseHandle(h)
			}
		}
		if console != 0 {
			windows.ClosePseudoConsole(console)
		}
		if job != 0 {
			windows.CloseHandle(job)
		}
	}()
	if err = windows.CreatePipe(&ptyInput, &inputWrite, nil, 0); err != nil {
		return nil, err
	}
	if err = windows.CreatePipe(&outputRead, &ptyOutput, nil, 0); err != nil {
		return nil, err
	}
	if err = windows.CreatePseudoConsole(terminalSize(rows, cols), ptyInput, ptyOutput, 0, &console); err != nil {
		return nil, fmt.Errorf("create Windows ConPTY (use Open in external terminal if unsupported): %w", err)
	}
	attrs, err = windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	// UpdateProcThreadAttribute expects the HPCON value itself as lpValue, not
	// a pointer to it. Reinterpret the handle bits as a pointer-sized value so
	// the child attaches to this pseudoconsole instead of inheriting our stdio.
	if err = attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&console)), unsafe.Sizeof(console)); err != nil {
		return nil, err
	}
	job, err = windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	if err = setJobLimits(job, 0); err != nil {
		return nil, err
	}
	app, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	cmdline, err := windows.UTF16FromString(windows.ComposeCommandLine(argv))
	if err != nil {
		return nil, err
	}
	var cwd *uint16
	if dir != "" {
		cwd, err = windows.UTF16PtrFromString(dir)
		if err != nil {
			return nil, err
		}
	}
	si := windows.StartupInfoEx{}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.ProcThreadAttributeList = attrs.List()
	// When our own stdio is redirected (go test, a parent shell with pipes),
	// the child would inherit those handles instead of the pseudoconsole's.
	// Requesting explicit NULL std handles makes ConPTY supply its own.
	si.Flags |= windows.STARTF_USESTDHANDLES
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_SUSPENDED | windows.CREATE_UNICODE_ENVIRONMENT)
	if err = windows.CreateProcess(app, &cmdline[0], nil, nil, false, flags, nil, cwd, &si.StartupInfo, &pi); err != nil {
		return nil, err
	}
	if err = windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		return nil, fmt.Errorf("assign terminal child to job: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if _, err = windows.ResumeThread(pi.Thread); err != nil {
		return nil, err
	}
	windows.CloseHandle(pi.Thread)
	pi.Thread = 0
	attrs.Delete()
	attrs = nil
	windows.CloseHandle(ptyInput)
	ptyInput = 0
	windows.CloseHandle(ptyOutput)
	ptyOutput = 0
	p := &Process{
		input: os.NewFile(uintptr(inputWrite), "conpty-input"), output: os.NewFile(uintptr(outputRead), "conpty-output"),
		console: console, job: job, child: pi.Process, done: make(chan struct{}), stopped: make(chan struct{}),
		forced: make(chan struct{}), forceDone: make(chan struct{}), readEOF: make(chan struct{}),
	}
	go p.reap()
	go func() {
		select {
		case <-ctx.Done():
			p.Stop()
		case <-p.stopped:
		}
	}()
	return p, nil
}

func (p *Process) reap() {
	status, err := windows.WaitForSingleObject(p.child, windows.INFINITE)
	if err == nil && status != windows.WAIT_OBJECT_0 {
		err = fmt.Errorf("wait for terminal process: unexpected status %d", status)
	}
	if err == nil {
		var code uint32
		if err = windows.GetExitCodeProcess(p.child, &code); err == nil && code != 0 {
			err = fmt.Errorf("terminal process exited with code %d", code)
		}
	}
	p.err = err
	close(p.done)
	p.beginTeardown()
}

func (p *Process) Read(b []byte) (int, error) {
	n, err := p.output.Read(b)
	if errors.Is(err, windows.ERROR_BROKEN_PIPE) || errors.Is(err, windows.ERROR_NO_DATA) || errors.Is(err, os.ErrClosed) || errors.Is(err, windows.ERROR_OPERATION_ABORTED) {
		err = io.EOF
	}
	if err != nil {
		p.readEnd.Do(func() { close(p.readEOF) })
	}
	return n, err
}

func (p *Process) Write(b []byte) (int, error) { return p.input.Write(b) }

func (p *Process) Resize(rows, cols uint16) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.console == 0 {
		return io.ErrClosedPipe
	}
	return windows.ResizePseudoConsole(p.console, terminalSize(rows, cols))
}

// Stop returns immediately. It forces cancellation of any pending pipe I/O;
// normal child exit instead lets the reader drain ConPTY's final output.
func (p *Process) Stop() {
	p.lifeMu.Lock()
	defer p.lifeMu.Unlock()
	if p.cleaned {
		return
	}
	p.force.Do(func() {
		close(p.forced)
		go func() {
			defer close(p.forceDone)
			p.terminateJob()
			_ = p.input.Close()
			_ = p.output.Close()
			p.beginTeardown()
		}()
	})
}

func (p *Process) beginTeardown() {
	p.finish.Do(func() { go p.teardown() })
}

func (p *Process) teardown() {
	<-p.done
	// The root can exit while a background child keeps the job and ConPTY
	// alive. Request termination before waiting for the job to empty. Keep
	// output open so the reader can consume the root's final frame.
	p.terminateJob()
	p.mu.Lock()
	windows.ClosePseudoConsole(p.console)
	p.console = 0
	p.mu.Unlock()
	// On modern Windows ClosePseudoConsole can return before its final frame
	// reaches the pipe. Let Read consume through EOF before closing our end.
	// Stop can override this wait when the UI is no longer reading.
	select {
	case <-p.readEOF:
	case <-p.forced:
	}
	// TerminateJobObject may return before a descendant releases its cwd.
	// WaitStopped is the worktree-deletion barrier, so do not signal it until
	// every member process object is signaled and the job reports no active
	// members. Accounting and the job PID list both drop before the process
	// object is signaled, so wait on handles captured while members were alive.
	// One budget for all members, so a large job cannot stretch this to
	// members×timeout.
	deadline := time.Now().Add(memberWaitBudget)
	for _, h := range p.members {
		remaining := max(time.Until(deadline), 0)
		_, _ = windows.WaitForSingleObject(h, uint32(remaining/time.Millisecond))
		windows.CloseHandle(h)
	}
	p.members = nil
	// The accounting poll shares the same deadline: a member that cannot be
	// ended (or a failing query) must not hold WaitStopped forever, since
	// restart waits on it with no timeout of its own. Worktree deletion has
	// its own bound and reports cleanup as pending.
	for time.Now().Before(deadline) {
		var accounting jobBasicAccounting
		err := windows.QueryInformationJobObject(p.job, windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil)
		if err != nil || accounting.activeProcesses == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.lifeMu.Lock()
	select {
	case <-p.forced:
		p.lifeMu.Unlock()
		<-p.forceDone
		p.lifeMu.Lock()
	default:
	}
	p.cleaned = true
	p.lifeMu.Unlock()
	_ = p.input.Close()
	_ = p.output.Close()
	windows.CloseHandle(p.child)
	windows.CloseHandle(p.job)
	close(p.stopped)
}

// terminateJob captures SYNCHRONIZE handles for every current job member and
// then terminates the job, exactly once. Teardown waits on those handles
// (within memberWaitBudget) before releasing WaitStopped: job accounting can
// reach zero before a member's process object is signaled, so an uncaptured
// member could still hold the worktree when WaitStopped opens.
func (p *Process) terminateJob() {
	p.terminate.Do(func() {
		// Freeze membership first: a member could otherwise spawn a child
		// between the snapshot and TerminateJobObject, and that child would
		// have no handle to wait on.
		if err := freezeJob(p.job); err != nil {
			log.Printf("embedded terminal: freezing job before teardown: %v", err)
		}
		ids, err := jobProcessIDs(p.job, jobPIDListInitial)
		if err != nil {
			log.Printf("embedded terminal: listing job members before teardown: %v", err)
		}
		for _, pid := range ids {
			h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
			if err != nil {
				continue // already gone
			}
			// The member may have exited after the listing and its id been
			// reused; only wait on processes that are still in this job.
			if !processInJob(h, p.job) {
				windows.CloseHandle(h)
				continue
			}
			p.members = append(p.members, h)
		}
		_ = windows.TerminateJobObject(p.job, 1)
	})
}

// freezeJob stops the job from gaining members. The active-process limit only
// applies to new associations: existing members keep running even when there
// are more of them than the limit, while a process that would join the job (a
// new child of a member) is refused or terminated as it is associated. The
// following snapshot therefore lists every member that can still run.
func freezeJob(job windows.Handle) error { return setJobLimits(job, 1) }

// setJobLimits applies the job limits Biomelab relies on in one place, so
// creation and teardown cannot drift apart: SetInformationJobObject
// replaces the whole structure. Every job kills its members when its last
// handle closes; activeLimit > 0 also caps the active process count.
func setJobLimits(job windows.Handle, activeLimit uint32) error {
	limit := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limit.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if activeLimit > 0 {
		limit.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_ACTIVE_PROCESS
		limit.BasicLimitInformation.ActiveProcessLimit = activeLimit
	}
	_, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limit)), uint32(unsafe.Sizeof(limit)))
	return err
}

// memberWaitBudget bounds the total wait for captured job members.
const memberWaitBudget = 5 * time.Second

var procIsProcessInJob = windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")

func processInJob(process, job windows.Handle) bool {
	var in int32
	r, _, _ := procIsProcessInJob.Call(uintptr(process), uintptr(job), uintptr(unsafe.Pointer(&in)))
	return r != 0 && in != 0
}

// jobPIDListInitial is the PID capacity of the first job query; the buffer
// grows when the job holds more processes.
const jobPIDListInitial = 64

// jobProcessIDs returns the ids of every process currently in job. The
// JOBOBJECT_BASIC_PROCESS_ID_LIST result is two DWORD counts followed by a
// ULONG_PTR array at offset 8. When the buffer is too small the call reports
// how many processes are assigned, so retry with room for all of them (plus
// slack, since the job may still be growing).
func jobProcessIDs(job windows.Handle, capacity int) ([]uintptr, error) {
	const headerBytes = 8
	headerWords := headerBytes / int(unsafe.Sizeof(uintptr(0)))
	for attempt := 0; ; attempt++ {
		buf := make([]uintptr, headerWords+max(capacity, 1))
		err := windows.QueryInformationJobObject(job, windows.JobObjectBasicProcessIdList,
			uintptr(unsafe.Pointer(&buf[0])), uint32(len(buf))*uint32(unsafe.Sizeof(uintptr(0))), nil)
		if err != nil && !errors.Is(err, windows.ERROR_MORE_DATA) {
			return nil, err
		}
		counts := (*[2]uint32)(unsafe.Pointer(&buf[0]))
		assigned, listed := int(counts[0]), int(counts[1])
		ids := append([]uintptr(nil), buf[headerWords:headerWords+min(listed, len(buf)-headerWords)]...)
		if listed >= assigned {
			return ids, nil
		}
		if attempt >= 8 {
			// The job keeps outgrowing the buffer; report the partial list.
			return ids, fmt.Errorf("job still growing after %d attempts: listed %d of %d members", attempt+1, listed, assigned)
		}
		capacity = assigned + 16
	}
}

func (p *Process) Close() error { p.Stop(); return nil }
func (p *Process) Wait() error  { <-p.done; return p.err }
func (p *Process) WaitStopped() { <-p.stopped }
