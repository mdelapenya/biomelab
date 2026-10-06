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
	got := (&GitLabProvider{}).FetchPRsContext(context.Background(), t.TempDir(), []string{"feature"})["feature"]
	if got == nil || got.Number != 31 || got.URL == "" || got.State != "open" || !got.Draft || got.CheckStatus != "failure" || got.ReviewStatus != "" {
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
	got := (&GitLabProvider{}).FetchPRsContext(context.Background(), t.TempDir(), []string{"feature"})["feature"]
	if got == nil || got.Number != 31 {
		t.Fatalf("selected MR = %+v", got)
	}
	if args := cliArgs(t, argsPath); !reflect.DeepEqual(args, []string{"mr", "view", "31", "--output", "json"}) {
		t.Fatalf("view args = %q", args)
	}
	t.Setenv("BIOME_CLI_DETAIL_RESPONSE", `{"iid":40,"state":"merged","web_url":"https://gitlab.com/a/b/-/merge_requests/40","source_branch":"feature"}`)
	got = (&GitLabProvider{}).FetchPRsContext(context.Background(), t.TempDir(), []string{"feature"})["feature"]
	if got != nil {
		t.Fatalf("wrong detail accepted: %+v", got)
	}
}
