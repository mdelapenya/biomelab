package provider

import (
	"encoding/json"
	"os/exec"
	"strings"
)

// GitHubProvider fetches PR information using the gh CLI.
type GitHubProvider struct{}

// CheckCLI verifies that gh is installed and authenticated.
func (g *GitHubProvider) CheckCLI() CLIAvailability {
	if _, err := exec.LookPath("gh"); err != nil {
		return CLINotFound
	}
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return CLINotAuthenticated
	}
	return CLIAvailable
}

// FetchPRs looks up open PRs for the given branch names using gh.
func (g *GitHubProvider) FetchPRs(repoDir string, branches []string) PRResult {
	return fetchPRsConcurrent(repoDir, branches, fetchGitHubPR)
}

// Name returns "GitHub".
func (g *GitHubProvider) Name() string { return "GitHub" }

// Provider returns ProviderGitHub.
func (g *GitHubProvider) Provider() Provider { return ProviderGitHub }

func fetchGitHubPR(repoDir, branch string) *PRInfo {
	cmd := exec.Command("gh", "pr", "view", branch,
		"--json", "number,title,state,isDraft,url,statusCheckRollup",
	)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var raw struct {
		Number            int    `json:"number"`
		Title             string `json:"title"`
		State             string `json:"state"`
		IsDraft           bool   `json:"isDraft"`
		URL               string `json:"url"`
		StatusCheckRollup []struct {
			State      string `json:"state"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		} `json:"statusCheckRollup"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil
	}

	pr := &PRInfo{
		Number: raw.Number,
		Title:  raw.Title,
		State:  strings.ToLower(raw.State),
		Draft:  raw.IsDraft,
		URL:    raw.URL,
	}

	pr.CheckStatus = rollupStatus(raw.StatusCheckRollup)
	return pr
}

func rollupStatus(checks []struct {
	State      string `json:"state"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}) string {
	if len(checks) == 0 {
		return ""
	}

	hasFailure := false
	hasPending := false
	for _, c := range checks {
		conclusion := strings.ToLower(c.Conclusion)
		status := strings.ToLower(c.Status)
		state := strings.ToLower(c.State)

		if conclusion == "failure" || conclusion == "timed_out" || conclusion == "cancelled" || state == "failure" || state == "error" {
			hasFailure = true
		} else if status == "in_progress" || status == "queued" || state == "pending" || conclusion == "" {
			hasPending = true
		}
	}

	if hasFailure {
		return "failure"
	}
	if hasPending {
		return "pending"
	}
	return "success"
}
