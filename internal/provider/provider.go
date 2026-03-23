package provider

import "strings"

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
// Falls back to a GitHub provider if the provider cannot be determined.
func NewProvider(remoteURL string) PRProvider {
	p := DetectProvider(remoteURL)
	switch p {
	case ProviderGitHub:
		return &GitHubProvider{}
	case ProviderGitLab:
		return &GitLabProvider{}
	default:
		// Unknown providers (including Bitbucket) default to GitHub behavior (most common case).
		return &GitHubProvider{}
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
