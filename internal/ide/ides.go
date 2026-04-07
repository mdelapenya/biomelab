package ide

// Kind identifies an IDE.
type Kind string

const (
	VSCode   Kind = "vscode"
	Cursor   Kind = "cursor"
	Zed      Kind = "zed"
	Windsurf Kind = "windsurf"
	GoLand   Kind = "goland"
	IntelliJ Kind = "intellij"
	PyCharm  Kind = "pycharm"
	Neovim   Kind = "neovim"
	Vim      Kind = "vim"
)

// processPattern pairs a Kind with the substrings that identify its process name.
type processPattern struct {
	Kind     Kind
	Patterns []string
}

// ProcessPatterns lists IDE kinds and their process-name substrings.
// Order matters: more specific patterns (e.g. "nvim") must appear before
// broader ones (e.g. "vim") to prevent mis-classification.
var ProcessPatterns = []processPattern{
	{Neovim, []string{"nvim"}},
	{VSCode, []string{"code"}},
	{Cursor, []string{"cursor"}},
	{Zed, []string{"zed"}},
	{Windsurf, []string{"windsurf"}},
	{GoLand, []string{"goland"}},
	{IntelliJ, []string{"idea"}},
	{PyCharm, []string{"pycharm"}},
	{Vim, []string{"vim"}},
}

// Info holds information about a detected IDE process.
type Info struct {
	Kind      Kind
	PID       int32   // representative PID shown on the card
	ExtraPIDs []int32 // all PIDs for this IDE kind (including PID); used for killing
}

// DetectionResult maps absolute worktree paths to the IDEs open in them.
type DetectionResult map[string][]Info
