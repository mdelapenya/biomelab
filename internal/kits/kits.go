// Package kits discovers Docker Sandbox kits published in
// docker/sbx-kits-contrib so they can be applied to a sandbox via
// `sbx create --kit "docker.io/sbx/<directory>-kit:latest"`.
package kits

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/mdelapenya/biomelab/internal/command"
)

// DefaultTag is the rolling Docker Hub tag published by the kit catalog.
const DefaultTag = "latest"

// Kind values from spec.yaml.
const (
	KindAgent = "agent"
	KindMixin = "mixin"
)

// nonKitDirs are top-level directories that are not kit definitions.
var nonKitDirs = map[string]struct{}{
	".github": {},
	"spec":    {},
	"tck":     {},
}

// Kit is a sandbox kit discovered in docker/sbx-kits-contrib.
// Fields mirror the relevant top-level keys in each kit's spec.yaml.
type Kit struct {
	Name        string `yaml:"name"`
	Kind        string `yaml:"kind"`
	DisplayName string `yaml:"displayName"`
	Description string `yaml:"description"`
	Extends     string `yaml:"extends"`
	Requires    struct {
		Agent string `yaml:"agent"`
	} `yaml:"requires"`
	// Directory determines the published artifact name, even when spec.name differs.
	Directory string `yaml:"-"`
}

// OCIReference returns this kit's published Docker Hub artifact reference.
func (k Kit) OCIReference() string {
	name := k.Directory
	if name == "" {
		name = k.Name
	}
	return "docker.io/sbx/" + name + "-kit:" + DefaultTag
}

// FetchAvailable lists every kit in docker/sbx-kits-contrib and splits them
// into (agents, mixins) by spec.yaml `kind`. Discovery uses `gh api`, which
// reuses the user's existing GitHub authentication (matching how
// internal/provider/github.go talks to GitHub).
//
// Directories without a spec.yaml or with an unknown kind are skipped.
func FetchAvailable(ctx context.Context) (agents, mixins []Kit, err error) {
	dirs, err := listKitDirs(ctx)
	if err != nil {
		return nil, nil, err
	}

	results := make([]Kit, len(dirs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	for i, dir := range dirs {
		idx, name := i, dir
		g.Go(func() error {
			k, ok, ferr := fetchKitSpec(gctx, name)
			if ferr != nil {
				return ferr
			}
			if ok {
				results[idx] = k
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, nil, err
	}

	for _, k := range results {
		switch k.Kind {
		case KindAgent:
			agents = append(agents, k)
		case KindMixin:
			mixins = append(mixins, k)
		}
	}
	sortByDisplay(agents)
	sortByDisplay(mixins)
	return agents, mixins, nil
}

// FilterMixinsForAgent returns the subset of mixins compatible with the given
// sandbox agent, honoring both current requires.agent and legacy extends.
func FilterMixinsForAgent(mixins []Kit, agent string) []Kit {
	out := make([]Kit, 0, len(mixins))
	for _, m := range mixins {
		if (m.Requires.Agent == "" || m.Requires.Agent == agent) &&
			(m.Extends == "" || m.Extends == agent) {
			out = append(out, m)
		}
	}
	return out
}

// listKitDirs returns the candidate top-level directory names in the
// kits-contrib repo (everything that's a dir and not in nonKitDirs).
func listKitDirs(ctx context.Context) ([]string, error) {
	cmd := command.BackgroundContext(ctx, "gh", "api", "repos/docker/sbx-kits-contrib/contents")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list kits: %w (%s)", err, ghStderr(err))
	}
	var entries []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parse kit listing: %w", err)
	}
	var dirs []string
	for _, e := range entries {
		if e.Type != "dir" {
			continue
		}
		if _, skip := nonKitDirs[e.Name]; skip {
			continue
		}
		dirs = append(dirs, e.Name)
	}
	return dirs, nil
}

// fetchKitSpec downloads and parses spec.yaml for a single kit directory.
// Returns ok=false (without error) if the directory has no spec.yaml.
func fetchKitSpec(ctx context.Context, dir string) (Kit, bool, error) {
	cmd := command.BackgroundContext(ctx, "gh", "api",
		"repos/docker/sbx-kits-contrib/contents/"+dir+"/spec.yaml")
	out, err := cmd.Output()
	if err != nil {
		stderr := ghStderr(err)
		if strings.Contains(stderr, "Not Found") || strings.Contains(stderr, "HTTP 404") {
			return Kit{}, false, nil
		}
		return Kit{}, false, fmt.Errorf("fetch %s/spec.yaml: %w (%s)", dir, err, stderr)
	}
	var resp struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return Kit{}, false, fmt.Errorf("parse %s/spec.yaml response: %w", dir, err)
	}
	raw, err := decodeContent(resp.Content, resp.Encoding)
	if err != nil {
		return Kit{}, false, fmt.Errorf("decode %s/spec.yaml: %w", dir, err)
	}
	k, err := ParseSpec(raw)
	if err != nil {
		return Kit{}, false, fmt.Errorf("parse %s/spec.yaml: %w", dir, err)
	}
	if k.Name == "" {
		k.Name = dir
	}
	k.Directory = dir
	return k, true, nil
}

// ParseSpec unmarshals a spec.yaml into a Kit.
func ParseSpec(raw []byte) (Kit, error) {
	var k Kit
	if err := yaml.Unmarshal(raw, &k); err != nil {
		return Kit{}, err
	}
	return k, nil
}

func decodeContent(content, encoding string) ([]byte, error) {
	if encoding != "base64" {
		return []byte(content), nil
	}
	// gh api returns base64 with newlines.
	cleaned := strings.ReplaceAll(content, "\n", "")
	return base64.StdEncoding.DecodeString(cleaned)
}

// ghStderr extracts the stderr of an *exec.ExitError, if any.
func ghStderr(err error) string {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return strings.TrimSpace(string(ee.Stderr))
	}
	return ""
}

func sortByDisplay(ks []Kit) {
	sort.Slice(ks, func(i, j int) bool {
		ai := ks[i].DisplayName
		if ai == "" {
			ai = ks[i].Name
		}
		aj := ks[j].DisplayName
		if aj == "" {
			aj = ks[j].Name
		}
		return strings.ToLower(ai) < strings.ToLower(aj)
	})
}
