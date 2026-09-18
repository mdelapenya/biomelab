package github

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
)

type issueRunnerFunc func(context.Context, string, []string) ([]byte, []byte, error)

func (f issueRunnerFunc) run(ctx context.Context, dir string, args []string) ([]byte, []byte, error) {
	return f(ctx, dir, args)
}

func TestParseIssueRef(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  IssueRef
		bad   bool
	}{
		{name: "number", input: "42", want: IssueRef{Number: 42}},
		{name: "trims whitespace", input: "  42\t", want: IssueRef{Number: 42}},
		{name: "qualified", input: "octo-org/repo_name#7", want: IssueRef{Number: 7, Repo: "octo-org/repo_name"}},
		{name: "dot github repository", input: "octo/.github#7", want: IssueRef{Number: 7, Repo: "octo/.github"}},
		{name: "empty", input: "", bad: true},
		{name: "zero", input: "0", bad: true},
		{name: "negative", input: "-1", bad: true},
		{name: "decimal only", input: "1.0", bad: true},
		{name: "overflow", input: "99999999999999999999999999999999999999", bad: true},
		{name: "missing repo", input: "repo#1", bad: true},
		{name: "extra slash", input: "owner/repo/extra#1", bad: true},
		{name: "unsafe repo", input: "owner/repo;rm#1", bad: true},
		{name: "empty component", input: "owner/#1", bad: true},
		{name: "multiple separators", input: "owner/repo#1#2", bad: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseIssueRef(tt.input)
			if tt.bad {
				if err == nil {
					t.Fatal("ParseIssueRef returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseIssueRef: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseIssueRef(%q) = %#v, want %#v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFetchIssuePassesStructuredArgumentsAndDirectory(t *testing.T) {
	var gotDir string
	var gotArgs []string
	runner := issueRunnerFunc(func(_ context.Context, dir string, args []string) ([]byte, []byte, error) {
		gotDir = dir
		gotArgs = append([]string(nil), args...)
		return []byte(`{"number":42,"title":"Fix it","body":"","url":"https://github.com/octo/repo/issues/42","state":"CLOSED"}`), nil, nil
	})

	info, err := fetchIssue(context.Background(), "/tmp/local-repo", IssueRef{Number: 42, Repo: "octo/repo"}, runner)
	if err != nil {
		t.Fatalf("fetchIssue: %v", err)
	}
	if gotDir != "/tmp/local-repo" {
		t.Errorf("directory = %q", gotDir)
	}
	wantArgs := []string{"issue", "view", "42", "--json", "number,title,body,url,state", "--repo", "octo/repo"}
	if !reflect.DeepEqual(gotArgs, wantArgs) {
		t.Errorf("arguments = %#v, want %#v", gotArgs, wantArgs)
	}
	if info != (IssueInfo{Number: 42, Title: "Fix it", URL: "https://github.com/octo/repo/issues/42", State: "CLOSED"}) {
		t.Errorf("info = %#v", info)
	}
}

func TestFetchIssueUsesCurrentRepositoryWithoutRepoFlag(t *testing.T) {
	runner := issueRunnerFunc(func(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
		for _, arg := range args {
			if arg == "--repo" {
				t.Fatal("unexpected --repo argument")
			}
		}
		return []byte(`{"number":3,"title":"Title","body":"body","url":"https://github.com/owner/repo/issues/3","state":"OPEN"}`), nil, nil
	})
	if _, err := fetchIssue(context.Background(), "/tmp/repo", IssueRef{Number: 3}, runner); err != nil {
		t.Fatalf("fetchIssue: %v", err)
	}
}

func TestFetchIssuePreservesCLIDiagnostics(t *testing.T) {
	runner := issueRunnerFunc(func(_ context.Context, _ string, _ []string) ([]byte, []byte, error) {
		return nil, []byte("authentication required\n"), errors.New("exit status 1")
	})
	_, err := fetchIssue(context.Background(), "/tmp/repo", IssueRef{Number: 3}, runner)
	if err == nil || !strings.Contains(err.Error(), "authentication required") || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("error = %v, want CLI diagnostics", err)
	}
}

func TestFetchIssuePreservesMissingCLIError(t *testing.T) {
	runner := issueRunnerFunc(func(_ context.Context, _ string, _ []string) ([]byte, []byte, error) {
		return nil, nil, exec.ErrNotFound
	})
	_, err := fetchIssue(context.Background(), "/tmp/repo", IssueRef{Number: 3}, runner)
	if !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("error = %v, want wrapped exec.ErrNotFound", err)
	}
}

func TestFetchIssueCancellationBeforeRunner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	runner := issueRunnerFunc(func(_ context.Context, _ string, _ []string) ([]byte, []byte, error) {
		called = true
		return nil, nil, nil
	})
	_, err := fetchIssue(ctx, "/tmp/repo", IssueRef{Number: 3}, runner)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("runner was called after cancellation")
	}
}

func TestFetchIssueCancellationAfterRunnerStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	runner := issueRunnerFunc(func(ctx context.Context, _ string, _ []string) ([]byte, []byte, error) {
		close(started)
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	result := make(chan error, 1)
	go func() {
		_, err := fetchIssue(ctx, "/tmp/repo", IssueRef{Number: 3}, runner)
		result <- err
	}()
	<-started
	cancel()
	err := <-result
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("error = %v, want cancellation diagnostic", err)
	}
}

func TestFetchIssueParentDeadlineIsTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	runner := issueRunnerFunc(func(ctx context.Context, _ string, _ []string) ([]byte, []byte, error) {
		<-ctx.Done()
		return nil, nil, ctx.Err()
	})
	_, err := fetchIssue(ctx, "/tmp/repo", IssueRef{Number: 3}, runner)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want timeout diagnostic", err)
	}
}

func TestFetchIssueRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{name: "malformed JSON", json: "{"},
		{name: "wrong number", json: `{"number":4,"title":"Title","url":"https://github.com/owner/repo/issues/4","state":"OPEN"}`},
		{name: "missing title", json: `{"number":3,"url":"https://github.com/owner/repo/issues/3","state":"OPEN"}`},
		{name: "missing state", json: `{"number":3,"title":"Title","url":"https://github.com/owner/repo/issues/3"}`},
		{name: "missing URL", json: `{"number":3,"title":"Title","state":"OPEN"}`},
		{name: "wrong URL number", json: `{"number":3,"title":"Title","url":"https://github.com/owner/repo/issues/4","state":"OPEN"}`},
		{name: "non GitHub URL", json: `{"number":3,"title":"Title","url":"https://example.com/owner/repo/issues/3","state":"OPEN"}`},
		{name: "URL query", json: `{"number":3,"title":"Title","url":"https://github.com/owner/repo/issues/3?x=1","state":"OPEN"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := issueRunnerFunc(func(_ context.Context, _ string, _ []string) ([]byte, []byte, error) {
				return []byte(tt.json), nil, nil
			})
			ref := IssueRef{Number: 3}
			if _, err := fetchIssue(context.Background(), "/tmp/repo", ref, runner); err == nil {
				t.Fatal("fetchIssue returned nil error")
			}
		})
	}
}
