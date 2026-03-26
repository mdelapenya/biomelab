package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// RepoEntry represents a registered repository.
type RepoEntry struct {
	Path string `json:"path"` // absolute path to repo root
	Name string `json:"name"` // display name (e.g., "owner/repo")
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
	return &cfg, nil
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

// Add adds a repo entry if not already present (dedup by path).
// Returns true if the entry was added.
func (c *Config) Add(path, name string) bool {
	for _, r := range c.Repos {
		if r.Path == path {
			return false
		}
	}
	c.Repos = append(c.Repos, RepoEntry{Path: path, Name: name})
	return true
}

// Remove removes a repo entry by path.
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
