package ops

import (
	"fmt"

	"github.com/mdelapenya/biomelab/internal/sandbox"
)

// EnsureSandbox creates a sandbox or reuses one discovered under a supported
// name. Kits are supported only during initial creation; setup never modifies
// an existing sandbox's agent container or deletes the sandbox.
func EnsureSandbox(repoName, repoPath, name, agent string, kitRefs []string) (actualName string, created bool, err error) {
	if err := sandbox.Preflight(); err != nil {
		return "", false, err
	}
	statuses, err := sandbox.ListStatuses()
	if err != nil {
		return "", false, err
	}
	if matched, _, ok := sandbox.MatchStatus(statuses, sandbox.Candidates(name, repoName, repoPath, agent)); ok {
		if len(kitRefs) > 0 {
			return "", false, fmt.Errorf("sandbox %s already exists; adding kits to existing sandboxes is not supported. Choose No to register it without changing its kits", matched)
		}
		return matched, false, nil
	}
	result := CreateSandbox(sandbox.CreateArgs(name, agent, repoPath, kitRefs))
	if result.Err != nil {
		return "", false, fmt.Errorf("create sandbox %s: %s", name, result.ErrorMessage())
	}
	return name, true, nil
}
