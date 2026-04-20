package provider

import (
	"encoding/json"
	"fmt"
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

// CreatePR creates a pull request on GitHub using the gh CLI.
// It runs "gh pr create --fill --head <branch>" with an optional "--repo <targetRepo>".
// After creation, it fetches full PR info via "gh pr view".
func (g *GitHubProvider) CreatePR(repoDir, branch, targetRepo string) (*PRInfo, error) {
	args := []string{"pr", "create", "--fill", "--head", branch}
	if targetRepo != "" {
		args = append(args, "--repo", targetRepo)
	}
	cmd := exec.Command("gh", args...)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("gh pr create: %s", strings.TrimSpace(string(out)))
	}

	// gh pr create prints the PR URL on success. Fetch full info via gh pr view.
	pr := fetchGitHubPR(repoDir, branch)
	if pr == nil {
		// Fallback: return minimal info from the create output (URL on last line).
		url := strings.TrimSpace(string(out))
		// Extract last line which is typically the URL.
		if lines := strings.Split(url, "\n"); len(lines) > 0 {
			url = strings.TrimSpace(lines[len(lines)-1])
		}
		return &PRInfo{URL: url, State: "open"}, nil
	}
	return pr, nil
}

func fetchGitHubPR(repoDir, branch string) *PRInfo {
	cmd := exec.Command("gh", "pr", "view", branch,
		"--json", "number,title,state,isDraft,url,statusCheckRollup,reviews",
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
		Reviews []struct {
			State string `json:"state"`
		} `json:"reviews"`
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
	pr.ReviewStatus = githubReviewStatus(raw.Reviews)
	return pr
}

// githubReviewStatus returns the most significant review state from a list of
// GitHub reviews. Priority: approved > changes_requested > commented.
func githubReviewStatus(reviews []struct{ State string `json:"state"` }) string {
	hasApproved := false
	hasChanges := false
	hasComment := false
	for _, r := range reviews {
		switch strings.ToUpper(r.State) {
		case "APPROVED":
			hasApproved = true
		case "CHANGES_REQUESTED":
			hasChanges = true
		case "COMMENTED":
			hasComment = true
		}
	}
	switch {
	case hasApproved:
		return "approved"
	case hasChanges:
		return "changes_requested"
	case hasComment:
		return "commented"
	default:
		return ""
	}
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
