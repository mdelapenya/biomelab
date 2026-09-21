package sysdeps

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/mdelapenya/biomelab/internal/config"
	"github.com/mdelapenya/biomelab/internal/provider"
	"github.com/mdelapenya/biomelab/internal/sandbox"

	"github.com/mdelapenya/biomelab/internal/command"
)

// Registry returns the standard set of checks biomelab knows about.
// Tests can substitute their own checks via Cache.SetChecks.
func Registry() []Check {
	return []Check{
		rgtCheck(),
		ghCheck(),
		glabCheck(),
		sbxCheck(),
	}
}

// rgtCheck inspects the re_gent CLI. Always applies — surfacing the audit
// trail on cards is an opt-in feature regardless of repo mode.
func rgtCheck() Check {
	return Check{
		Name:        "rgt",
		DisplayName: "re_gent",
		Reason:      "Surface AI agent audit trails on worktree cards.",
		Probe: func() Result {
			path, err := exec.LookPath("rgt")
			if err != nil {
				return Result{Status: StatusMissing}
			}
			version := probeVersion(path, "version")
			if version == "" {
				version = probeVersion(path, "--version")
			}
			return Result{Status: StatusOK, Version: version}
		},
		DocsURL:  "https://github.com/regent-vcs/re_gent",
		Optional: true,
	}
}

// ghCheck wraps provider.GitHubProvider.CheckCLI so the existing
// authentication detection is reused without duplicating subprocess logic.
// Suppressed when glab is installed — users on one provider don't need
// the other CLI nagging them about being missing.
func ghCheck() Check {
	return Check{
		Name:        "gh",
		DisplayName: "GitHub CLI",
		Reason:      "Fetch PR status for GitHub-hosted repositories.",
		Probe: func() Result {
			avail := (&provider.GitHubProvider{}).CheckCLI()
			switch avail {
			case provider.CLIAvailable:
				return Result{Status: StatusOK, Version: probeVersion("gh", "--version")}
			case provider.CLINotAuthenticated:
				return Result{Status: StatusDegraded, Version: probeVersion("gh", "--version"), Note: "not authenticated — run: gh auth login"}
			default:
				return Result{Status: StatusMissing}
			}
		},
		InstallHint:   "brew install gh",
		DocsURL:       "https://cli.github.com/",
		SuppressIfAny: []string{"glab"},
	}
}

// glabCheck mirrors ghCheck for GitLab. Suppressed when gh is installed.
func glabCheck() Check {
	return Check{
		Name:        "glab",
		DisplayName: "GitLab CLI",
		Reason:      "Fetch MR status for GitLab-hosted repositories.",
		Probe: func() Result {
			avail := (&provider.GitLabProvider{}).CheckCLI()
			switch avail {
			case provider.CLIAvailable:
				return Result{Status: StatusOK, Version: probeVersion("glab", "--version")}
			case provider.CLINotAuthenticated:
				return Result{Status: StatusDegraded, Version: probeVersion("glab", "--version"), Note: "not authenticated — run: glab auth login"}
			default:
				return Result{Status: StatusMissing}
			}
		},
		InstallHint:   "brew install glab",
		DocsURL:       "https://gitlab.com/gitlab-org/cli",
		SuppressIfAny: []string{"gh"},
	}
}

// sbxCheck reports Docker Sandbox CLI readiness. Only applies when at least
// one repo is configured in sandbox mode — otherwise it shows as N/A.
func sbxCheck() Check {
	return Check{
		Name:        "sbx",
		DisplayName: "Docker Sandbox",
		Reason:      "Run agents inside Docker sandboxes (sandbox mode).",
		Applies:     hasSandboxRepo,
		Probe: func() Result {
			if !sandbox.Available() {
				return Result{Status: StatusMissing}
			}
			version := sandbox.Version().Client
			if err := sandbox.Preflight(); err != nil {
				return Result{Status: StatusDegraded, Version: version, Note: firstLine(err.Error())}
			}
			return Result{Status: StatusOK, Version: version}
		},
		DocsURL: "https://docs.docker.com/ai/sandboxes/",
	}
}

// hasSandboxRepo returns true when cfg has at least one repo with a
// sandbox mode entry. Used to gate sbx and docker checks.
func hasSandboxRepo(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	for _, r := range cfg.Repos {
		for _, m := range r.Modes {
			if m.Type == "sandbox" {
				return true
			}
		}
	}
	return false
}

// ansiEscape matches CSI (`ESC [ … letter`) and OSC (`ESC ] … BEL`) escape
// sequences emitted by tools that color their output (e.g. `rgt version`
// prints "re_gent\x1b[38;5;141m" segments). Stripped so the dialog shows
// clean text regardless of whether the tool respects NO_COLOR.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]|\x1b\][^\x07]*\x07`)

// probeVersion runs `<bin> <subcmd>` and returns the first non-empty line of
// stdout, ANSI-stripped and trimmed. Sets NO_COLOR=1 so well-behaved tools
// skip color output entirely; the regex strips anything that leaks through.
// Returns "" on any error so the caller can decide how to render the row.
func probeVersion(bin, subcmd string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := command.BackgroundContext(ctx, bin, subcmd)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return firstLine(stripANSI(string(out)))
}

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

// firstLine returns the first non-empty trimmed line of s.
func firstLine(s string) string {
	for line := range strings.SplitSeq(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}
