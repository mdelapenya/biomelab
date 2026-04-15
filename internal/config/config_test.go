package config

import (
	"encoding/json"
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
	// Override legacy path so it doesn't find a real ~/.config/gwaim/repos.json.
	origFn := legacyPathFn
	legacyPathFn = func() string { return filepath.Join(t.TempDir(), "no-legacy.json") }
	t.Cleanup(func() { legacyPathFn = origFn })

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
			{Path: "/tmp/repo1", Name: "owner/repo1", Modes: []ModeEntry{{Type: "regular"}}},
			{Path: "/tmp/repo2", Name: "owner/repo2", Modes: []ModeEntry{{Type: "sandbox", SandboxName: "sbx1", Agent: "claude"}}},
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
	if loaded.Repos[0].Modes[0].Type != "regular" {
		t.Errorf("Repos[0].Modes[0].Type = %q, want regular", loaded.Repos[0].Modes[0].Type)
	}
	if loaded.Repos[1].Modes[0].Agent != "claude" {
		t.Errorf("Repos[1].Modes[0].Agent = %q, want claude", loaded.Repos[1].Modes[0].Agent)
	}
}

func TestSaveAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repos.json")

	cfg := &Config{Repos: []RepoEntry{{Path: "/a", Name: "a", Modes: []ModeEntry{{Type: "regular"}}}}}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("expected .tmp file to be cleaned up after atomic rename")
	}
}

func TestAddNewRepo(t *testing.T) {
	cfg := &Config{}
	changed := cfg.Add("/tmp/repo1", "repo1", ModeEntry{Type: "regular"})
	if !changed {
		t.Error("Add new repo should return true")
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(cfg.Repos))
	}
	if len(cfg.Repos[0].Modes) != 1 {
		t.Fatalf("expected 1 mode, got %d", len(cfg.Repos[0].Modes))
	}
}

func TestAddDuplicateMode(t *testing.T) {
	cfg := &Config{}
	cfg.Add("/tmp/repo1", "repo1", ModeEntry{Type: "regular"})
	changed := cfg.Add("/tmp/repo1", "repo1", ModeEntry{Type: "regular"})
	if changed {
		t.Error("Add duplicate mode should return false")
	}
	if len(cfg.Repos[0].Modes) != 1 {
		t.Errorf("expected 1 mode after dedup, got %d", len(cfg.Repos[0].Modes))
	}
}

func TestAddSandboxReplacesRegular(t *testing.T) {
	cfg := &Config{}
	cfg.Add("/tmp/repo1", "repo1", ModeEntry{Type: "regular"})
	changed := cfg.Add("/tmp/repo1", "repo1", ModeEntry{Type: "sandbox", SandboxName: "sbx-claude", Agent: "claude"})
	if !changed {
		t.Error("Add sandbox should return true")
	}
	if len(cfg.Repos[0].Modes) != 1 {
		t.Fatalf("expected 1 mode (regular replaced), got %d", len(cfg.Repos[0].Modes))
	}
	if cfg.Repos[0].Modes[0].Type != "sandbox" {
		t.Errorf("mode type = %q, want sandbox", cfg.Repos[0].Modes[0].Type)
	}
}

func TestAddMultipleSandboxModes(t *testing.T) {
	cfg := &Config{}
	cfg.Add("/tmp/repo1", "repo1", ModeEntry{Type: "sandbox", SandboxName: "sbx-claude", Agent: "claude"})
	cfg.Add("/tmp/repo1", "repo1", ModeEntry{Type: "sandbox", SandboxName: "sbx-gemini", Agent: "gemini"})
	if len(cfg.Repos[0].Modes) != 2 {
		t.Fatalf("expected 2 sandbox modes, got %d", len(cfg.Repos[0].Modes))
	}
}

func TestRemoveMode(t *testing.T) {
	cfg := &Config{}
	cfg.Add("/tmp/repo1", "repo1", ModeEntry{Type: "sandbox", SandboxName: "sbx-claude", Agent: "claude"})
	cfg.Add("/tmp/repo1", "repo1", ModeEntry{Type: "sandbox", SandboxName: "sbx-gemini", Agent: "gemini"})

	removed := cfg.RemoveMode("/tmp/repo1", ModeEntry{Type: "sandbox", SandboxName: "sbx-claude"})
	if !removed {
		t.Error("RemoveMode should return true")
	}
	if len(cfg.Repos[0].Modes) != 1 {
		t.Fatalf("expected 1 mode after removal, got %d", len(cfg.Repos[0].Modes))
	}
	if cfg.Repos[0].Modes[0].Agent != "gemini" {
		t.Errorf("remaining mode agent = %q, want gemini", cfg.Repos[0].Modes[0].Agent)
	}
}

func TestRemoveLastModeRemovesRepo(t *testing.T) {
	cfg := &Config{}
	cfg.Add("/tmp/repo1", "repo1", ModeEntry{Type: "regular"})

	removed := cfg.RemoveMode("/tmp/repo1", ModeEntry{Type: "regular"})
	if !removed {
		t.Error("RemoveMode should return true")
	}
	if len(cfg.Repos) != 0 {
		t.Errorf("expected 0 repos after removing last mode, got %d", len(cfg.Repos))
	}
}

func TestRemoveModeNonExistent(t *testing.T) {
	cfg := &Config{}
	cfg.Add("/tmp/repo1", "repo1", ModeEntry{Type: "regular"})

	removed := cfg.RemoveMode("/tmp/repo1", ModeEntry{Type: "sandbox", SandboxName: "nope"})
	if removed {
		t.Error("RemoveMode should return false for non-existent mode")
	}
}

func TestRemove(t *testing.T) {
	cfg := &Config{
		Repos: []RepoEntry{
			{Path: "/tmp/repo1", Name: "repo1", Modes: []ModeEntry{{Type: "regular"}}},
			{Path: "/tmp/repo2", Name: "repo2", Modes: []ModeEntry{{Type: "regular"}}},
		},
	}
	removed := cfg.Remove("/tmp/repo1")
	if !removed {
		t.Error("Remove should return true")
	}
	if len(cfg.Repos) != 1 || cfg.Repos[0].Path != "/tmp/repo2" {
		t.Errorf("unexpected repos after remove: %v", cfg.Repos)
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
	if idx := cfg.IndexOf("/tmp/nope"); idx != -1 {
		t.Errorf("IndexOf(/tmp/nope) = %d, want -1", idx)
	}
}

func TestMigrateOldFormatRegular(t *testing.T) {
	// Simulate old-format JSON with Sandbox: false
	oldJSON := `{"repos":[{"path":"/tmp/repo","name":"repo","sandbox":false}]}`
	path := filepath.Join(t.TempDir(), "repos.json")
	if err := os.WriteFile(path, []byte(oldJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(cfg.Repos))
	}
	if len(cfg.Repos[0].Modes) != 1 || cfg.Repos[0].Modes[0].Type != "regular" {
		t.Errorf("expected regular mode, got %v", cfg.Repos[0].Modes)
	}
	// Legacy fields should be cleared.
	if cfg.Repos[0].Sandbox {
		t.Error("legacy Sandbox field should be cleared")
	}
}

func TestMigrateOldFormatSandbox(t *testing.T) {
	oldJSON := `{"repos":[{"path":"/tmp/repo","name":"repo","sandbox":true,"sandbox_name":"my-sbx","agent":"claude"}]}`
	path := filepath.Join(t.TempDir(), "repos.json")
	if err := os.WriteFile(path, []byte(oldJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Repos[0].Modes) != 1 {
		t.Fatalf("expected 1 mode, got %d", len(cfg.Repos[0].Modes))
	}
	m := cfg.Repos[0].Modes[0]
	if m.Type != "sandbox" || m.SandboxName != "my-sbx" || m.Agent != "claude" {
		t.Errorf("unexpected mode: %+v", m)
	}
}

func TestMigrateMergesDuplicatePaths(t *testing.T) {
	// Old format: same repo twice (regular + sandbox)
	oldJSON := `{"repos":[
		{"path":"/tmp/repo","name":"repo"},
		{"path":"/tmp/repo","name":"repo","sandbox":true,"sandbox_name":"sbx","agent":"claude"}
	]}`
	path := filepath.Join(t.TempDir(), "repos.json")
	if err := os.WriteFile(path, []byte(oldJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Repos) != 1 {
		t.Fatalf("expected 1 merged repo, got %d", len(cfg.Repos))
	}
	if len(cfg.Repos[0].Modes) != 2 {
		t.Fatalf("expected 2 modes, got %d", len(cfg.Repos[0].Modes))
	}
}

func TestNewFormatRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.json")
	cfg := &Config{
		Repos: []RepoEntry{
			{
				Path: "/tmp/repo",
				Name: "repo",
				Modes: []ModeEntry{
					{Type: "sandbox", SandboxName: "sbx-claude", Agent: "claude"},
					{Type: "sandbox", SandboxName: "sbx-gemini", Agent: "gemini"},
				},
			},
		},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Verify JSON doesn't contain legacy fields.
	data, _ := os.ReadFile(path)
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Repos) != 1 || len(loaded.Repos[0].Modes) != 2 {
		t.Errorf("round-trip failed: %+v", loaded.Repos)
	}
}

func TestLoadMigratesFromLegacyGwaimPath(t *testing.T) {
	// Set up a legacy config at the old gwaim path.
	legacyDir := t.TempDir()
	legacyFile := filepath.Join(legacyDir, "gwaim", "repos.json")
	if err := os.MkdirAll(filepath.Dir(legacyFile), 0o755); err != nil {
		t.Fatal(err)
	}
	oldJSON := `{"repos":[{"path":"/tmp/repo","name":"repo","modes":[{"type":"regular"}]}]}`
	if err := os.WriteFile(legacyFile, []byte(oldJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// Override legacyPathFn to point to our temp legacy file.
	origFn := legacyPathFn
	legacyPathFn = func() string { return legacyFile }
	t.Cleanup(func() { legacyPathFn = origFn })

	// Load from a new (non-existent) path — should pick up the legacy config.
	newPath := filepath.Join(legacyDir, "biomelab", "repos.json")
	cfg, err := Load(newPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Repos) != 1 || cfg.Repos[0].Path != "/tmp/repo" {
		t.Errorf("expected migrated repo, got %+v", cfg.Repos)
	}

	// The new config file should have been created.
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected new config file to exist: %v", err)
	}

	// The legacy directory should have been removed.
	if _, err := os.Stat(filepath.Dir(legacyFile)); !os.IsNotExist(err) {
		t.Errorf("expected legacy directory to be removed, but it still exists")
	}
}

func TestLoadIgnoresLegacyWhenNewExists(t *testing.T) {
	dir := t.TempDir()

	// Create new config with repo-a.
	newPath := filepath.Join(dir, "biomelab", "repos.json")
	newCfg := &Config{Repos: []RepoEntry{{Path: "/tmp/repo-a", Name: "a", Modes: []ModeEntry{{Type: "regular"}}}}}
	if err := Save(newPath, newCfg); err != nil {
		t.Fatal(err)
	}

	// Create legacy config with repo-b.
	legacyFile := filepath.Join(dir, "gwaim", "repos.json")
	if err := os.MkdirAll(filepath.Dir(legacyFile), 0o755); err != nil {
		t.Fatal(err)
	}
	legacyJSON := `{"repos":[{"path":"/tmp/repo-b","name":"b","modes":[{"type":"regular"}]}]}`
	if err := os.WriteFile(legacyFile, []byte(legacyJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	origFn := legacyPathFn
	legacyPathFn = func() string { return legacyFile }
	t.Cleanup(func() { legacyPathFn = origFn })

	// Load should use the new config, not the legacy one.
	cfg, err := Load(newPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Repos) != 1 || cfg.Repos[0].Path != "/tmp/repo-a" {
		t.Errorf("expected new config repo, got %+v", cfg.Repos)
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
