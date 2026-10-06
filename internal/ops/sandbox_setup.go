package ops

import (
	"fmt"

	"github.com/mdelapenya/biomelab/internal/sandbox"
)

// ExistingSandboxWithKitsError identifies a safely matched sandbox that cannot
// receive the requested kits without recreating its agent container. Callers
// may offer an explicit registration without installing those kits.
type ExistingSandboxWithKitsError struct {
	Name string
}

func (e *ExistingSandboxWithKitsError) Error() string {
	return fmt.Sprintf("sandbox %s already exists; adding kits to existing sandboxes is not supported", e.Name)
}

// EnsureSandbox creates a sandbox or reuses an explicitly stored or
// repo-specific generated name. Kits are supported only during initial
// creation; setup never modifies an existing sandbox's agent container.
func EnsureSandbox(repoName, repoPath, name, agent string, kitRefs []string) (actualName string, created bool, err error) {
	return EnsureSandboxWithKit(repoName, repoPath, name, agent, "", kitRefs)
}

// EnsureSandboxWithKit creates with a selected sandbox kit as its positional
// workload and optional mixins as --kit flags. An empty sandboxRef uses agent.
func EnsureSandboxWithKit(repoName, repoPath, name, agent, sandboxRef string, mixinRefs []string) (actualName string, created bool, err error) {
	if name == "" {
		name = sandbox.GeneratedName(repoPath, agent)
	}
	if name == "" {
		return "", false, fmt.Errorf("sandbox requires a repository path and agent")
	}
	if err := sandbox.Preflight(); err != nil {
		return "", false, err
	}
	statuses, err := sandbox.ListStatuses()
	if err != nil {
		return "", false, err
	}
	if matched, _, ok := sandbox.MatchStatus(statuses, sandbox.Candidates(name, repoName, repoPath, agent)); ok {
		if sandboxRef != "" || len(mixinRefs) > 0 {
			return "", false, &ExistingSandboxWithKitsError{Name: matched}
		}
		return matched, false, nil
	}
	workload := agent
	if sandboxRef != "" {
		workload = sandboxRef
	}
	result := CreateSandbox(sandbox.CreateWithKitArgs(name, workload, repoPath, mixinRefs))
	if result.Err != nil {
		return "", false, fmt.Errorf("create sandbox %s: %s", name, result.ErrorMessage())
	}
	return name, true, nil
}

// RegisterExistingSandbox rechecks one previously matched name immediately
// before registration. It never creates a sandbox or adopts another candidate
// if the original disappears between the prompt and confirmation.
func RegisterExistingSandbox(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("sandbox name is required for registration")
	}
	if err := sandbox.Preflight(); err != nil {
		return "", err
	}
	statuses, err := sandbox.ListStatuses()
	if err != nil {
		return "", err
	}
	if _, ok := statuses[name]; !ok {
		return "", fmt.Errorf("sandbox %s no longer exists; retry setup", name)
	}
	return name, nil
}
