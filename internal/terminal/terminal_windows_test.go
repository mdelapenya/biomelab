package terminal

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

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
	if !strings.EqualFold(filepath.Clean(got.CWD), filepath.Clean(workdir)) || !reflect.DeepEqual(got.Args, want) {
		t.Fatalf("PowerShell changed data: cwd=%q args=%q", got.CWD, got.Args)
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
	WindowsTerminal                bool
	WTSession                      string
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
	visible, _, _ := windows.NewLazySystemDLL("user32.dll").NewProc("IsWindowVisible").Call(hwnd)
	record := windowsConsoleReport{
		Window: hwnd, Input: stdinOK, Output: stdoutOK, Visible: visible,
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
	if !record.WindowsTerminal && (record.Window == 0 || record.Visible == 0) {
		t.Fatalf("classic console is not visible: %+v", record)
	}
	if record.WTSession == "stale-parent-session" {
		t.Fatalf("fallback inherited the parent terminal identity: %+v", record)
	}
	if session.info.Kind == WindowsTerminal {
		if !record.WindowsTerminal || session.windowID != "" {
			t.Fatalf("delegated WT host was misclassified: session=%+v report=%+v", session.info, record)
		}
	} else if record.WindowsTerminal || session.windowID == "" {
		t.Fatalf("classic console was misclassified: session=%+v window=%q report=%+v", session.info, session.windowID, record)
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
	args := windowsTerminalArgs(`C:\Program Files\PowerShell\pwsh.exe`, []string{"-NoExit", "-EncodedCommand", encoded})
	for _, arg := range args {
		if arg == "--startingDirectory" || arg == "--title" || strings.Contains(arg, req.dir) || strings.Contains(arg, req.windowTitle) || strings.Contains(arg, ";") {
			t.Fatalf("user data reached wt.exe grammar: %q", args)
		}
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
