package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mdelapenya/biomelab/internal/command"
)

// GitLabProvider fetches MR information using the glab CLI.
type GitLabProvider struct{}

// CheckCLI verifies that glab is installed and authenticated.
func (g *GitLabProvider) CheckCLI() CLIAvailability {
	return g.CheckCLIContext(context.Background())
}

func (g *GitLabProvider) CheckCLIContext(ctx context.Context) CLIAvailability {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, err := exec.LookPath("glab"); err != nil {
		return CLINotFound
	}
	cmd := command.BackgroundContext(ctx, "glab", "auth", "status")
	if err := cmd.Run(); err != nil {
		return CLINotAuthenticated
	}
	return CLIAvailable
}

// FetchPRs looks up open MRs for the given branch names using glab.
func (g *GitLabProvider) FetchPRs(repoDir string, branches []string) PRResult {
	return g.FetchPRsContext(context.Background(), repoDir, branches)
}

func (g *GitLabProvider) FetchPRsContext(ctx context.Context, repoDir string, branches []string) PRResult {
	return fetchPRsConcurrent(ctx, repoDir, branches, func(dir, branch string) *PRInfo {
		return fetchGitLabMRContext(ctx, dir, branch)
	})
}

// Name returns "GitLab".
func (g *GitLabProvider) Name() string { return "GitLab" }

// Provider returns ProviderGitLab.
func (g *GitLabProvider) Provider() Provider { return ProviderGitLab }

// CreatePR creates a merge request on GitLab using the glab CLI.
// Without either override, it runs "glab mr create --fill --source-branch <branch>"
// so commit messages become the title and description. When title is non-empty,
// it is passed as --title; when bodyFile is non-empty, it is passed as
// --description-file. Mixed cases fill in the remaining side from defaults
// (commit subject for title, --fill for description). An optional
// "--repo <targetRepo>" is passed through in any case. After creation, full
// MR info is fetched via "glab mr view".
func (g *GitLabProvider) CreatePR(repoDir, branch, targetRepo, title, bodyFile string) (*PRInfo, error) {
	args := []string{"mr", "create", "--source-branch", branch}
	hasOverride := title != "" || bodyFile != ""
	if hasOverride {
		effectiveTitle := title
		if effectiveTitle == "" {
			subj, terr := commitSubject(repoDir, branch)
			if terr != nil || subj == "" {
				subj = branch
			}
			effectiveTitle = subj
		}
		args = append(args, "--title", effectiveTitle)
		if bodyFile != "" {
			args = append(args, "--description-file", bodyFile)
		} else {
			args = append(args, "--fill")
		}
	} else {
		args = append(args, "--fill")
	}
	if targetRepo != "" {
		args = append(args, "--repo", targetRepo)
	}
	cmd := command.Background("glab", args...)
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
	return fetchGitLabMRContext(context.Background(), repoDir, branch)
}

func fetchGitLabMRContext(ctx context.Context, repoDir, branch string) *PRInfo {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := command.BackgroundContext(ctx, "glab", "mr", "view", branch,
		"--json", "iid,title,state,draft,webUrl,headPipeline,approvedBy",
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
		ApprovedBy []struct {
			Username string `json:"username"`
		} `json:"approvedBy"`
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
	if len(raw.ApprovedBy) > 0 {
		pr.ReviewStatus = "approved"
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
