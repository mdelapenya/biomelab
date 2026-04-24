package terminal

import "github.com/mdelapenya/biomelab/internal/process"

// Kind identifies a terminal emulator.
type Kind string

const (
	TerminalApp     Kind = "Terminal"
	ITerm2          Kind = "iTerm2"
	Alacritty       Kind = "Alacritty"
	Kitty           Kind = "kitty"
	WezTerm         Kind = "WezTerm"
	GnomeTerminal   Kind = "gnome-terminal"
	Konsole         Kind = "Konsole"
	Tilix           Kind = "Tilix"
	Xfce4Terminal   Kind = "xfce4-terminal"
	Hyper           Kind = "Hyper"
	WindowsTerminal Kind = "Windows Terminal"
)

// ShellPatterns lists substrings that identify interactive shell processes.
// We don't care which shell it is — only that it is a shell.
var ShellPatterns = []string{"bash", "zsh", "fish", "pwsh", "powershell"}

// processPattern pairs a Kind with the process-name substrings that identify it.
type processPattern struct {
	Kind     Kind
	Patterns []string
}

// EmulatorPatterns lists terminal emulators and their process-name substrings.
// Order matters: more specific patterns must appear before broader ones
// (e.g. "iterm2" before "terminal") to prevent mis-classification.
var EmulatorPatterns = []processPattern{
	{ITerm2, []string{"iterm2"}},
	{Alacritty, []string{"alacritty"}},
	{Kitty, []string{"kitty"}},
	{WezTerm, []string{"wezterm"}},
	{GnomeTerminal, []string{"gnome-terminal"}},
	{Konsole, []string{"konsole"}},
	{Tilix, []string{"tilix"}},
	{Xfce4Terminal, []string{"xfce4-terminal"}},
	{Hyper, []string{"hyper"}},
	{WindowsTerminal, []string{"windowsterminal", "windows terminal"}},
	// TerminalApp must come last: "terminal" is a broad pattern that could
	// match emulators whose name contains the word (e.g. gnome-terminal).
	{TerminalApp, []string{"terminal"}},
}

// Info holds information about a detected terminal session.
type Info struct {
	Kind     Kind  // which terminal emulator
	ShellPID int32 // PID of the shell process (bash/zsh/fish)
	RootPID  int32 // PID of the terminal emulator process
}

// DetectionResult maps absolute worktree paths to detected terminal sessions.
type DetectionResult map[string][]Info

// Detector finds terminal shell processes and matches them to worktree paths.
type Detector struct {
	lister process.Lister
}

// NewDetector creates a Detector using real system processes.
func NewDetector() *Detector {
	return &Detector{lister: &process.OSLister{}}
}

// NewDetectorWithLister creates a Detector with a custom process lister (for testing).
func NewDetectorWithLister(lister process.Lister) *Detector {
	return &Detector{lister: lister}
}
