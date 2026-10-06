package provider

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
)

// Provider represents a git hosting provider.
type Provider int

const (
	// ProviderUnknown means the provider could not be determined.
	ProviderUnknown Provider = iota
	// ProviderGitHub represents GitHub.
	ProviderGitHub
	// ProviderGitLab represents GitLab.
	ProviderGitLab
)

// String returns the human-readable name of the provider.
func (p Provider) String() string {
	switch p {
	case ProviderGitHub:
		return "GitHub"
	case ProviderGitLab:
		return "GitLab"
	default:
		return "Unknown"
	}
}

// DetectProvider determines the hosting provider from a remote URL.
// Supports both SSH (git@host:owner/repo.git) and HTTPS (https://host/owner/repo.git) formats.
// Self-hosted instances are detected via hostname patterns (e.g., gitlab.mycompany.com).
func DetectProvider(remoteURL string) Provider {
	host := remoteHost(remoteURL)
	switch {
	case host == "github.com":
		return ProviderGitHub
	case host == "gitlab.com", strings.HasPrefix(host, "gitlab."), strings.HasSuffix(host, ".gitlab.com"):
		return ProviderGitLab
	default:
		return ProviderUnknown
	}
}

func remoteHost(remote string) string {
	if u, err := url.Parse(remote); err == nil && u.Scheme != "" && u.Hostname() != "" {
		return strings.ToLower(u.Hostname())
	}
	// Git's SCP-style remotes have no URL scheme: user@host:path.
	if before, _, ok := strings.Cut(remote, ":"); ok && !strings.ContainsAny(before, `/\\`) {
		if _, host, hasUser := strings.Cut(before, "@"); hasUser {
			return strings.ToLower(host)
		}
		return strings.ToLower(before)
	}
	return ""
}

// CLIAvailability represents whether a provider's CLI tool is usable.
type CLIAvailability int

const (
	// CLIAvailable means the CLI is installed and authenticated.
	CLIAvailable CLIAvailability = iota
	// CLINotFound means the CLI is not installed or not in PATH.
	CLINotFound
	// CLINotAuthenticated means the CLI is installed but not authenticated.
	CLINotAuthenticated
	// CLIUnsupportedProvider means the provider has no CLI integration yet.
	CLIUnsupportedProvider
)

// PRInfo holds pull/merge request information for a branch.
type PRInfo struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Draft  bool   `json:"isDraft"`
	URL    string `json:"url"`

	// CI check status: "success", "failure", "pending", or "" if unknown.
	CheckStatus string

	// ReviewStatus is the current review state known for the PR/MR.
	// Values: "approved", "changes_requested", "commented", or "" (no reviews).
	ReviewStatus string
}

// PRResult maps branch names to their PR info.
type PRResult map[string]*PRInfo

// PRLookup distinguishes a confirmed absence (Info and Err both nil) from a
// failed lookup (Err non-nil). Consumers can retain old data only on failure.
type PRLookup struct {
	Info *PRInfo
	Err  error
}

type PRLookupResult map[string]PRLookup

// DetailedPRProvider is an optional extension of PRProvider. Each requested
// non-empty branch receives a result, including canceled and failed lookups.
type DetailedPRProvider interface {
	FetchPRsDetailedContext(context.Context, string, []string) PRLookupResult
}

// PRProvider fetches PR/MR information for branches from a hosting provider.
type PRProvider interface {
	// CheckCLI performs a pre-flight check for the provider's CLI tool.
	// Intended to be called once at startup.
	CheckCLI() CLIAvailability
	CheckCLIContext(context.Context) CLIAvailability

	// FetchPRs looks up PRs/MRs for the given branch names.
	// Returns results for branches that have an associated PR/MR.
	FetchPRs(repoDir string, branches []string) PRResult
	FetchPRsContext(context.Context, string, []string) PRResult

	// CreatePR pushes the branch (if needed) and creates a PR/MR.
	// targetRepo is "owner/repo" derived from the selected remote;
	// if empty, the CLI's default repo detection is used.
	// title, when non-empty, is passed as the PR/MR title (overriding the
	// usual commit-subject derivation). When empty, the implementation
	// derives the title from the branch's latest commit.
	// bodyFile, when non-empty, is the absolute path to a markdown file
	// whose contents become the PR/MR description (replacing the default
	// commit-derived body). When empty, the CLI's --fill default is used.
	CreatePR(repoDir, branch, targetRepo, title, bodyFile string) (*PRInfo, error)

	// Name returns the display name of the provider (e.g., "GitHub", "GitLab").
	Name() string

	// Provider returns the provider type.
	Provider() Provider
}

// NewProvider creates the appropriate PRProvider for the given remote URL.
// Returns an UnsupportedProvider for unrecognized hosting platforms.
func NewProvider(remoteURL string) PRProvider {
	p := DetectProvider(remoteURL)
	switch p {
	case ProviderGitHub:
		return &GitHubProvider{}
	case ProviderGitLab:
		return &GitLabProvider{}
	default:
		return NewUnsupportedProvider(p)
	}
}

// StatusIcon returns a colored icon for the CI check status.
func StatusIcon(status string) string {
	switch status {
	case "success":
		return "\u2713"
	case "failure":
		return "\u2717"
	case "pending":
		return "\u25cf"
	default:
		return ""
	}
}

// UnsupportedProvider is a no-op provider for hosting platforms without CLI support.
type UnsupportedProvider struct {
	provider Provider
}

// NewUnsupportedProvider creates a provider that always returns CLIUnsupportedProvider.
func NewUnsupportedProvider(p Provider) *UnsupportedProvider {
	return &UnsupportedProvider{provider: p}
}

// CheckCLI always returns CLIUnsupportedProvider.
func (u *UnsupportedProvider) CheckCLI() CLIAvailability {
	return CLIUnsupportedProvider
}

// FetchPRs always returns an empty result.
func (u *UnsupportedProvider) FetchPRs(_ string, _ []string) PRResult {
	return make(PRResult)
}

// CreatePR always returns an error for unsupported providers.
func (u *UnsupportedProvider) CreatePR(_, _, _, _, _ string) (*PRInfo, error) {
	return nil, fmt.Errorf("PR creation not supported for %s", u.provider)
}

// Name returns the provider name.
func (u *UnsupportedProvider) Name() string {
	return u.provider.String()
}

// Provider returns the provider type.
func (u *UnsupportedProvider) Provider() Provider {
	return u.provider
}

func fetchPRsDetailedConcurrent(ctx context.Context, repoDir string, branches []string, fetchFn func(string, string) (*PRInfo, error)) PRLookupResult {
	result := make(PRLookupResult, len(branches))
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
			select {
			case <-ctx.Done():
				mu.Lock()
				result[br] = PRLookup{Err: ctx.Err()}
				mu.Unlock()
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				mu.Lock()
				result[br] = PRLookup{Err: err}
				mu.Unlock()
				return
			}
			info, err := fetchFn(repoDir, br)
			mu.Lock()
			result[br] = PRLookup{Info: info, Err: err}
			mu.Unlock()
		}(branch)
	}
	wg.Wait()
	return result
}

func successfulPRs(lookups PRLookupResult) PRResult {
	result := make(PRResult)
	for branch, lookup := range lookups {
		if lookup.Err == nil && lookup.Info != nil {
			result[branch] = lookup.Info
		}
	}
	return result
}

func (u *UnsupportedProvider) CheckCLIContext(context.Context) CLIAvailability { return u.CheckCLI() }

func (u *UnsupportedProvider) FetchPRsContext(_ context.Context, dir string, branches []string) PRResult {
	return u.FetchPRs(dir, branches)
}
