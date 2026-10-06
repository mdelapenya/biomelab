package provider

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func fakeCLI(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fake CLI script")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$BIOME_CLI_ARGS\"\n" +
		"printf '%s\\n' \"$*\" >> \"$BIOME_CLI_CALLS\"\n" +
		"if [ -n \"$BIOME_CLI_EXIT\" ]; then exit \"$BIOME_CLI_EXIT\"; fi\n" +
		"if [ \"$2\" = view ] && [ -n \"$BIOME_CLI_DETAIL_RESPONSE\" ]; then printf '%s' \"$BIOME_CLI_DETAIL_RESPONSE\"; exit 0; fi\n" +
		"printf '%s' \"$BIOME_CLI_RESPONSE\"\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("BIOME_CLI_ARGS", filepath.Join(dir, "args"))
	t.Setenv("BIOME_CLI_CALLS", filepath.Join(dir, "calls"))
	t.Setenv("BIOME_CLI_DETAIL_RESPONSE", "")
	t.Setenv("BIOME_CLI_EXIT", "")
	return filepath.Join(dir, "args")
}

func cliArgs(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func TestGitLabLookupUsesDocumentedOutputAndSchema(t *testing.T) {
	argsPath := fakeCLI(t, "glab")
	t.Setenv("BIOME_CLI_RESPONSE", `[{"iid":31,"title":"MR","state":"opened","draft":true,"web_url":"https://gitlab.com/a/b/-/merge_requests/31","source_branch":"feature"}]`)
	t.Setenv("BIOME_CLI_DETAIL_RESPONSE", `{"iid":31,"title":"MR","state":"opened","draft":true,"web_url":"https://gitlab.com/a/b/-/merge_requests/31","source_branch":"feature","head_pipeline":{"status":"failed"}}`)
	got := (&GitLabProvider{}).FetchPRsDetailedContext(context.Background(), t.TempDir(), []string{"feature"})["feature"]
	if got.Err != nil || got.Info == nil || got.Info.Number != 31 || got.Info.URL == "" || got.Info.State != "open" || !got.Info.Draft || got.Info.CheckStatus != "failure" || got.Info.ReviewStatus != "" {
		t.Fatalf("wrong GitLab MR: %+v", got)
	}
	want := []string{"mr", "view", "31", "--output", "json"}
	if args := cliArgs(t, argsPath); !reflect.DeepEqual(args, want) {
		t.Fatalf("glab arguments = %q, want %q", args, want)
	}
	calls, err := os.ReadFile(filepath.Join(filepath.Dir(argsPath), "calls"))
	if err != nil || !strings.Contains(string(calls), "mr list --all --source-branch feature --per-page 100 --output json\n") {
		t.Fatalf("glab list call = %q, %v", calls, err)
	}
}

func TestGitLabLookupPrefersOpenMRAndRejectsWrongDetail(t *testing.T) {
	argsPath := fakeCLI(t, "glab")
	t.Setenv("BIOME_CLI_RESPONSE", `[{"iid":40,"state":"merged","web_url":"https://gitlab.com/a/b/-/merge_requests/40","source_branch":"feature"},{"iid":31,"state":"opened","web_url":"https://gitlab.com/a/b/-/merge_requests/31","source_branch":"feature"}]`)
	t.Setenv("BIOME_CLI_DETAIL_RESPONSE", `{"iid":31,"title":"current","state":"opened","web_url":"https://gitlab.com/a/b/-/merge_requests/31","source_branch":"feature"}`)
	got := (&GitLabProvider{}).FetchPRsDetailedContext(context.Background(), t.TempDir(), []string{"feature"})["feature"]
	if got.Err != nil || got.Info == nil || got.Info.Number != 31 {
		t.Fatalf("selected MR = %+v", got)
	}
	if args := cliArgs(t, argsPath); !reflect.DeepEqual(args, []string{"mr", "view", "31", "--output", "json"}) {
		t.Fatalf("view args = %q", args)
	}
	t.Setenv("BIOME_CLI_DETAIL_RESPONSE", `{"iid":40,"state":"merged","web_url":"https://gitlab.com/a/b/-/merge_requests/40","source_branch":"feature"}`)
	got = (&GitLabProvider{}).FetchPRsDetailedContext(context.Background(), t.TempDir(), []string{"feature"})["feature"]
	if got.Err == nil || got.Info != nil {
		t.Fatalf("wrong detail accepted: %+v", got)
	}
}

func TestGitLabLookupDistinguishesAbsenceAndErrors(t *testing.T) {
	fakeCLI(t, "glab")
	p := &GitLabProvider{}
	for _, tt := range []struct {
		output, exit   string
		absent, failed bool
	}{
		{output: "[]", absent: true},
		{output: "{broken", failed: true},
		{output: "null", failed: true},
		{output: "[]", exit: "1", failed: true},
	} {
		t.Setenv("BIOME_CLI_RESPONSE", tt.output)
		t.Setenv("BIOME_CLI_EXIT", tt.exit)
		got := p.FetchPRsDetailedContext(context.Background(), t.TempDir(), []string{"feature"})["feature"]
		if (got.Err != nil) != tt.failed || (got.Info == nil && got.Err == nil) != tt.absent {
			t.Errorf("output=%q exit=%q got %+v", tt.output, tt.exit, got)
		}
	}
}

func TestGitHubLookupUsesCurrentReviewDecision(t *testing.T) {
	argsPath := fakeCLI(t, "gh")
	t.Setenv("BIOME_CLI_RESPONSE", `[{"number":42,"title":"PR","state":"OPEN","url":"https://github.com/a/b/pull/42","headRefName":"feature","reviewDecision":"CHANGES_REQUESTED","latestReviews":[{"state":"APPROVED"}]}]`)
	got := (&GitHubProvider{}).FetchPRsDetailedContext(context.Background(), t.TempDir(), []string{"feature"})["feature"]
	if got.Err != nil || got.Info == nil || got.Info.ReviewStatus != "changes_requested" {
		t.Fatalf("stale approval defeated current decision: %+v", got)
	}
	args := cliArgs(t, argsPath)
	if len(args) < 3 || args[0] != "pr" || args[1] != "list" || !strings.Contains(strings.Join(args, ","), "reviewDecision") {
		t.Fatalf("wrong gh arguments: %q", args)
	}
	t.Setenv("BIOME_CLI_RESPONSE", `[{"number":42,"title":"PR","state":"OPEN","url":"https://github.com/a/b/pull/42","headRefName":"feature","latestReviews":[{"state":"APPROVED"},{"state":"CHANGES_REQUESTED"}]}]`)
	got = (&GitHubProvider{}).FetchPRsDetailedContext(context.Background(), t.TempDir(), []string{"feature"})["feature"]
	if got.Err != nil || got.Info.ReviewStatus != "changes_requested" {
		t.Fatalf("mixed current reviews reported approval: %+v", got)
	}
}

func TestGitHubLookupDistinguishesAbsenceAndErrors(t *testing.T) {
	fakeCLI(t, "gh")
	p := &GitHubProvider{}
	for _, tt := range []struct {
		output, exit   string
		absent, failed bool
	}{
		{output: "[]", absent: true},
		{output: "{broken", failed: true},
		{output: "null", failed: true},
		{output: "[]", exit: "1", failed: true},
	} {
		t.Setenv("BIOME_CLI_RESPONSE", tt.output)
		t.Setenv("BIOME_CLI_EXIT", tt.exit)
		got := p.FetchPRsDetailedContext(context.Background(), t.TempDir(), []string{"feature"})["feature"]
		if (got.Err != nil) != tt.failed || (got.Info == nil && got.Err == nil) != tt.absent {
			t.Errorf("output=%q exit=%q got %+v", tt.output, tt.exit, got)
		}
	}
}

func TestGitHubAuthCheckScopesHost(t *testing.T) {
	argsPath := fakeCLI(t, "gh")
	if got := (&GitHubProvider{}).CheckCLI(); got != CLIAvailable {
		t.Fatalf("auth check = %v", got)
	}
	if args := cliArgs(t, argsPath); !reflect.DeepEqual(args, []string{"auth", "status", "--hostname", "github.com"}) {
		t.Fatalf("auth arguments = %q", args)
	}
}

func TestDetectProviderUsesHostOnly(t *testing.T) {
	for _, tt := range []struct {
		remote string
		want   Provider
	}{
		{"https://gitlab.com/team/github.com-mirror.git", ProviderGitLab},
		{"git@gitlab.com:team/github.com-mirror.git", ProviderGitLab},
		{"ssh://git@github.com:2222/team/gitlab.com-mirror.git", ProviderGitHub},
		{"https://github.com.evil.example/team/repo.git", ProviderUnknown},
		{"https://example.com/github.com/team/repo", ProviderUnknown},
	} {
		if got := DetectProvider(tt.remote); got != tt.want {
			t.Errorf("DetectProvider(%q) = %v, want %v", tt.remote, got, tt.want)
		}
	}
}
