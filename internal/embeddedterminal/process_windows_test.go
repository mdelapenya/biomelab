//go:build windows

package embeddedterminal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// This test binary is a native ConPTY client, so no PowerShell installation
// or shell quoting rules are needed to verify argument and environment safety.
func TestConPTYHelper(t *testing.T) {
	if os.Getenv("BIOMELAB_CONPTY_HELPER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	if len(args) < 2 {
		os.Exit(90)
	}
	switch args[1] {
	case "facts":
		cwd, _ := os.Getwd()
		fmt.Printf("FACT|%s|%s|%s|%s|END\n", cwd, os.Getenv("BIOMELAB_CONPTY_VALUE"), strings.Join(args[2:], "~"), sizeText())
	case "resize":
		fmt.Printf("READY|%s|END\n", sizeText())
		var b [1]byte
		_, _ = os.Stdin.Read(b[:])
		fmt.Printf("RESIZED|%s|END\n", sizeText())
	case "exit":
		fmt.Print("EXIT|END\n")
		os.Exit(37)
	case "busy":
		fmt.Print("READY|END\n")
		for {
			_, _ = os.Stdout.Write(bytes.Repeat([]byte("X"), 65536))
		}
	case "child":
		fmt.Print("CHILD|READY|END\n")
		for {
			time.Sleep(time.Hour)
		}
	case "parent":
		cmd := exec.Command(os.Args[0], "-test.run=^TestConPTYHelper$", "--", "child")
		cmd.Env = append(os.Environ(), "BIOMELAB_CONPTY_HELPER=1")
		if err := cmd.Start(); err != nil {
			fmt.Printf("CHILDERR|%v|END\n", err)
			os.Exit(91)
		}
		fmt.Printf("PID|%d|END\n", cmd.Process.Pid)
		for {
			time.Sleep(time.Hour)
		}
	case "parentexit":
		cmd := exec.Command(os.Args[0], "-test.run=^TestConPTYHelper$", "--", "child")
		cmd.Env = append(os.Environ(), "BIOMELAB_CONPTY_HELPER=1")
		if err := cmd.Start(); err != nil {
			fmt.Printf("CHILDERR|%v|END\n", err)
			os.Exit(91)
		}
		fmt.Printf("PID|%d|END\n", cmd.Process.Pid)
		_ = cmd.Process.Release()
		var b [1]byte
		_, _ = os.Stdin.Read(b[:])
		fmt.Print("ROOT|FINAL|END\n")
	default:
		os.Exit(92)
	}
	os.Exit(0)
}

func sizeText() string {
	h, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return "error:" + err.Error()
	}
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(h, &info); err != nil {
		return "error:" + err.Error()
	}
	return fmt.Sprintf("%dx%d", info.Size.Y, info.Size.X)
}

func helperArgs(mode string, extra ...string) []string {
	args := []string{os.Args[0], "-test.run=^TestConPTYHelper$", "--", mode}
	return append(args, extra...)
}

func requireConPTY(t *testing.T, ctx context.Context, dir string, args []string, rows, cols uint16) *Process {
	t.Helper()
	p, err := Start(ctx, dir, args, rows, cols)
	if err != nil {
		t.Fatalf("native Windows ConPTY startup failed: %v", err)
	}
	t.Cleanup(func() {
		p.Stop()
		waitSignal(t, p.stopped, "transport teardown")
	})
	return p
}

func waitSignal(t *testing.T, ch <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for %s", name)
	}
}

func readMarker(t *testing.T, p *Process, marker string) string {
	t.Helper()
	result := make(chan string, 1)
	go func() {
		var out strings.Builder
		buf := make([]byte, 4096)
		for !strings.Contains(out.String(), marker) {
			n, err := p.Read(buf)
			out.Write(buf[:n])
			if err != nil {
				break
			}
		}
		result <- out.String()
	}()
	select {
	case s := <-result:
		if !strings.Contains(s, marker) {
			t.Fatalf("output %q missing marker %q", s, marker)
		}
		return s
	case <-time.After(5 * time.Second):
		p.Stop()
		t.Fatalf("timeout reading %q", marker)
		return ""
	}
}

func TestConPTYArgumentsDirectoryEnvironmentAndSize(t *testing.T) {
	t.Setenv("BIOMELAB_CONPTY_HELPER", "1")
	t.Setenv("BIOMELAB_CONPTY_VALUE", "value snowman ☃")
	dir := t.TempDir()
	p := requireConPTY(t, context.Background(), dir, helperArgs("facts", `space and "quote"`, "ユニコード"), 23, 81)
	out := readMarker(t, p, "|END")
	for _, want := range []string{dir, "value snowman ☃", `space and "quote"~ユニコード`, "23x81"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q missing %q", out, want)
		}
	}
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestConPTYResizeAndExitStatus(t *testing.T) {
	t.Setenv("BIOMELAB_CONPTY_HELPER", "1")
	p := requireConPTY(t, context.Background(), t.TempDir(), helperArgs("resize"), 24, 80)
	if out := readMarker(t, p, "READY|24x80|END"); !strings.Contains(out, "READY|24x80|END") {
		t.Fatal(out)
	}
	if err := p.Resize(31, 91); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Write([]byte("x\r")); err != nil {
		t.Fatal(err)
	}
	if out := readMarker(t, p, "RESIZED|31x91|END"); !strings.Contains(out, "RESIZED|31x91|END") {
		t.Fatal(out)
	}
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
	q := requireConPTY(t, context.Background(), "", helperArgs("exit"), 24, 80)
	readMarker(t, q, "EXIT|END")
	if err := q.Wait(); err == nil || !strings.Contains(err.Error(), "37") {
		t.Fatalf("nonzero exit status = %v", err)
	}
}

func TestConPTYNaturalExitDrainsOutputAndReachesEOF(t *testing.T) {
	t.Setenv("BIOMELAB_CONPTY_HELPER", "1")
	p := requireConPTY(t, context.Background(), "", helperArgs("exit"), 24, 80)
	output := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		b, err := io.ReadAll(p)
		output <- struct {
			data []byte
			err  error
		}{b, err}
	}()
	select {
	case result := <-output:
		if result.err != nil || !bytes.Contains(result.data, []byte("EXIT|END")) {
			t.Fatalf("natural-exit output %q, error %v", result.data, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("natural exit did not close ConPTY output")
	}
	waitSignal(t, p.stopped, "natural-exit teardown")
	if err := p.Wait(); err == nil || !strings.Contains(err.Error(), "37") {
		t.Fatalf("natural-exit status = %v", err)
	}
}

func TestConPTYRootExitWithDescendantClosesSession(t *testing.T) {
	t.Setenv("BIOMELAB_CONPTY_HELPER", "1")
	p := requireConPTY(t, context.Background(), "", helperArgs("parentexit"), 24, 80)
	out := readMarker(t, p, "|END")
	i := strings.Index(out, "PID|")
	if i < 0 {
		t.Fatalf("missing descendant PID: %q", out)
	}
	part := strings.SplitN(out[i+4:], "|", 2)[0]
	pid, err := strconv.Atoi(part)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	if status, err := windows.WaitForSingleObject(h, 0); err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		t.Fatalf("descendant was not alive before root exit: status=%d error=%v", status, err)
	}
	if _, err := p.Write([]byte("x\r")); err != nil {
		t.Fatal(err)
	}
	result := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		b, err := io.ReadAll(p)
		result <- struct {
			data []byte
			err  error
		}{b, err}
	}()
	select {
	case got := <-result:
		if got.err != nil || !bytes.Contains(got.data, []byte("ROOT|FINAL|END")) {
			t.Fatalf("root final output %q, error %v", got.data, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("root exit with descendant did not produce EOF")
	}
	waitSignal(t, p.stopped, "root-exit descendant teardown")
	if status, err := windows.WaitForSingleObject(h, 0); err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("descendant survived natural exit: status=%d error=%v", status, err)
	}
	if err := p.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestConPTYCanceledAndFailedStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Start(ctx, "", helperArgs("facts"), 24, 80); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled startup = %v", err)
	}
	if _, err := Start(context.Background(), filepath.Join(t.TempDir(), "missing"), helperArgs("facts"), 24, 80); err == nil {
		t.Fatal("missing directory accepted")
	}
	if _, err := Start(context.Background(), "", []string{"not-a-real-terminal-command-92741.exe"}, 24, 80); err == nil {
		t.Fatal("missing executable accepted")
	}
	// An absolute path passes executable resolution and exercises cleanup
	// after the pipes, ConPTY and job have been created.
	if _, err := Start(context.Background(), "", []string{filepath.Join(t.TempDir(), "missing.exe")}, 24, 80); err == nil {
		t.Fatal("CreateProcess failure accepted")
	}
}

func TestConPTYStopKillsDescendantsAndUnblocksIO(t *testing.T) {
	t.Setenv("BIOMELAB_CONPTY_HELPER", "1")
	ctx, cancel := context.WithCancel(context.Background())
	p := requireConPTY(t, ctx, "", helperArgs("parent"), 24, 80)
	out := readMarker(t, p, "|END")
	i := strings.Index(out, "PID|")
	if i < 0 {
		t.Fatalf("missing descendant PID: %q", out)
	}
	part := strings.SplitN(out[i+4:], "|", 2)[0]
	pid, err := strconv.Atoi(part)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	cancel()
	p.Stop()
	p.Stop()
	waitSignal(t, p.stopped, "stop after cancellation")
	status, err := windows.WaitForSingleObject(h, 0)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("descendant survived Stop: status=%d error=%v", status, err)
	}
	if _, err := io.ReadAll(p); err != nil {
		t.Fatalf("read after Stop: %v", err)
	}
}

func TestConPTYStopUnblocksBlockedRead(t *testing.T) {
	t.Setenv("BIOMELAB_CONPTY_HELPER", "1")
	p := requireConPTY(t, context.Background(), "", helperArgs("child"), 24, 80)
	readMarker(t, p, "CHILD|READY|END")
	readDone := make(chan error, 1)
	var inRead atomic.Int64 // UnixNano when the current Read started, 0 outside Read
	go func() {
		// ConPTY may still deliver bytes queued after the marker (line
		// endings, VT sequences); drain them, then stay blocked in Read.
		b := make([]byte, 4096)
		for {
			inRead.Store(time.Now().UnixNano())
			_, err := p.Read(b)
			inRead.Store(0)
			if err != nil {
				readDone <- err
				return
			}
		}
	}()
	// Only call Stop once the reader has sat blocked in Read with nothing to
	// read, so the test really covers Stop unblocking a pending read.
	parked := time.Now().Add(5 * time.Second)
	for {
		if since := inRead.Load(); since != 0 && time.Since(time.Unix(0, since)) >= 200*time.Millisecond {
			break
		}
		if time.Now().After(parked) {
			t.Fatal("reader never blocked in Read")
		}
		time.Sleep(10 * time.Millisecond)
	}
	p.Stop()
	waitSignal(t, p.stopped, "blocked-read teardown")
	select {
	case err := <-readDone:
		if err != io.EOF {
			t.Fatalf("closed read returned %v, want EOF", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked Read survived Stop")
	}
}

func TestConPTYStopWhileProducingLargeOutput(t *testing.T) {
	t.Setenv("BIOMELAB_CONPTY_HELPER", "1")
	p := requireConPTY(t, context.Background(), "", helperArgs("busy"), 24, 80)
	readMarker(t, p, "READY|END")
	resizeDone := make(chan struct{})
	go func() {
		defer close(resizeDone)
		for i := 0; i < 5; i++ {
			_ = p.Resize(uint16(24+i), uint16(80+i))
		}
	}()
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		_, _ = p.Write(bytes.Repeat([]byte("w"), 16<<20))
	}()
	p.Stop()
	waitSignal(t, p.stopped, "large-output teardown")
	waitSignal(t, resizeDone, "concurrent resize")
	waitSignal(t, writeDone, "concurrent write")
	if _, err := io.ReadAll(p); err != nil {
		t.Fatal(err)
	}
}

// More members than the first query can hold must still all be captured,
// or WaitStopped could open while an uncaptured member holds the worktree.
func TestJobProcessIDsGrowsPastInitialCapacity(t *testing.T) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(job)
	defer windows.TerminateJobObject(job, 1)
	want := map[uintptr]bool{}
	for i := 0; i < 3; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestConPTYHelper$", "--", "child")
		cmd.Env = append(os.Environ(), "BIOMELAB_CONPTY_HELPER=1")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
		if err != nil {
			t.Fatal(err)
		}
		if err := windows.AssignProcessToJobObject(job, h); err != nil {
			windows.CloseHandle(h)
			t.Fatal(err)
		}
		windows.CloseHandle(h)
		want[uintptr(cmd.Process.Pid)] = true
	}
	got, err := jobProcessIDs(job, 1) // force the growth path
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d job members %v, want %d", len(got), got, len(want))
	}
	for _, pid := range got {
		if !want[pid] {
			t.Fatalf("unexpected job member %d in %v", pid, got)
		}
	}
}

// A reused PID must not be waited on as a member: processInJob tells job
// members apart from unrelated processes.
func TestProcessInJobDistinguishesMembers(t *testing.T) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(job)
	defer windows.TerminateJobObject(job, 1)
	start := func() *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestConPTYHelper$", "--", "child")
		cmd.Env = append(os.Environ(), "BIOMELAB_CONPTY_HELPER=1")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		return cmd
	}
	open := func(pid int) windows.Handle {
		h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { windows.CloseHandle(h) })
		return h
	}
	member, outsider := open(start().Process.Pid), open(start().Process.Pid)
	if err := windows.AssignProcessToJobObject(job, member); err != nil {
		t.Fatal(err)
	}
	if !processInJob(member, job) {
		t.Fatal("job member not reported as in the job")
	}
	if processInJob(outsider, job) {
		t.Fatal("unrelated process reported as a job member")
	}
}
