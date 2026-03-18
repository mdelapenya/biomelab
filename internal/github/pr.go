package github

import (
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
)

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

