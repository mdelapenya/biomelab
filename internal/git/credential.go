package git

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	githttp "github.com/go-git/go-git/v6/plumbing/transport/http"

	"github.com/mdelapenya/biomelab/internal/command"
)

// credentialFill asks configured helpers for cached credentials.
// Background GUI refreshes must not open a login dialog or terminal prompt.
func credentialFill(ctx context.Context, remoteURL string) (*githttp.BasicAuth, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	u, err := url.Parse(remoteURL)
	if err != nil {
		return nil, fmt.Errorf("parse remote URL: %w", err)
	}

	// Build the credential input per git-credential protocol.
	input := fmt.Sprintf("protocol=%s\nhost=%s\npath=%s\n\n",
		u.Scheme, u.Host, strings.TrimPrefix(u.Path, "/"))

	cmd := command.BackgroundContext(ctx, "git", "credential", "fill")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	cmd.Stdin = strings.NewReader(input)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git credential fill: %w", err)
	}

	return parseCredentialOutput(out)
}

func parseCredentialOutput(data []byte) (*githttp.BasicAuth, error) {
	auth := &githttp.BasicAuth{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if k, v, ok := strings.Cut(line, "="); ok {
			switch k {
			case "username":
				auth.Username = v
			case "password":
				auth.Password = v
			}
		}
	}
	if auth.Username == "" && auth.Password == "" {
		return nil, fmt.Errorf("no credentials returned")
	}
	return auth, nil
}
