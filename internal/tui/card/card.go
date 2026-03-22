package card

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/mdelapenya/gwaim/internal/agent"
	"github.com/mdelapenya/gwaim/internal/git"
	"github.com/mdelapenya/gwaim/internal/github"
)

var (
	branchStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	agentActiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	agentDetailStyle = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("42"))
	agentNoneStyle   = lipgloss.NewStyle().Faint(true)
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

	ghUnavailStyle = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("214"))

	syncUpToDateStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	syncNeedSyncStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	syncDivergedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	syncUnknownStyle  = lipgloss.NewStyle().Faint(true)
)

// Render produces the content for a single worktree card.
func Render(wt git.Worktree, agents []agent.Info, pr *github.PRInfo, ghAvail github.GHAvailability) string {
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
	b.WriteString("\n\n")

	// PR status
	switch {
	case pr != nil:
		b.WriteString(renderPR(pr))
		b.WriteString("\n")
	case ghAvail == github.GHNotFound:
		b.WriteString(ghUnavailStyle.Render("gh not installed — install gh CLI"))
		b.WriteString("\n")
	case ghAvail == github.GHNotAuthenticated:
		b.WriteString(ghUnavailStyle.Render("gh not authenticated — run: gh auth login"))
		b.WriteString("\n")
	}

	// Agent status
	if len(agents) > 0 {
		for _, a := range agents {
			line := fmt.Sprintf("%s (PID %s)", string(a.Kind), a.PID)
			b.WriteString(agentActiveStyle.Render("● " + line))
			b.WriteString("\n")
			detail := fmt.Sprintf("  state: %s  started: %s", a.State, a.Started)
			b.WriteString(agentDetailStyle.Render(detail))
			b.WriteString("\n")
		}
	} else {
		b.WriteString(agentNoneStyle.Render("○ no agent"))
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

func renderPR(pr *github.PRInfo) string {
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

	line := style.Render(fmt.Sprintf("PR %s %s (%s)", num, title, stateLabel))

	// CI status
	if pr.CheckStatus != "" {
		icon := github.StatusIcon(pr.CheckStatus)
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

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
