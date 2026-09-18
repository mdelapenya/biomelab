package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// IssueRef identifies an issue, optionally in an explicit GitHub repository.
// Repo has the form "owner/repo"; an empty Repo uses the repository at repoDir.
type IssueRef struct {
	Number int
	Repo   string
}

// IssueInfo is the issue data needed to create a worktree from an issue.
type IssueInfo struct {
	Number int
	Title  string
	Body   string
	URL    string
	State  string
}

const issueLookupTimeout = 15 * time.Second

// ParseIssueRef parses either an issue number in the current repository or an
// explicit owner/repo#number reference.
func ParseIssueRef(input string) (IssueRef, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return IssueRef{}, errors.New("empty issue reference")
	}

	if strings.Contains(input, "#") {
		if strings.Count(input, "#") != 1 {
			return IssueRef{}, fmt.Errorf("invalid issue reference %q: expected owner/repo#number", input)
		}
		parts := strings.SplitN(input, "#", 2)
		if !validIssueRepo(parts[0]) {
			return IssueRef{}, fmt.Errorf("invalid repository %q: expected safe owner/repo", parts[0])
		}
		number, err := parseIssueNumber(parts[1])
		if err != nil {
			return IssueRef{}, fmt.Errorf("invalid issue number %q: %w", parts[1], err)
		}
		return IssueRef{Number: number, Repo: parts[0]}, nil
	}

	number, err := parseIssueNumber(input)
	if err != nil {
		return IssueRef{}, fmt.Errorf("invalid issue number %q: %w", input, err)
	}
	return IssueRef{Number: number}, nil
}

func parseIssueNumber(value string) (int, error) {
	if value == "" {
		return 0, errors.New("empty value")
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, errors.New("must be a decimal integer")
		}
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("overflows int: %w", err)
	}
	if number <= 0 {
		return 0, errors.New("must be positive")
	}
	return number, nil
}

func validIssueRepo(repo string) bool {
	parts := strings.Split(repo, "/")
	return len(parts) == 2 && validIssueOwner(parts[0]) && validIssueRepoName(parts[1])
}

func validIssueOwner(owner string) bool {
	if owner == "" || !isASCIIAlphaNumeric(owner[0]) {
		return false
	}
	for i := 1; i < len(owner); i++ {
		c := owner[i]
		if !isASCIIAlphaNumeric(c) && c != '-' && c != '_' && c != '.' {
			return false
		}
	}
	return true
}

func validIssueRepoName(repo string) bool {
	if repo == "" || repo == "." || repo == ".." {
		return false
	}
	for i := 0; i < len(repo); i++ {
		c := repo[i]
		if !isASCIIAlphaNumeric(c) && c != '-' && c != '_' && c != '.' {
			return false
		}
	}
	return true
}

func isASCIIAlphaNumeric(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

type issueRunner interface {
	run(context.Context, string, []string) ([]byte, []byte, error)
}

type execIssueRunner struct{}

func (execIssueRunner) run(ctx context.Context, dir string, args []string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	var stderr strings.Builder
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	return stdout, []byte(stderr.String()), err
}

// FetchIssue retrieves an issue using gh from repoDir. An explicit repository
// reference is passed to gh with --repo; otherwise gh resolves repoDir.
func FetchIssue(ctx context.Context, repoDir string, ref IssueRef) (IssueInfo, error) {
	return fetchIssue(ctx, repoDir, ref, execIssueRunner{})
}

func fetchIssue(ctx context.Context, repoDir string, ref IssueRef, runner issueRunner) (IssueInfo, error) {
	if ref.Number <= 0 {
		return IssueInfo{}, fmt.Errorf("invalid issue number %d: must be positive", ref.Number)
	}
	if ref.Repo != "" && !validIssueRepo(ref.Repo) {
		return IssueInfo{}, fmt.Errorf("invalid repository %q: expected safe owner/repo", ref.Repo)
	}
	if err := ctx.Err(); err != nil {
		return IssueInfo{}, fmt.Errorf("issue lookup cancelled: %w", err)
	}

	lookupCtx, cancel := context.WithTimeout(ctx, issueLookupTimeout)
	defer cancel()
	args := []string{"issue", "view", strconv.Itoa(ref.Number), "--json", "number,title,body,url,state"}
	if ref.Repo != "" {
		args = append(args, "--repo", ref.Repo)
	}
	stdout, stderr, err := runner.run(lookupCtx, repoDir, args)
	if err != nil {
		if ctx.Err() != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return IssueInfo{}, fmt.Errorf("issue lookup timed out: %w", ctx.Err())
			}
			return IssueInfo{}, fmt.Errorf("issue lookup cancelled: %w", ctx.Err())
		}
		if errors.Is(lookupCtx.Err(), context.DeadlineExceeded) {
			return IssueInfo{}, fmt.Errorf("issue lookup timed out after %s: %w", issueLookupTimeout, lookupCtx.Err())
		}
		if detail := strings.TrimSpace(string(stderr)); detail != "" {
			return IssueInfo{}, fmt.Errorf("gh issue view #%d failed: %w: %s", ref.Number, err, detail)
		}
		return IssueInfo{}, fmt.Errorf("gh issue view #%d failed: %w", ref.Number, err)
	}

	var info IssueInfo
	if err := json.Unmarshal(stdout, &info); err != nil {
		return IssueInfo{}, fmt.Errorf("invalid JSON from gh issue view #%d: %w", ref.Number, err)
	}
	if err := validateIssueInfo(info, ref); err != nil {
		return IssueInfo{}, err
	}
	return info, nil
}

func validateIssueInfo(info IssueInfo, ref IssueRef) error {
	if info.Number != ref.Number {
		return fmt.Errorf("gh returned issue #%d, expected #%d", info.Number, ref.Number)
	}
	if strings.TrimSpace(info.Title) == "" {
		return errors.New("gh returned an issue without a title")
	}
	if strings.TrimSpace(info.State) == "" {
		return errors.New("gh returned an issue without a state")
	}
	if err := validateIssueURL(info.URL, info.Number); err != nil {
		return err
	}
	return nil
}

func validateIssueURL(rawURL string, number int) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("gh returned invalid issue URL %q: %w", rawURL, err)
	}
	if u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("gh returned non-canonical GitHub issue URL %q", rawURL)
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 4 || !validIssueOwner(parts[0]) || !validIssueRepoName(parts[1]) || parts[2] != "issues" || parts[3] != strconv.Itoa(number) {
		return fmt.Errorf("gh returned non-canonical GitHub issue URL %q", rawURL)
	}
	return nil
}
