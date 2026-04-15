package card

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mdelapenya/biomelab/internal/agent"
	"github.com/mdelapenya/biomelab/internal/git"
	"github.com/mdelapenya/biomelab/internal/ide"
	"github.com/mdelapenya/biomelab/internal/provider"
)

var (
	branchStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	agentActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	agentDetailStyle = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("42"))
	agentNoneStyle   = lipgloss.NewStyle().Faint(true)
	ideActiveStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	ideNoneStyle     = lipgloss.NewStyle().Faint(true)
	dirtyStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	cleanStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	mainBadge        = lipgloss.NewStyle().
				Background(lipgloss.Color("62")).
				Foreground(lipgloss.Color("230")).
				Padding(0, 1).
				Bold(true)
	pathStyle = lipgloss.NewStyle().Faint(true)

	prStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	prDraftStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	prMergedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("135"))
	prClosedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	ciSuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	ciFailStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	ciPendStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	cliUnavailStyle = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("214"))

	sandboxRunStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // green
	sandboxStopStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // yellow
	sandboxWarnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196")) // red

	syncUpToDateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	syncNeedSyncStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	syncDivergedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	syncUnknownStyle  = lipgloss.NewStyle().Faint(true)
)

// SandboxStatus represents the state of a sandbox for display.
type SandboxStatus int

const (
	SandboxNotFound SandboxStatus = iota // sandbox does not exist
	SandboxRunning                       // sandbox is running
	SandboxStopped                       // sandbox exists but is stopped
)

// SandboxInfo holds sandbox-specific display data for a card.
// When Name is non-empty, the card shows a sandbox badge.
type SandboxInfo struct {
	Name          string        // sandbox name (derived from repo + agent)
	Status        SandboxStatus // current sandbox status
	ClientVersion string        // sbx client version
	ServerVersion string        // sbx server version
}

// Render produces the content for a single worktree card.
func Render(wt git.Worktree, agents []agent.Info, ides []ide.Info, pr *provider.PRInfo, cliAvail provider.CLIAvailability, prov provider.Provider, sbx *SandboxInfo) string {
	var b strings.Builder

	// Branch line
	branch := branchStyle.Render(wt.Branch)
	if wt.IsMain {
		branch += " " + mainBadge.Render("main")
	}
	if wt.Detached {
		branch += " (detached)"
	}
	b.WriteString(branch)
	b.WriteString("\n")

	// Path
	b.WriteString(pathStyle.Render(wt.Path))
	b.WriteString("\n")

	// Sandbox info
	if sbx != nil && sbx.Name != "" {
		switch sbx.Status {
		case SandboxRunning:
			b.WriteString(sandboxRunStyle.Render("🐳 sandbox: " + sbx.Name + " (running)"))
		case SandboxStopped:
			b.WriteString(sandboxStopStyle.Render("🐳 sandbox: " + sbx.Name + " (stopped)"))
		case SandboxNotFound:
			b.WriteString(sandboxWarnStyle.Render("🐳 sandbox: " + sbx.Name + " (not found)"))
		}
		b.WriteString("\n")
		if sbx.ClientVersion != "" {
			ver := "sbx: client " + sbx.ClientVersion
			if sbx.ServerVersion != "" {
				ver += "  server " + sbx.ServerVersion
			}
			b.WriteString(pathStyle.Render(ver))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// PR/MR status
	prLabel := "PR"
	if prov == provider.ProviderGitLab {
		prLabel = "MR"
	}

	switch {
	case pr != nil:
		b.WriteString(renderPR(pr, prLabel))
		b.WriteString("\n")
	case cliAvail == provider.CLIUnsupportedProvider:
		b.WriteString(cliUnavailStyle.Render(fmt.Sprintf("%s status: %s not yet supported", prLabel, prov.String())))
		b.WriteString("\n")
	case cliAvail == provider.CLINotFound:
		cliName := cliNameForProvider(prov)
		b.WriteString(cliUnavailStyle.Render(fmt.Sprintf("%s not installed — install %s CLI", cliName, cliName)))
		b.WriteString("\n")
	case cliAvail == provider.CLINotAuthenticated:
		cliName := cliNameForProvider(prov)
		b.WriteString(cliUnavailStyle.Render(fmt.Sprintf("%s not authenticated — run: %s auth login", cliName, cliName)))
		b.WriteString("\n")
	}

	// Agent status
	if len(agents) > 0 {
		for _, a := range agents {
			prefix := "● "
			detailPrefix := "  "
			if a.IsSubAgent {
				prefix = "  ↳ "
				detailPrefix = "    "
			}
			line := fmt.Sprintf("%s (PID %s)", string(a.Kind), a.PID)
			b.WriteString(agentActiveStyle.Render(prefix + line))
			b.WriteString("\n")
			detail := fmt.Sprintf("%sstate: %s  started: %s", detailPrefix, a.State, a.Started)
			b.WriteString(agentDetailStyle.Render(detail))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(agentNoneStyle.Render("○ no agent"))
		b.WriteString("\n")
	}

	// IDE status
	if len(ides) > 0 {
		for _, i := range ides {
			line := fmt.Sprintf("■ %s (PID %d)", string(i.Kind), i.PID)
			b.WriteString(ideActiveStyle.Render(line))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(ideNoneStyle.Render("□ no IDE"))
		b.WriteString("\n")
	}

	// Dirty status
	if wt.IsDirty {
		b.WriteString(dirtyStyle.Render("~ dirty"))
	} else {
		b.WriteString(cleanStyle.Render("✓ clean"))
	}

	// Sync status
	b.WriteString("  ")
	switch wt.Sync {
	case git.SyncUpToDate:
		b.WriteString(syncUpToDateStyle.Render("↕ up-to-date"))
	case git.SyncAhead:
		b.WriteString(syncNeedSyncStyle.Render("↑ ahead"))
	case git.SyncBehind:
		b.WriteString(syncNeedSyncStyle.Render("↓ behind"))
	case git.SyncDiverged:
		b.WriteString(syncDivergedStyle.Render("↕ diverged"))
	case git.SyncNoUpstream:
		b.WriteString(syncUnknownStyle.Render("- no upstream"))
	}

	return b.String()
}

func renderPR(pr *provider.PRInfo, label string) string {
	title := truncate(pr.Title, 30)
	num := fmt.Sprintf("#%d", pr.Number)

	var style lipgloss.Style
	stateLabel := pr.State
	switch {
	case pr.Draft:
		style = prDraftStyle
		stateLabel = "draft"
	case pr.State == "merged":
		style = prMergedStyle
	case pr.State == "closed":
		style = prClosedStyle
	default:
		style = prStyle
	}

	line := style.Render(fmt.Sprintf("%s %s %s (%s)", label, num, title, stateLabel))

	// CI status
	if pr.CheckStatus != "" {
		icon := provider.StatusIcon(pr.CheckStatus)
		switch pr.CheckStatus {
		case "success":
			line += " " + ciSuccessStyle.Render(icon)
		case "failure":
			line += " " + ciFailStyle.Render(icon)
		case "pending":
			line += " " + ciPendStyle.Render(icon)
		}
	}

	return line
}

// cliNameForProvider returns the CLI tool name for a given provider.
func cliNameForProvider(p provider.Provider) string {
	switch p {
	case provider.ProviderGitHub:
		return "gh"
	case provider.ProviderGitLab:
		return "glab"
	default:
		return "cli"
	}
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
