package agent

// Kind identifies a coding agent.
type Kind string

const (
	Claude   Kind = "claude"
	Kiro     Kind = "kiro"
	Copilot  Kind = "copilot"
	Codex    Kind = "codex"
	OpenCode Kind = "opencode"
	Gemini   Kind = "gemini"
)

// ProcessPatterns maps each agent kind to substrings that appear in their process names.
var ProcessPatterns = map[Kind][]string{
	Claude:   {"claude"},
	Kiro:     {"kiro"},
	Copilot:  {"copilot-agent", "copilot-language-server"},
	Codex:    {"codex"},
	OpenCode: {"opencode"},
	Gemini:   {"gemini"},
}

// Info holds information about a detected agent process.
type Info struct {
	Kind    Kind
	PID     string
	State   string // process state (e.g. "S", "R", "Z")
	Started string // start date/time
}

// DetectionResult maps absolute worktree paths to the agents working in them.
type DetectionResult map[string][]Info
