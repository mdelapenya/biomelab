package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// ModeEntry describes how a repo is managed: regular (host worktrees) or
// sandbox (Docker Sandbox via sbx CLI).
type ModeEntry struct {
	Type        string `json:"type"`                   // "regular" or "sandbox"
	SandboxName string `json:"sandbox_name,omitempty"` // sbx sandbox name (sandbox only)
	Agent       string `json:"agent,omitempty"`        // agent name (sandbox only, e.g. "claude")
}

// RepoEntry represents a registered repository with one or more modes.
type RepoEntry struct {
	Path  string      `json:"path"`  // absolute path to repo root
	Name  string      `json:"name"`  // display name (e.g., "owner/repo")
	Modes []ModeEntry `json:"modes"` // at least one mode (regular or sandbox)

	// Legacy fields for backward-compatible deserialization.
	// Zeroed after migration; omitempty ensures they don't get written back.
	Sandbox        bool   `json:"sandbox,omitempty"`
	OldSandboxName string `json:"sandbox_name,omitempty"`
	OldAgent       string `json:"agent,omitempty"`
}

// Config holds the list of registered repositories.
type Config struct {
	Repos []RepoEntry `json:"repos"`
}

// DefaultPath returns the default config file path (~/.config/gwaim/repos.json).
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "gwaim", "repos.json")
}

// Load reads the config from the given path.
// Returns an empty Config (not an error) if the file does not exist.
// Migrates old-format entries (no Modes field) to the new format.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.migrate()
	return &cfg, nil
}

// migrate converts old-format entries (Sandbox bool, no Modes) to the new
// format. Entries with the same path are merged into a single entry with
// multiple modes.
func (c *Config) migrate() {
	merged := make(map[string]*RepoEntry)
	var order []string

	for i := range c.Repos {
		r := &c.Repos[i]

		// Convert old format to modes if needed.
		if len(r.Modes) == 0 {
			if r.Sandbox {
				r.Modes = []ModeEntry{{
					Type:        "sandbox",
					SandboxName: r.OldSandboxName,
					Agent:       r.OldAgent,
				}}
			} else {
				r.Modes = []ModeEntry{{Type: "regular"}}
			}
		}
		// Clear legacy fields.
		r.Sandbox = false
		r.OldSandboxName = ""
		r.OldAgent = ""

		// Merge entries with the same path.
		if existing, ok := merged[r.Path]; ok {
			for _, m := range r.Modes {
				if !hasMode(existing.Modes, m) {
					existing.Modes = append(existing.Modes, m)
				}
			}
		} else {
			merged[r.Path] = r
			order = append(order, r.Path)
		}
	}

	// Rebuild repos in original order.
	c.Repos = make([]RepoEntry, 0, len(order))
	for _, p := range order {
		c.Repos = append(c.Repos, *merged[p])
	}
}

// Save writes the config to the given path atomically (write to tmp, rename).
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Add adds a mode to a repo. If the repo doesn't exist, it's created.
// If adding a sandbox mode to a repo that has a regular mode, the regular
// mode is replaced. Returns true if the config was changed.
func (c *Config) Add(path, name string, mode ModeEntry) bool {
	for i, r := range c.Repos {
		if r.Path == path {
			if hasMode(r.Modes, mode) {
				return false // already has this exact mode
			}
			// Adding sandbox replaces regular mode.
			if mode.Type == "sandbox" {
				c.Repos[i].Modes = removeRegularModes(r.Modes)
			}
			c.Repos[i].Modes = append(c.Repos[i].Modes, mode)
			return true
		}
	}
	c.Repos = append(c.Repos, RepoEntry{Path: path, Name: name, Modes: []ModeEntry{mode}})
	return true
}

// RemoveMode removes a specific mode from a repo.
// If the repo has no modes left, the entire entry is removed.
// Returns true if the config was changed.
func (c *Config) RemoveMode(path string, mode ModeEntry) bool {
	for i, r := range c.Repos {
		if r.Path == path {
			newModes := make([]ModeEntry, 0, len(r.Modes))
			for _, m := range r.Modes {
				if !modesEqual(m, mode) {
					newModes = append(newModes, m)
				}
			}
			if len(newModes) == len(r.Modes) {
				return false // mode not found
			}
			if len(newModes) == 0 {
				c.Repos = append(c.Repos[:i], c.Repos[i+1:]...)
			} else {
				c.Repos[i].Modes = newModes
			}
			return true
		}
	}
	return false
}

// Remove removes a repo entry by path (all modes).
// Returns true if the entry was found and removed.
func (c *Config) Remove(path string) bool {
	for i, r := range c.Repos {
		if r.Path == path {
			c.Repos = append(c.Repos[:i], c.Repos[i+1:]...)
			return true
		}
	}
	return false
}

// IndexOf returns the index of the repo with the given path, or -1 if not found.
func (c *Config) IndexOf(path string) int {
	for i, r := range c.Repos {
		if r.Path == path {
			return i
		}
	}
	return -1
}

// hasMode returns true if modes contains an entry matching m.
func hasMode(modes []ModeEntry, m ModeEntry) bool {
	for _, existing := range modes {
		if modesEqual(existing, m) {
			return true
		}
	}
	return false
}

// modesEqual returns true if two mode entries are equivalent.
func modesEqual(a, b ModeEntry) bool {
	if a.Type != b.Type {
		return false
	}
	if a.Type == "sandbox" {
		return a.SandboxName == b.SandboxName
	}
	return true // both regular
}

// removeRegularModes returns a new slice with all regular modes removed.
func removeRegularModes(modes []ModeEntry) []ModeEntry {
	result := make([]ModeEntry, 0, len(modes))
	for _, m := range modes {
		if m.Type != "regular" {
			result = append(result, m)
		}
	}
	return result
}
