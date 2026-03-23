package github

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
)

// GHAvailability represents whether the gh CLI is usable.
type GHAvailability int

const (
	// GHAvailable means gh is installed and authenticated.
	GHAvailable GHAvailability = iota
	// GHNotFound means gh is not installed or not in PATH.
	GHNotFound
	// GHNotAuthenticated means gh is installed but not authenticated.
	GHNotAuthenticated
)

// CheckGH performs a pre-flight check for the gh CLI.
// It verifies that gh is present in PATH and that the user is authenticated.
// Intended to be called once at startup, not on every refresh.
func CheckGH() GHAvailability {
	if _, err := exec.LookPath("gh"); err != nil {
		return GHNotFound
	}
	cmd := exec.Command("gh", "auth", "status")
	if err := cmd.Run(); err != nil {
		return GHNotAuthenticated
	}
	return GHAvailable
}

// PRInfo holds pull request information for a branch.
type PRInfo struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Draft  bool   `json:"isDraft"`
	URL    string `json:"url"`

	// CI check status: "success", "failure", "pending", or "" if unknown.
	CheckStatus string
}

// PRResult maps branch names to their PR info.
type PRResult map[string]*PRInfo

// FetchPRs looks up open PRs for the given branch names using `gh`.
// Returns results for branches that have an associated PR.
func FetchPRs(repoDir string, branches []string) PRResult {
	result := make(PRResult)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // limit concurrency

	for _, branch := range branches {
		if branch == "" {
			continue
		}
		wg.Add(1)
		go func(br string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			pr := fetchPR(repoDir, br)
			if pr != nil {
				mu.Lock()
				result[br] = pr
				mu.Unlock()
			}
		}(branch)
	}
	wg.Wait()
	return result
}

func fetchPR(repoDir, branch string) *PRInfo {
	cmd := exec.Command("gh", "pr", "view", branch,
		"--json", "number,title,state,isDraft,url,statusCheckRollup",
	)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var raw struct {
		Number             int    `json:"number"`
		Title              string `json:"title"`
		State              string `json:"state"`
		IsDraft            bool   `json:"isDraft"`
		URL                string `json:"url"`
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

// PRRef holds the information needed to fetch a PR into a worktree.
type PRRef struct {
	Number int    // PR number
	Repo   string // "owner/repo" (empty = current repo)
}

// ParsePRRef parses a PR reference string. Accepted formats:
//   - "123"              → PR #123 in the current repo
//   - "owner/repo#123"   → PR #123 in the given fork
func ParsePRRef(input string) (PRRef, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return PRRef{}, fmt.Errorf("empty PR reference")
	}

	if idx := strings.LastIndex(input, "#"); idx > 0 {
		// Fork format: owner/repo#123
		repo := input[:idx]
		numStr := input[idx+1:]
		n, err := parsePositiveInt(numStr)
		if err != nil {
			return PRRef{}, fmt.Errorf("invalid PR number %q: %w", numStr, err)
		}
		if !strings.Contains(repo, "/") {
			return PRRef{}, fmt.Errorf("invalid repo format %q: expected owner/repo", repo)
		}
		return PRRef{Number: n, Repo: repo}, nil
	}

	// Plain number.
	n, err := parsePositiveInt(input)
	if err != nil {
		return PRRef{}, fmt.Errorf("invalid PR number %q: %w", input, err)
	}
	return PRRef{Number: n}, nil
}

func parsePositiveInt(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a number")
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return 0, fmt.Errorf("must be positive")
	}
	return n, nil
}

// ValidatePR checks that a PR exists using the gh CLI.
// repoDir is the local repo directory (used as working dir for gh).
// Returns the PR head branch name if the PR is valid.
func ValidatePR(repoDir string, ref PRRef) (string, error) {
	args := []string{"pr", "view", fmt.Sprintf("%d", ref.Number),
		"--json", "number,headRefName",
	}
	if ref.Repo != "" {
		args = append(args, "--repo", ref.Repo)
	}

	cmd := exec.Command("gh", args...)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("PR #%d not found", ref.Number)
	}

	var raw struct {
		Number     int    `json:"number"`
		HeadRefName string `json:"headRefName"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return "", fmt.Errorf("failed to parse PR info: %w", err)
	}

	return raw.HeadRefName, nil
}

// StatusIcon returns a colored icon for the check status.
func StatusIcon(status string) string {
	switch status {
	case "success":
		return "✓"
	case "failure":
		return "✗"
	case "pending":
		return "●"
	default:
		return ""
	}
}

