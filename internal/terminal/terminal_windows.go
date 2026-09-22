package terminal

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/mdelapenya/biomelab/internal/command"
)

type windowsTerminalKind int

const (
	windowsTerminalWT windowsTerminalKind = iota
	windowsTerminalPowerShell
)

type windowsTerminalExecutable struct {
	path string
	kind windowsTerminalKind
}

func windowsOpen(req launchRequest) error {
	if req.windowTitle == "" && req.identifier != "" {
		req.windowTitle = Title(req.identifier)
	}
	if err := validateLaunchRequest(req); err != nil {
		return err
	}
	terminal, err := findWindowsTerminal(os.Getenv("BIOME_TERMINAL"), exec.LookPath)
	if err != nil {
		return err
	}
	shell, err := findWindowsPowerShell(terminal, exec.LookPath)
	if err != nil {
		return err
	}
	script := windowsPowerShellScript(req, terminal.kind == windowsTerminalWT)
	shellArgs := []string{"-NoLogo", "-NoProfile", "-NoExit", "-EncodedCommand", encodePowerShell(script)}

	var cmd *exec.Cmd
	if terminal.kind == windowsTerminalWT {
		cmd = exec.Command(terminal.path, windowsTerminalArgs(shell.path, shellArgs)...)
	} else {
		return startWindowsConsole(terminal.path, terminal.path, shellArgs)
	}
	cmd.Stderr = os.Stderr
	return startLauncher(cmd)
}

// windowsTerminalArgs intentionally contains only fixed launcher tokens and
// the encoded PowerShell command. In particular, paths and titles must not be
// passed to wt.exe: its CLI applies a second command grammar in which semicolon
// is meaningful even after the Windows argv split. The encoded script carries
// all user-controlled data instead.
func windowsTerminalArgs(shell string, shellArgs []string) []string {
	args := []string{"-w", "new", "new-tab", shell}
	return append(args, shellArgs...)
}

// startWindowsConsole uses a hidden, short-lived helper to ask Windows to
// create the final console application in a new interactive window. Launching
// it directly through os/exec would populate STARTUPINFO with Go's null-device
// standard handles, leaving a console that cannot be used interactively from a
// GUI parent. Start-Process without -NoNewWindow supplies the new console's own
// input and output handles.
func startWindowsConsole(launcher, executable string, args []string) error {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = powerShellLiteral(arg)
	}
	script := "$ErrorActionPreference = 'Stop'\n$biomeArgs = [string[]]@(" + strings.Join(quoted, ",") + ")\n" +
		"Start-Process -FilePath " + powerShellLiteral(executable) + " -ArgumentList $biomeArgs -WindowStyle Normal"
	cmd := command.Background(launcher, "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell(script))
	// WT_SESSION describes the current host, not a future console. Carrying it
	// from a Biomelab process launched in Windows Terminal would falsely label a
	// classic conhost fallback. A delegated default-terminal host sets fresh
	// values on the final shell.
	cmd.Env = withoutWindowsTerminalEnvironment(os.Environ())
	cmd.Stderr = os.Stderr
	return startLauncher(cmd)
}

func withoutWindowsTerminalEnvironment(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(name, "WT_SESSION") || strings.EqualFold(name, "WT_PROFILE_ID") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func validateLaunchRequest(req launchRequest) error {
	if req.command != "" && len(req.args) != 0 {
		return fmt.Errorf("terminal: command source and argument vector are mutually exclusive")
	}
	if req.dir == "" && req.command == "" && len(req.args) == 0 && req.markerDir == "" {
		return fmt.Errorf("terminal: nothing to run (no dir or command)")
	}
	values := []string{req.dir, req.command, req.identifier, req.markerDir, req.windowTitle}
	values = append(values, req.args...)
	for _, value := range values {
		if strings.IndexByte(value, 0) >= 0 {
			return fmt.Errorf("terminal: value contains NUL")
		}
	}
	if len(req.args) != 0 && req.args[0] == "" {
		return fmt.Errorf("terminal: executable is empty")
	}
	return nil
}

func findWindowsTerminal(configured string, lookPath func(string) (string, error)) (windowsTerminalExecutable, error) {
	if configured != "" {
		kind, ok := knownWindowsTerminal(configured)
		if !ok {
			return windowsTerminalExecutable{}, fmt.Errorf("terminal: unsupported BIOME_TERMINAL %q; use wt.exe, pwsh.exe, or powershell.exe", configured)
		}
		path, err := lookPath(configured)
		if err != nil {
			return windowsTerminalExecutable{}, fmt.Errorf("terminal: BIOME_TERMINAL %q not found: %w", configured, err)
		}
		return windowsTerminalExecutable{path: path, kind: kind}, nil
	}
	if path, err := lookPath("wt.exe"); err == nil {
		return windowsTerminalExecutable{path: path, kind: windowsTerminalWT}, nil
	}
	for _, name := range []string{"pwsh.exe", "powershell.exe"} {
		if path, err := lookPath(name); err == nil {
			return windowsTerminalExecutable{path: path, kind: windowsTerminalPowerShell}, nil
		}
	}
	return windowsTerminalExecutable{}, fmt.Errorf("terminal: neither Windows Terminal (wt.exe) nor PowerShell was found")
}

func knownWindowsTerminal(path string) (windowsTerminalKind, bool) {
	base := strings.ToLower(filepath.Base(path))
	base = strings.TrimSuffix(base, ".exe")
	switch base {
	case "wt":
		return windowsTerminalWT, true
	case "pwsh", "powershell":
		return windowsTerminalPowerShell, true
	default:
		return 0, false
	}
}

func findWindowsPowerShell(terminal windowsTerminalExecutable, lookPath func(string) (string, error)) (windowsTerminalExecutable, error) {
	if terminal.kind == windowsTerminalPowerShell {
		return terminal, nil
	}
	for _, name := range []string{"pwsh.exe", "powershell.exe"} {
		if path, err := lookPath(name); err == nil {
			return windowsTerminalExecutable{path: path, kind: windowsTerminalPowerShell}, nil
		}
	}
	return windowsTerminalExecutable{}, fmt.Errorf("terminal: Windows Terminal requires pwsh.exe or powershell.exe")
}

func windowsPowerShellScript(req launchRequest, windowsTerminal bool) string {
	var statements []string
	if req.dir != "" {
		statements = append(statements, "Set-Location -LiteralPath "+powerShellLiteral(filepath.Clean(req.dir)))
	}
	if req.windowTitle != "" {
		statements = append(statements, "$Host.UI.RawUI.WindowTitle = "+powerShellLiteral(req.windowTitle))
	}
	if req.markerDir != "" {
		kind := "''"
		window := "''"
		if windowsTerminal {
			kind = powerShellLiteral(string(WindowsTerminal))
		} else {
			// GetConsoleWindow is valid for the classic console fallback. It is
			// deliberately not used for Windows Terminal's pseudoconsole, where it
			// identifies a hidden message-only window rather than the visible tab.
			statements = append(statements, "Add-Type -Namespace BiomeLab -Name ConsoleWindow -MemberDefinition '[DllImport(\"kernel32.dll\")] public static extern IntPtr GetConsoleWindow(); [DllImport(\"user32.dll\")] public static extern bool IsWindowVisible(IntPtr hWnd);'")
			// A native PowerShell launch can still be redirected into Windows
			// Terminal when it is the user's default terminal. Detect the actual
			// host inside the long-lived shell, and never retain that pseudoconsole's
			// hidden GetConsoleWindow message handle.
			statements = append(statements,
				"$biomeConsoleWindow = [BiomeLab.ConsoleWindow]::GetConsoleWindow()",
				"$biomeHasVisibleConsole = ($biomeConsoleWindow -ne [IntPtr]::Zero) -and [BiomeLab.ConsoleWindow]::IsWindowVisible($biomeConsoleWindow)",
				"$biomeTerminalKind = if ($biomeHasVisibleConsole) { '' } elseif ($env:WT_SESSION) { "+powerShellLiteral(string(WindowsTerminal))+" } else { '' }",
				"$biomeWindow = if ($biomeHasVisibleConsole) { [string]$biomeConsoleWindow.ToInt64() } else { '' }")
			kind = "$biomeTerminalKind"
			window = "$biomeWindow"
		}
		pending := filepath.Join(req.markerDir, "pending")
		record := filepath.Join(req.markerDir, "session")
		statements = append(statements,
			"$biomeRecord = [string[]]@([string]$PID, '', "+window+", "+kind+")",
			"[IO.File]::WriteAllLines("+powerShellLiteral(pending)+", $biomeRecord, [Text.UTF8Encoding]::new($false))",
			"[IO.File]::Move("+powerShellLiteral(pending)+", "+powerShellLiteral(record)+")")
	}
	if len(req.args) != 0 {
		quoted := make([]string, len(req.args))
		for i, arg := range req.args {
			quoted[i] = powerShellLiteral(arg)
		}
		statements = append(statements, "& "+strings.Join(quoted, " "))
	} else if req.command != "" {
		statements = append(statements, "& {\n"+req.command+"\n}")
	}
	return strings.Join(statements, "\n")
}

func powerShellLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func encodePowerShell(script string) string {
	return base64.StdEncoding.EncodeToString(utf16LE(script))
}

func utf16LE(value string) []byte {
	units := utf16.Encode([]rune(value))
	encoded := make([]byte, len(units)*2)
	for i, unit := range units {
		encoded[i*2] = byte(unit)
		encoded[i*2+1] = byte(unit >> 8)
	}
	return encoded
}
