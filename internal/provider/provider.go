package provider

import (
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
	lower := strings.ToLower(remoteURL)
	switch {
	case strings.Contains(lower, "github.com"):
		return ProviderGitHub
	case strings.Contains(lower, "gitlab.com"), strings.Contains(lower, "gitlab."):
		return ProviderGitLab
	default:
		return ProviderUnknown
	}
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
}

// PRResult maps branch names to their PR info.
type PRResult map[string]*PRInfo

// PRProvider fetches PR/MR information for branches from a hosting provider.
type PRProvider interface {
	// CheckCLI performs a pre-flight check for the provider's CLI tool.
	// Intended to be called once at startup.
	CheckCLI() CLIAvailability

	// FetchPRs looks up open PRs/MRs for the given branch names.
	// Returns results for branches that have an associated PR/MR.
	FetchPRs(repoDir string, branches []string) PRResult

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

// Name returns the provider name.
func (u *UnsupportedProvider) Name() string {
	return u.provider.String()
}

// Provider returns the provider type.
func (u *UnsupportedProvider) Provider() Provider {
	return u.provider
}

// fetchPRsConcurrent fetches PR/MR info for multiple branches concurrently,
// using the provided fetch function for each branch. Limits concurrency to 4.
func fetchPRsConcurrent(repoDir string, branches []string, fetchFn func(repoDir, branch string) *PRInfo) PRResult {
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

			pr := fetchFn(repoDir, br)
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
