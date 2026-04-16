package ops

import "github.com/mdelapenya/biomelab/internal/sandbox"

// SandboxResult is the outcome of a sandbox operation.
type SandboxResult struct {
	SandboxName string
	Output      string
	Err         error
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
