package ops

import (
	"strings"

	"github.com/mdelapenya/biomelab/internal/sandbox"
)

// SandboxResult is the outcome of a sandbox operation.
type SandboxResult struct {
	SandboxName string
	Output      string
	Err         error
}

// ErrorMessage returns a user-facing error message. Prefers the underlying
// CLI's stderr/stdout (Output) over the bare exec error ("exit status 1"),
// which is what users actually need to see — sbx puts actionable messages
// like "needs restart", "login required", "policy not set" in its output.
// Returns "" if Err is nil.
func (r SandboxResult) ErrorMessage() string {
	if r.Err == nil {
		return ""
	}
	if msg := FirstNonEmptyLine(r.Output); msg != "" {
		return msg
	}
	return r.Err.Error()
}

// FirstNonEmptyLine returns the first non-empty, trimmed line of s, or "".
// Used to compress multi-line CLI errors into a single-line status message.
func FirstNonEmptyLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// CreateSandbox creates a new sandbox.
func CreateSandbox(args []string) SandboxResult {
	out, err := sandbox.Create(args)
	name := ""
	if len(args) > 0 {
		name = args[len(args)-1] // convention: last arg is the sandbox name or repo path
	}
	return SandboxResult{SandboxName: name, Output: out, Err: err}
}

// StartSandbox starts a stopped sandbox.
func StartSandbox(name string) SandboxResult {
	out, err := sandbox.Start(name)
	return SandboxResult{SandboxName: name, Output: out, Err: err}
}

// StopSandbox stops a running sandbox.
func StopSandbox(name string) SandboxResult {
	out, err := sandbox.Stop(name)
	return SandboxResult{SandboxName: name, Output: out, Err: err}
}

// RemoveSandbox removes a sandbox.
func RemoveSandbox(name string) SandboxResult {
	out, err := sandbox.Remove(name)
	return SandboxResult{SandboxName: name, Output: out, Err: err}
}
