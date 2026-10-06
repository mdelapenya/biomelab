package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
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

// FetchPRs looks up MRs in any state for the given branch names using glab.
func (g *GitLabProvider) FetchPRs(repoDir string, branches []string) PRResult {
	return g.FetchPRsContext(context.Background(), repoDir, branches)
}

func (g *GitLabProvider) FetchPRsContext(ctx context.Context, repoDir string, branches []string) PRResult {
	return successfulPRs(g.FetchPRsDetailedContext(ctx, repoDir, branches))
}

func (g *GitLabProvider) FetchPRsDetailedContext(ctx context.Context, repoDir string, branches []string) PRLookupResult {
	return fetchPRsDetailedConcurrent(ctx, repoDir, branches, func(dir, branch string) (*PRInfo, error) {
		return lookupGitLabMRContext(ctx, dir, branch)
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
	mr, _ := lookupGitLabMRContext(ctx, repoDir, branch)
	return mr
}

func lookupGitLabMRContext(ctx context.Context, repoDir, branch string) (*PRInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := command.BackgroundContext(ctx, "glab", "mr", "list", "--all",
		"--source-branch", branch, "--per-page", "100", "--output", "json",
	)
	cmd.Dir = repoDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("glab mr list %q: %w", branch, err)
	}

	var raw []struct {
		IID          int    `json:"iid"`
		Title        string `json:"title"`
		State        string `json:"state"`
		Draft        bool   `json:"draft"`
		WebURL       string `json:"web_url"`
		SourceBranch string `json:"source_branch"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse glab MR list %q: %w", branch, err)
	}
	if raw == nil {
		return nil, fmt.Errorf("glab MR list %q returned null", branch)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	selected := -1
	for i := range raw {
		if raw[i].SourceBranch == branch && raw[i].IID > 0 && raw[i].WebURL != "" {
			// An active MR is the current branch's review even if an
			// older branch reuse has a higher historical IID.
			open := mapGitLabState(raw[i].State) == "open"
			selectedOpen := selected >= 0 && mapGitLabState(raw[selected].State) == "open"
			if selected < 0 || (open && !selectedOpen) || (open == selectedOpen && raw[i].IID > raw[selected].IID) {
				selected = i
			}
		}
	}
	if selected < 0 {
		return nil, fmt.Errorf("glab MR list %q returned no valid matching MR", branch)
	}
	r := raw[selected]
	// glab's list JSON is BasicMergeRequest, which has no head_pipeline.
	// Fetch the selected IID as a full MergeRequest to get current CI status.
	detailCmd := command.BackgroundContext(ctx, "glab", "mr", "view", strconv.Itoa(r.IID), "--output", "json")
	detailCmd.Dir = repoDir
	detail, err := detailCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("glab mr view !%d: %w", r.IID, err)
	}
	var full struct {
		IID          int    `json:"iid"`
		Title        string `json:"title"`
		State        string `json:"state"`
		Draft        bool   `json:"draft"`
		WebURL       string `json:"web_url"`
		SourceBranch string `json:"source_branch"`
		HeadPipeline *struct {
			Status string `json:"status"`
		} `json:"head_pipeline"`
	}
	if err := json.Unmarshal(detail, &full); err != nil {
		return nil, fmt.Errorf("parse glab MR view !%d: %w", r.IID, err)
	}
	if full.IID != r.IID || full.SourceBranch != branch || full.WebURL == "" {
		return nil, fmt.Errorf("glab MR view !%d returned a different MR", r.IID)
	}

	pr := &PRInfo{
		Number: full.IID,
		Title:  full.Title,
		State:  mapGitLabState(full.State),
		Draft:  full.Draft,
		URL:    full.WebURL,
	}

	if full.HeadPipeline != nil {
		pr.CheckStatus = mapGitLabPipelineStatus(full.HeadPipeline.Status)
	}
	// Merge-request approval state lives behind a separate approvals endpoint.
	// The MR response alone cannot prove requirements were satisfied.
	return pr, nil
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
