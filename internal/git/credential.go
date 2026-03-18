package git

import (
	"bufio"
	"fmt"
	"net/url"
	"os/exec"
	"strings"

	githttp "github.com/go-git/go-git/v6/plumbing/transport/http"
)

// credentialFill invokes `git credential fill` to obtain credentials
// from the user's configured credential helpers (osxkeychain, gh auth, etc.).
func credentialFill(remoteURL string) (*githttp.BasicAuth, error) {
	u, err := url.Parse(remoteURL)
	if err != nil {
		return nil, fmt.Errorf("parse remote URL: %w", err)
	}

	// Build the credential input per git-credential protocol.
	input := fmt.Sprintf("protocol=%s\nhost=%s\npath=%s\n\n",
		u.Scheme, u.Host, strings.TrimPrefix(u.Path, "/"))

	cmd := exec.Command("git", "credential", "fill")
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
