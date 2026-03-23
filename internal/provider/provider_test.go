package provider

import (
	"testing"
)

func TestDetectProvider(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		want      Provider
	}{
		// GitHub
		{"github ssh", "git@github.com:owner/repo.git", ProviderGitHub},
		{"github https", "https://github.com/owner/repo.git", ProviderGitHub},
		{"github https no .git", "https://github.com/owner/repo", ProviderGitHub},

		// GitLab
		{"gitlab ssh", "git@gitlab.com:owner/repo.git", ProviderGitLab},
		{"gitlab https", "https://gitlab.com/owner/repo.git", ProviderGitLab},
		{"gitlab self-hosted", "git@gitlab.mycompany.com:team/project.git", ProviderGitLab},
		{"gitlab self-hosted https", "https://gitlab.internal.io/team/project.git", ProviderGitLab},

		// Unknown (includes Bitbucket and other unsupported providers)
		{"bitbucket ssh", "git@bitbucket.org:owner/repo.git", ProviderUnknown},
		{"bitbucket https", "https://bitbucket.org/owner/repo.git", ProviderUnknown},
		{"unknown", "git@custom.host:owner/repo.git", ProviderUnknown},
		{"empty", "", ProviderUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectProvider(tt.remoteURL)
			if got != tt.want {
				t.Errorf("DetectProvider(%q) = %v, want %v", tt.remoteURL, got, tt.want)
			}
		})
	}
}

func TestDetectProvider_CaseInsensitive(t *testing.T) {
	got := DetectProvider("git@GITHUB.COM:owner/repo.git")
	if got != ProviderGitHub {
		t.Errorf("expected ProviderGitHub for uppercase URL, got %v", got)
	}
}

func TestProvider_String(t *testing.T) {
	tests := []struct {
		p    Provider
		want string
	}{
		{ProviderGitHub, "GitHub"},
		{ProviderGitLab, "GitLab"},
		{ProviderUnknown, "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.p.String(); got != tt.want {
			t.Errorf("%d.String() = %q, want %q", tt.p, got, tt.want)
		}
	}
}

func TestStatusIcon(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"success", "\u2713"},
		{"failure", "\u2717"},
		{"pending", "\u25cf"},
		{"", ""},
		{"unknown", ""},
	}
	for _, tc := range cases {
		got := StatusIcon(tc.status)
		if got != tc.want {
			t.Errorf("StatusIcon(%q) = %q, want %q", tc.status, got, tc.want)
		}
	}
}

func TestCLIAvailability_Constants(t *testing.T) {
	// Ensure all constants are distinct.
	vals := map[CLIAvailability]bool{
		CLIAvailable:           true,
		CLINotFound:            true,
		CLINotAuthenticated:    true,
		CLIUnsupportedProvider: true,
	}
	if len(vals) != 4 {
		t.Error("CLIAvailability constants must be distinct")
	}
}

func TestUnsupportedProvider(t *testing.T) {
	p := NewUnsupportedProvider(ProviderUnknown)

	if p.CheckCLI() != CLIUnsupportedProvider {
		t.Error("expected CLIUnsupportedProvider")
	}
	if p.Name() != "Unknown" {
		t.Errorf("Name() = %q, want Unknown", p.Name())
	}
	if p.Provider() != ProviderUnknown {
		t.Errorf("Provider() = %v, want ProviderUnknown", p.Provider())
	}

	result := p.FetchPRs("/tmp", []string{"main"})
	if len(result) != 0 {
		t.Errorf("expected empty PRResult, got %d entries", len(result))
	}
}

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name      string
		remoteURL string
		wantType  string
		wantProv  Provider
	}{
		{"github", "git@github.com:o/r.git", "GitHub", ProviderGitHub},
		{"gitlab", "git@gitlab.com:o/r.git", "GitLab", ProviderGitLab},
		{"unknown is unsupported", "git@custom.host:o/r.git", "Unknown", ProviderUnknown},
		{"bitbucket is unsupported", "git@bitbucket.org:o/r.git", "Unknown", ProviderUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProvider(tt.remoteURL)
			if p.Name() != tt.wantType {
				t.Errorf("NewProvider(%q).Name() = %q, want %q", tt.remoteURL, p.Name(), tt.wantType)
			}
		})
	}
}

func TestGitHubProvider_Interface(t *testing.T) {
	var _ PRProvider = &GitHubProvider{}
}

func TestGitLabProvider_Interface(t *testing.T) {
	var _ PRProvider = &GitLabProvider{}
}

func TestUnsupportedProvider_Interface(t *testing.T) {
	var _ PRProvider = &UnsupportedProvider{}
}

func TestRollupStatus(t *testing.T) {
	tests := []struct {
		name   string
		checks []struct {
			State      string `json:"state"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		}
		want string
	}{
		{"empty", nil, ""},
		{"all success", []struct {
			State      string `json:"state"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		}{
			{Conclusion: "success", Status: "completed"},
			{Conclusion: "success", Status: "completed"},
		}, "success"},
		{"any failure", []struct {
			State      string `json:"state"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		}{
			{Conclusion: "success", Status: "completed"},
			{Conclusion: "failure", Status: "completed"},
		}, "failure"},
		{"pending when in progress", []struct {
			State      string `json:"state"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		}{
			{Conclusion: "success", Status: "completed"},
			{Status: "in_progress"},
		}, "pending"},
		{"failure over pending", []struct {
			State      string `json:"state"`
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
		}{
			{Conclusion: "failure"},
			{Status: "in_progress"},
		}, "failure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rollupStatus(tt.checks)
			if got != tt.want {
				t.Errorf("rollupStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMapGitLabState(t *testing.T) {
	tests := []struct {
		state string
		want  string
	}{
		{"opened", "open"},
		{"merged", "merged"},
		{"closed", "closed"},
		{"Opened", "open"},
		{"custom", "custom"},
	}
	for _, tt := range tests {
		got := mapGitLabState(tt.state)
		if got != tt.want {
			t.Errorf("mapGitLabState(%q) = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestMapGitLabPipelineStatus(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"success", "success"},
		{"failed", "failure"},
		{"canceled", "failure"},
		{"running", "pending"},
		{"pending", "pending"},
		{"created", "pending"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := mapGitLabPipelineStatus(tt.status)
		if got != tt.want {
			t.Errorf("mapGitLabPipelineStatus(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestGitHubCheckCLI_NotFound(t *testing.T) {
	t.Setenv("PATH", "")
	p := &GitHubProvider{}
	got := p.CheckCLI()
	if got != CLINotFound {
		t.Errorf("expected CLINotFound, got %v", got)
	}
}

func TestGitLabCheckCLI_NotFound(t *testing.T) {
	t.Setenv("PATH", "")
	p := &GitLabProvider{}
	got := p.CheckCLI()
	if got != CLINotFound {
		t.Errorf("expected CLINotFound, got %v", got)
	}
}
