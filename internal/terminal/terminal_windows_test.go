package terminal

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/mdelapenya/biomelab/internal/process"
)

var (
	procEnumWindows       = user32.NewProc("EnumWindows")
	procGetForegroundWin  = user32.NewProc("GetForegroundWindow")
	procIsIconic          = user32.NewProc("IsIconic")
	enumMu                sync.Mutex
	enumResult            []windows.Handle
	enumCollectorCallback = syscall.NewCallback(func(h windows.Handle, _ uintptr) uintptr {
		enumResult = append(enumResult, h)
		return 1
	})
)

// cascadiaWindows returns the visible top-level Windows Terminal windows,
// keyed by HWND. Diffing this across a launch identifies our window without
// relying on the tab title, which the shell may overwrite.
func cascadiaWindows() map[windows.Handle]struct{} {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumResult = enumResult[:0]
	procEnumWindows.Call(enumCollectorCallback, 0)
	out := make(map[windows.Handle]struct{})
	for _, h := range enumResult {
		v, _, _ := procIsWindowVisible.Call(uintptr(h))
		if v != 0 && consoleWindowClass(h) == "CASCADIA_HOSTING_WINDOW_CLASS" {
			out[h] = struct{}{}
		}
	}
	return out
}

func foregroundWindow() windows.Handle {
	h, _, _ := procGetForegroundWin.Call()
	return windows.Handle(h)
}

func isIconic(h windows.Handle) bool {
	v, _, _ := procIsIconic.Call(uintptr(h))
	return v != 0
}

func TestWindowsPowerShellHelper(t *testing.T) {
	if os.Getenv("BIOMELAB_WINDOWS_TERMINAL_HELPER") == "" {
		return
	}
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(2)
	}
	record := struct {
		CWD  string
		Args []string
	}{}
	record.CWD, _ = os.Getwd()
	record.Args = os.Args[separator+2:]
	data, _ := json.Marshal(record)
	if err := os.WriteFile(os.Args[separator+1], data, 0o600); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func TestWindowsPowerShellScriptPreservesArgumentDataAndHandshake(t *testing.T) {
	shellPath, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatalf("windows-latest contract requires powershell.exe: %v", err)
	}
	root := t.TempDir()
	workdir := filepath.Join(root, "work & ' quoted")
	marker := filepath.Join(root, "marker")
	if err := os.Mkdir(workdir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(marker, 0o700); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(root, "arguments.json")
	want := []string{"space value", "O'Brien", "; Write-Output injected", "$env:PATH", `C:\path with space\x`}
	args := []string{os.Args[0], "-test.run=^TestWindowsPowerShellHelper$", "--", result}
	args = append(args, want...)
	script := windowsPowerShellScript(launchRequest{dir: workdir, args: args, markerDir: marker}, false)
	t.Setenv("WT_SESSION", "biomelab-test-pseudoconsole")
	cmd := exec.Command(shellPath, "-NoLogo", "-NoProfile", "-EncodedCommand", encodePowerShell(script))
	cmd.Env = append(os.Environ(), "BIOMELAB_WINDOWS_TERMINAL_HELPER=1", "GORACE=atexit_sleep_ms=0")
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("PowerShell script failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		CWD  string
		Args []string
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	gotDir, err := os.Stat(got.CWD)
	if err != nil {
		t.Fatalf("PowerShell reported an invalid cwd %q: %v", got.CWD, err)
	}
	wantDir, err := os.Stat(workdir)
	if err != nil {
		t.Fatalf("stat expected cwd %q: %v", workdir, err)
	}
	// Windows may report the same directory through its long name after it was
	// entered through an 8.3 path from TEMP (for example, runneradmin versus
	// RUNNER~1). Compare the filesystem objects rather than their spellings.
	if !os.SameFile(gotDir, wantDir) {
		t.Fatalf("PowerShell changed cwd: got %q, want %q", got.CWD, workdir)
	}
	if !reflect.DeepEqual(got.Args, want) {
		t.Fatalf("PowerShell changed arguments: got %q, want %q", got.Args, want)
	}
	record, err := os.ReadFile(filepath.Join(marker, "session"))
	if err != nil {
		t.Fatal(err)
	}
	pid, _, window, kind, err := parseSessionRecord(record)
	if err != nil || pid <= 1 || window != "" || kind != WindowsTerminal {
		t.Fatalf("invalid hosted handshake: pid=%d window=%q kind=%q err=%v record=%q", pid, window, kind, err, record)
	}
	if _, err := os.Stat(filepath.Join(marker, "pending")); !os.IsNotExist(err) {
		t.Fatal("handshake was not atomically renamed")
	}
}

type windowsConsoleReport struct {
	Window, Input, Output, Visible uintptr
	ClassName                      string
	WindowsTerminal                bool
	WTSession                      string
}

// conPTY reports whether the shell that produced this record was hosted by a
// pseudoconsole rather than a classic console window. WT_SESSION is not a
// substitute: the OS default-terminal setting can delegate a plain
// powershell.exe launch to Windows Terminal without setting it.
func (r windowsConsoleReport) conPTY() bool {
	return r.ClassName == windowsPseudoConsoleClass
}

func TestWindowsVisibleConsoleHelper(t *testing.T) {
	if os.Getenv("BIOMELAB_VISIBLE_CONSOLE_HELPER") == "" {
		return
	}
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")
	hwnd, _, _ := kernel32.NewProc("GetConsoleWindow").Call()
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	stdin, _, _ := getStdHandle.Call(uintptr(^uint32(9)))
	stdout, _, _ := getStdHandle.Call(uintptr(^uint32(10)))
	var mode uint32
	stdinOK, _, _ := getConsoleMode.Call(stdin, uintptr(unsafe.Pointer(&mode)))
	stdoutOK, _, _ := getConsoleMode.Call(stdout, uintptr(unsafe.Pointer(&mode)))
	visible, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd)
	className := ""
	if hwnd != 0 {
		buf := make([]uint16, 256)
		n, _, _ := user32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		className = windows.UTF16ToString(buf[:n])
	}
	record := windowsConsoleReport{
		Window: hwnd, Input: stdinOK, Output: stdoutOK, Visible: visible, ClassName: className,
		WindowsTerminal: os.Getenv("WT_SESSION") != "", WTSession: os.Getenv("WT_SESSION"),
	}
	data, err := json.Marshal(record)
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile(os.Getenv("BIOMELAB_CONSOLE_RESULT"), data, 0o600); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func TestWindowsIntentionalLaunchCreatesInteractiveLongLivedConsole(t *testing.T) {
	shellPath, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatalf("windows-latest contract requires powershell.exe: %v", err)
	}
	root := t.TempDir()
	result := filepath.Join(root, "console.json")
	t.Setenv("BIOMELAB_VISIBLE_CONSOLE_HELPER", "1")
	t.Setenv("BIOMELAB_CONSOLE_RESULT", result)
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	// Simulate Biomelab itself having been launched from WT. The fallback must
	// not leak this stale host identity into the new console.
	t.Setenv("WT_SESSION", "stale-parent-session")
	t.Setenv("WT_PROFILE_ID", "stale-parent-profile")
	t.Setenv("BIOME_TERMINAL", shellPath)
	session, err := openTrackedWindows(launchRequest{
		dir:  root,
		args: []string{os.Args[0], "-test.run=^TestWindowsVisibleConsoleHelper$"},
	})
	if err != nil {
		t.Fatalf("tracked visible terminal failed: %v", err)
	}
	if session == nil || !session.ready || session.born == 0 {
		t.Fatalf("terminal did not register a birth-checked shell: %+v", session)
	}
	terminated := false
	terminate := func() {
		if terminated {
			return
		}
		terminated = true
		if process, findErr := os.FindProcess(int(session.info.ShellPID)); findErr == nil {
			_ = process.Kill()
		}
		session.Cleanup()
	}
	t.Cleanup(terminate)

	deadline := time.Now().Add(10 * time.Second)
	var data []byte
	for time.Now().Before(deadline) {
		data, err = os.ReadFile(result)
		if err == nil && len(data) != 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("new console helper did not report: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("new console helper produced an empty report")
	}
	var record windowsConsoleReport
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("invalid console report %q: %v", data, err)
	}
	if record.Input == 0 || record.Output == 0 {
		t.Fatalf("new terminal has unusable console handles: %+v", record)
	}
	if !record.conPTY() && (record.Window == 0 || record.Visible == 0) {
		t.Fatalf("classic console is not visible: %+v", record)
	}
	if record.WTSession == "stale-parent-session" {
		t.Fatalf("fallback inherited the parent terminal identity: %+v", record)
	}
	if session.info.Kind == WindowsTerminal {
		if !record.conPTY() && !record.WindowsTerminal {
			t.Fatalf("host reported as Windows Terminal but owns a classic console: session=%+v report=%+v", session.info, record)
		}
		if session.windowID != "" {
			t.Fatalf("pseudoconsole handle was recorded for activation: session=%+v window=%q report=%+v", session.info, session.windowID, record)
		}
	} else if record.conPTY() || record.WindowsTerminal || session.windowID == "" {
		t.Fatalf("classic console was misclassified: session=%+v window=%q report=%+v", session.info, session.windowID, record)
	}
	// The shell must not adopt the worktree as its process working directory:
	// Windows would hold an open handle on it. See
	// TestWindowsTrackedShellDoesNotLockItsWorktree.
	shell := process.Info{PID: session.info.ShellPID}
	process.Enrich(context.Background(), &shell)
	if shellDir, statErr := os.Stat(shell.Cwd); statErr == nil {
		if rootDir, rootErr := os.Stat(root); rootErr == nil && os.SameFile(shellDir, rootDir) {
			t.Fatalf("tracked shell holds the worktree %q as its working directory", root)
		}
	}
	// The foreground helper has exited (it produced the report), but -NoExit
	// must leave the exact registered PowerShell process alive and interactive.
	currentBirth, err := processBirth(context.Background(), session.info.ShellPID)
	if err != nil || currentBirth != session.born {
		t.Fatalf("host shell did not survive foreground command: birth=%d want=%d err=%v", currentBirth, session.born, err)
	}
	terminate()
	exitDeadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(exitDeadline) {
		birth, birthErr := processBirth(context.Background(), session.info.ShellPID)
		if birthErr == nil && (birth == 0 || birth != session.born) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("test-owned PowerShell shell survived cleanup")
}

// awaitSessionRecord waits for a launched shell to complete its handshake.
func awaitSessionRecord(t *testing.T, marker string) []byte {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(filepath.Join(marker, "session")); err == nil && len(data) != 0 {
			return data
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("shell never registered a session in %s", marker)
	return nil
}

func consoleWindowClass(hwnd windows.Handle) string {
	buf := make([]uint16, 256)
	n, _, _ := user32.NewProc("GetClassNameW").Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return windows.UTF16ToString(buf[:n])
}

// The two tests below pin each host branch of the fallback script regardless of
// this machine's default-terminal setting, which decides which one the
// end-to-end launch test happens to exercise.

// A PowerShell fallback launch can be delegated into Windows Terminal by the OS
// default-terminal setting. Hosting the classic-launch script in wt.exe
// reproduces that shape exactly. The pseudoconsole reports itself visible and
// carries no WT_SESSION, so a visibility check alone records it as a focusable
// classic console and activation then aims at the wrong window.
func TestWindowsFallbackScriptDetectsDelegatedWindowsTerminalHost(t *testing.T) {
	wtPath, err := exec.LookPath("wt.exe")
	if err != nil {
		t.Skip("wt.exe is not installed")
	}
	shellPath, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatalf("windows-latest contract requires powershell.exe: %v", err)
	}
	marker := t.TempDir()
	script := windowsPowerShellScript(launchRequest{markerDir: marker}, false)
	// No window name: reproduce the OS-delegated shape, where Biomelab never
	// assigned one.
	args := windowsTerminalArgs("", shellPath, []string{"-NoLogo", "-NoProfile", "-EncodedCommand", encodePowerShell(script)})
	if err := exec.Command(wtPath, args...).Start(); err != nil {
		t.Fatalf("could not host the fallback script in Windows Terminal: %v", err)
	}

	pid, _, window, kind, err := parseSessionRecord(awaitSessionRecord(t, marker))
	if err != nil || pid <= 1 {
		t.Fatalf("invalid handshake: pid=%d err=%v", pid, err)
	}
	if kind != WindowsTerminal {
		t.Errorf("delegated Windows Terminal host recorded as %q, want %q", kind, WindowsTerminal)
	}
	if window != "" {
		t.Errorf("recorded pseudoconsole window %q for activation; no handle may be kept", window)
	}
}

// The classic console is the one host Biomelab may focus, so it must still be
// recognised and its window handle retained. conhost.exe hosts the shell
// directly, which bypasses default-terminal delegation without changing any
// machine-wide setting.
func TestWindowsFallbackScriptRecordsClassicConsoleForActivation(t *testing.T) {
	shellPath, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatalf("windows-latest contract requires powershell.exe: %v", err)
	}
	if _, err := exec.LookPath("conhost.exe"); err != nil {
		t.Skip("conhost.exe is not available")
	}
	marker := t.TempDir()
	title := "biomelab: classic console probe"
	script := windowsPowerShellScript(launchRequest{
		markerDir: marker, windowTitle: title, command: "Start-Sleep -Seconds 30",
	}, false)
	err = startWindowsConsole(shellPath, "conhost.exe",
		[]string{shellPath, "-NoLogo", "-NoProfile", "-EncodedCommand", encodePowerShell(script)})
	if err != nil {
		t.Fatalf("could not host the fallback script in a classic console: %v", err)
	}

	pid, _, window, kind, err := parseSessionRecord(awaitSessionRecord(t, marker))
	if err != nil || pid <= 1 {
		t.Fatalf("invalid handshake: pid=%d err=%v", pid, err)
	}
	t.Cleanup(func() {
		if p, findErr := os.FindProcess(int(pid)); findErr == nil {
			_ = p.Kill()
		}
	})
	if kind != "" {
		t.Errorf("classic console recorded as %q, want an unlabelled host", kind)
	}
	handle, err := strconv.ParseUint(window, 10, 64)
	if err != nil || handle == 0 {
		t.Fatalf("classic console recorded no activation handle (%q): %v", window, err)
	}
	if class := consoleWindowClass(windows.Handle(handle)); class != windowsClassicConsoleClass {
		t.Errorf("recorded window class is %q, want %q", class, windowsClassicConsoleClass)
	}
	// Activation refuses any handle whose live window text no longer matches the
	// recorded title, so a handle that cannot satisfy that check is useless.
	if text := windowText(windows.Handle(handle)); text != title {
		t.Errorf("recorded window text is %q, want %q", text, title)
	}
	// Whether Windows then grants the focus change depends on the foreground
	// lock, which a test process usually does not hold. Only a refusal that
	// means "this is not the window you recorded" is a defect here.
	session := &Session{windowTitle: title, windowID: window}
	if err := activateSessionWindows(context.Background(), session); err != nil &&
		!strings.Contains(err.Error(), "Windows refused to focus") {
		t.Errorf("recorded classic console was rejected as the wrong window: %v", err)
	}
}

func TestWindowsTerminalArgsNamesTheWindow(t *testing.T) {
	named := windowsTerminalArgs("biomelab-session-42", "pwsh.exe", []string{"-NoExit"})
	if len(named) < 4 || named[0] != "-w" || named[1] != "biomelab-session-42" || named[2] != "new-tab" || named[3] != "pwsh.exe" {
		t.Fatalf("named launch did not target its window: %q", named)
	}
	unnamed := windowsTerminalArgs("", "pwsh.exe", []string{"-NoExit"})
	if unnamed[0] != "-w" || unnamed[1] != "new" {
		t.Fatalf("unnamed launch should use the new keyword: %q", unnamed)
	}
}

// A Windows Terminal session with no window name is one the OS default-terminal
// setting delegated a PowerShell fallback into: Biomelab never invoked wt.exe,
// so there is no window to target. It must refuse focus with guidance rather
// than guess.
func TestActivateDelegatedWindowsTerminalRefusesFocus(t *testing.T) {
	s := &Session{windowTitle: "biomelab: session [x]"}
	s.info.Kind = WindowsTerminal
	err := activateSessionWindows(context.Background(), s)
	if err == nil || !strings.Contains(err.Error(), "OS default-terminal") {
		t.Fatalf("delegated WT session should refuse focus with guidance, got %v", err)
	}
}

// A Windows Terminal session Biomelab launched is raised by its window name,
// even after the shell overwrites the tab title. This drives the real
// production path: openTrackedWindows assigns the name, and activateSessionWindows
// focuses via `wt -w <name> focus-tab` through command.Background.
func TestWindowsLaunchedWindowsTerminalSessionIsFocusable(t *testing.T) {
	if _, err := exec.LookPath("wt.exe"); err != nil {
		t.Skip("wt.exe is not installed")
	}
	t.Setenv("BIOME_TERMINAL", "wt.exe")
	before := cascadiaWindows()
	session, err := openTrackedWindows(launchRequest{
		identifier: "focus-test",
		command:    "$Host.UI.RawUI.WindowTitle = 'OVERWRITTEN-BY-AGENT'; Start-Sleep -Seconds 60",
	})
	if err != nil {
		t.Fatalf("tracked WT terminal failed: %v", err)
	}
	t.Cleanup(func() {
		if p, e := os.FindProcess(int(session.info.ShellPID)); e == nil {
			_ = p.Kill()
		}
		session.Cleanup()
	})
	if session.info.Kind != WindowsTerminal {
		t.Skipf("launch was not hosted in Windows Terminal (kind=%q); focus path not exercised", session.info.Kind)
	}
	if session.wtWindow == "" {
		t.Fatal("launched WT session has no window name to focus by")
	}

	// Locate our window title-independently by diffing the CASCADIA windows.
	var hwnd windows.Handle
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && hwnd == 0 {
		for h := range cascadiaWindows() {
			if _, existed := before[h]; !existed {
				hwnd = h
			}
		}
		if hwnd == 0 {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if hwnd == 0 {
		t.Fatal("could not locate the launched Windows Terminal window")
	}

	// Minimize it so a successful focus is observable.
	procShowWindowAsync.Call(uintptr(hwnd), 6 /* SW_MINIMIZE */)
	time.Sleep(700 * time.Millisecond)

	if err := activateSessionWindows(context.Background(), session); err != nil {
		t.Fatalf("focusing the launched Windows Terminal window failed: %v", err)
	}

	// wt relays asynchronously. Foreground grant depends on the desktop's
	// foreground lock (headless CI may withhold it), but wt should at least
	// restore our window from the taskbar. Prefer the strong signal, accept the
	// weaker one, fail only if wt did not act on our window at all.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if foregroundWindow() == hwnd {
			return // raised to foreground
		}
		time.Sleep(150 * time.Millisecond)
	}
	if !isIconic(hwnd) {
		t.Logf("wt focus-tab restored our window but the desktop withheld foreground (fg=%d ours=%d)", foregroundWindow(), hwnd)
		return
	}
	t.Fatalf("wt focus-tab did not act on our window (still minimized; fg=%d ours=%d)", foregroundWindow(), hwnd)
}

// Removing a worktree must not be blocked by the terminal Biomelab opened in
// it. Windows holds an open handle on a process's working directory, so a
// shell that adopted the worktree would make RemoveWorktree delete the
// contents and then fail on the directory itself, leaving the worktree
// half-removed and unrecreatable until the user closes the terminal.
func TestWindowsTrackedShellDoesNotLockItsWorktree(t *testing.T) {
	wtPath, err := exec.LookPath("wt.exe")
	if err != nil {
		t.Skip("wt.exe is not installed")
	}
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	if err := os.MkdirAll(filepath.Join(worktree, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BIOME_TERMINAL", wtPath)
	session, err := openTrackedWindows(launchRequest{dir: worktree, command: "Start-Sleep -Seconds 30"})
	if err != nil {
		t.Fatalf("tracked terminal failed: %v", err)
	}
	t.Cleanup(func() {
		if p, findErr := os.FindProcess(int(session.info.ShellPID)); findErr == nil {
			_ = p.Kill()
		}
		session.Cleanup()
	})

	if err := os.RemoveAll(worktree); err != nil {
		t.Fatalf("worktree could not be removed while its terminal is open: %v", err)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("worktree directory survived removal: %v", err)
	}
}

func TestWindowsTerminalSelectionAndRecipes(t *testing.T) {
	paths := map[string]string{"wt.exe": `C:\WindowsApps\wt.exe`, "pwsh.exe": `C:\Program Files\PowerShell\pwsh.exe`, "powershell.exe": `C:\Windows\powershell.exe`}
	lookup := func(name string) (string, error) {
		if path := paths[name]; path != "" {
			return path, nil
		}
		return "", exec.ErrNotFound
	}
	got, err := findWindowsTerminal("", lookup)
	if err != nil || got.path != paths["wt.exe"] || got.kind != windowsTerminalWT {
		t.Fatalf("Windows Terminal was not preferred: %+v %v", got, err)
	}
	for _, configured := range []string{"pwsh", "pwsh.exe", "powershell.exe", `C:\Tools\wt.exe`} {
		paths[configured] = configured
		if _, err := findWindowsTerminal(configured, lookup); err != nil {
			t.Errorf("known BIOME_TERMINAL %q rejected: %v", configured, err)
		}
	}
	for _, configured := range []string{"cmd.exe", "wt.exe --fullscreen", "unknown-terminal.exe"} {
		if _, err := findWindowsTerminal(configured, lookup); err == nil || !strings.Contains(err.Error(), "unsupported BIOME_TERMINAL") {
			t.Errorf("unknown BIOME_TERMINAL %q: %v", configured, err)
		}
	}
}

func TestWindowsPowerShellLiteralAndEncoding(t *testing.T) {
	value := "O'Brien; $env:PATH 日本語"
	if got := powerShellLiteral(value); got != "'O''Brien; $env:PATH 日本語'" {
		t.Fatalf("unsafe literal: %q", got)
	}
	if strings.Contains(encodePowerShell(value), value) {
		t.Fatal("encoded command exposed source text")
	}
}

func TestWindowsTerminalRecipeKeepsUserDataOutOfWTGrammar(t *testing.T) {
	req := launchRequest{
		dir:         `C:\work; new-tab --title injected`,
		windowTitle: `branch; new-tab --title injected`,
		args:        []string{"sbx", "run", "--name", `box; new-tab`},
	}
	script := windowsPowerShellScript(req, true)
	encoded := encodePowerShell(script)
	args := windowsTerminalArgs("biomelab-session-0", `C:\Program Files\PowerShell\pwsh.exe`, []string{"-NoExit", "-EncodedCommand", encoded})
	for _, arg := range args {
		if arg == "--startingDirectory" || arg == "--title" || strings.Contains(arg, req.dir) || strings.Contains(arg, req.windowTitle) || strings.Contains(arg, ";") {
			t.Fatalf("user data reached wt.exe grammar: %q", args)
		}
	}
	if args[0] != "-w" || args[1] != "biomelab-session-0" {
		t.Fatalf("window name not passed to wt.exe: %q", args)
	}
	if !strings.Contains(script, powerShellLiteral(filepath.Clean(req.dir))) || !strings.Contains(script, powerShellLiteral(req.windowTitle)) {
		t.Fatalf("encoded script lost data: %s", script)
	}
}

func TestWithoutWindowsTerminalEnvironment(t *testing.T) {
	got := withoutWindowsTerminalEnvironment([]string{"Path=x", "WT_SESSION=old", "wt_profile_id=old", "OTHER=y"})
	want := []string{"Path=x", "OTHER=y"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("terminal environment not scrubbed: %q", got)
	}
}
