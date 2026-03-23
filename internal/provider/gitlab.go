package provider

import (
	"encoding/json"
	"os/exec"
	"strings"
	"sync"
)

// GitLabProvider fetches MR information using the glab CLI.
type GitLabProvider struct{}

// CheckCLI verifies that glab is installed and authenticated.
func (g *GitLabProvider) CheckCLI() CLIAvailability {
	if _, err := exec.LookPath("glab"); err != nil {
		return CLINotFound
	}
	cmd := exec.Command("glab", "auth", "status")
	if err := cmd.Run(); err != nil {
		return CLINotAuthenticated
	}
	return CLIAvailable
}

// FetchPRs looks up open MRs for the given branch names using glab.
func (g *GitLabProvider) FetchPRs(repoDir string, branches []string) PRResult {
	result := make(PRResult)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)

	for _, branch := range branches {
		if branch == "" {
			continue
		}
		wg.Add(1)
		go func(br string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			mr := fetchGitLabMR(repoDir, br)
			if mr != nil {
				mu.Lock()
				result[br] = mr
				mu.Unlock()
			}
		}(branch)
	}
	wg.Wait()
	return result
}

// Name returns "GitLab".
func (g *GitLabProvider) Name() string { return "GitLab" }

// Provider returns ProviderGitLab.
func (g *GitLabProvider) Provider() Provider { return ProviderGitLab }

func fetchGitLabMR(repoDir, branch string) *PRInfo {
	cmd := exec.Command("glab", "mr", "view", branch,
		"--json", "iid,title,state,draft,webUrl,headPipeline",
	)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var raw struct {
		IID          int    `json:"iid"`
		Title        string `json:"title"`
		State        string `json:"state"`
		Draft        bool   `json:"draft"`
		WebURL       string `json:"webUrl"`
		HeadPipeline *struct {
			Status string `json:"status"`
		} `json:"headPipeline"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil
	}

	pr := &PRInfo{
		Number: raw.IID,
		Title:  raw.Title,
		State:  mapGitLabState(raw.State),
		Draft:  raw.Draft,
		URL:    raw.WebURL,
	}

	if raw.HeadPipeline != nil {
		pr.CheckStatus = mapGitLabPipelineStatus(raw.HeadPipeline.Status)
	}

	return pr
}

// mapGitLabState normalizes GitLab MR states to the common format used by PRInfo.
func mapGitLabState(state string) string {
	switch strings.ToLower(state) {
	case "opened":
		return "open"
	case "merged":
		return "merged"
	case "closed":
		return "closed"
	default:
		return strings.ToLower(state)
	}
}

// mapGitLabPipelineStatus normalizes GitLab pipeline statuses to success/failure/pending.
func mapGitLabPipelineStatus(status string) string {
	switch strings.ToLower(status) {
	case "success":
		return "success"
	case "failed", "canceled":
		return "failure"
	case "running", "pending", "created", "waiting_for_resource", "preparing":
		return "pending"
	default:
		return ""
	}
}
