package provider

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
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
	return fetchPRsConcurrent(repoDir, branches, fetchGitLabMR)
}

// Name returns "GitLab".
func (g *GitLabProvider) Name() string { return "GitLab" }

// Provider returns ProviderGitLab.
func (g *GitLabProvider) Provider() Provider { return ProviderGitLab }

// CreatePR creates a merge request on GitLab using the glab CLI.
// It runs "glab mr create --fill --source-branch <branch>" with an optional "--repo <targetRepo>".
// After creation, it fetches full MR info via "glab mr view".
func (g *GitLabProvider) CreatePR(repoDir, branch, targetRepo string) (*PRInfo, error) {
	args := []string{"mr", "create", "--fill", "--source-branch", branch}
	if targetRepo != "" {
		args = append(args, "--repo", targetRepo)
	}
	cmd := exec.Command("glab", args...)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("glab mr create: %s", strings.TrimSpace(string(out)))
	}

	// glab mr create prints the MR URL on success. Fetch full info via glab mr view.
	mr := fetchGitLabMR(repoDir, branch)
	if mr == nil {
		url := strings.TrimSpace(string(out))
		if lines := strings.Split(url, "\n"); len(lines) > 0 {
			url = strings.TrimSpace(lines[len(lines)-1])
		}
		return &PRInfo{URL: url, State: "open"}, nil
	}
	return mr, nil
}

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
