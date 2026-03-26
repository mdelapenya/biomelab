package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPath(t *testing.T) {
	p := DefaultPath()
	if p == "" {
		t.Fatal("DefaultPath() returned empty string")
	}
	if filepath.Base(p) != "repos.json" {
		t.Errorf("DefaultPath() = %q, want basename repos.json", p)
	}
}

func TestLoadNonExistent(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load non-existent: %v", err)
	}
	if len(cfg.Repos) != 0 {
		t.Errorf("expected empty repos, got %d", len(cfg.Repos))
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "repos.json")
	cfg := &Config{
		Repos: []RepoEntry{
			{Path: "/tmp/repo1", Name: "owner/repo1"},
			{Path: "/tmp/repo2", Name: "owner/repo2"},
		},
	}

	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(loaded.Repos))
	}
	if loaded.Repos[0].Path != "/tmp/repo1" {
		t.Errorf("Repos[0].Path = %q, want /tmp/repo1", loaded.Repos[0].Path)
	}
	if loaded.Repos[1].Name != "owner/repo2" {
		t.Errorf("Repos[1].Name = %q, want owner/repo2", loaded.Repos[1].Name)
	}
}

func TestSaveAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repos.json")

	cfg := &Config{Repos: []RepoEntry{{Path: "/a", Name: "a"}}}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Ensure no .tmp file remains.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("expected .tmp file to be cleaned up after atomic rename")
	}
}

func TestAddDeduplicate(t *testing.T) {
	cfg := &Config{}

	added := cfg.Add("/tmp/repo1", "repo1")
	if !added {
		t.Error("first Add should return true")
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(cfg.Repos))
	}

	added = cfg.Add("/tmp/repo1", "repo1-different-name")
	if added {
		t.Error("duplicate Add should return false")
	}
	if len(cfg.Repos) != 1 {
		t.Errorf("expected 1 repo after dedup, got %d", len(cfg.Repos))
	}
}

func TestAddMultiple(t *testing.T) {
	cfg := &Config{}
	cfg.Add("/tmp/repo1", "repo1")
	cfg.Add("/tmp/repo2", "repo2")
	cfg.Add("/tmp/repo3", "repo3")

	if len(cfg.Repos) != 3 {
		t.Errorf("expected 3 repos, got %d", len(cfg.Repos))
	}
}

func TestRemove(t *testing.T) {
	cfg := &Config{
		Repos: []RepoEntry{
			{Path: "/tmp/repo1", Name: "repo1"},
			{Path: "/tmp/repo2", Name: "repo2"},
		},
	}

	removed := cfg.Remove("/tmp/repo1")
	if !removed {
		t.Error("Remove should return true for existing entry")
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("expected 1 repo after removal, got %d", len(cfg.Repos))
	}
	if cfg.Repos[0].Path != "/tmp/repo2" {
		t.Errorf("remaining repo = %q, want /tmp/repo2", cfg.Repos[0].Path)
	}
}

func TestRemoveNonExistent(t *testing.T) {
	cfg := &Config{
		Repos: []RepoEntry{{Path: "/tmp/repo1", Name: "repo1"}},
	}
	removed := cfg.Remove("/tmp/nope")
	if removed {
		t.Error("Remove should return false for non-existent path")
	}
	if len(cfg.Repos) != 1 {
		t.Errorf("repos should be unchanged, got %d", len(cfg.Repos))
	}
}

func TestIndexOf(t *testing.T) {
	cfg := &Config{
		Repos: []RepoEntry{
			{Path: "/tmp/repo1", Name: "repo1"},
			{Path: "/tmp/repo2", Name: "repo2"},
		},
	}

	if idx := cfg.IndexOf("/tmp/repo1"); idx != 0 {
		t.Errorf("IndexOf(/tmp/repo1) = %d, want 0", idx)
	}
	if idx := cfg.IndexOf("/tmp/repo2"); idx != 1 {
		t.Errorf("IndexOf(/tmp/repo2) = %d, want 1", idx)
	}
	if idx := cfg.IndexOf("/tmp/nope"); idx != -1 {
		t.Errorf("IndexOf(/tmp/nope) = %d, want -1", idx)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
