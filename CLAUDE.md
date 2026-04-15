# CLAUDE.md -- Project context for AI agents

biomelab is a Go TUI that manages git worktrees for AI coding agents. Multi-repo
dashboard with sandbox (Docker) and regular modes.

For product features and decision rationale, invoke the `/product-owner` skill.
For internal architecture, design decisions, and state machines, see [ARCHITECTURE.md](ARCHITECTURE.md).

## Build and test commands

```
task build        # Build to bin/biomelab
task install      # Install to $GOPATH/bin
task test         # Run tests
task test-race    # Run tests with -race
task lint         # Run golangci-lint
task clean        # Remove build artifacts
```

Go version: 1.25+ (check `go env GOROOT` if you hit version mismatches).

## Key dependencies

- **go-git v6** (unreleased, from main branch) -- All git operations.
- **gopsutil** -- Cross-platform process detection.
- **bubbletea + lipgloss + bubbles** -- TUI framework.
- **gh CLI / glab CLI** -- External tools for PR/MR status.

## Testing patterns

- **Config tests**: `t.TempDir()` for config paths. Round-trip save/load, dedup, remove.
- **Git tests**: Real temp repos via `t.TempDir()` + `gogit.PlainInit`.
- **Agent/IDE tests**: Mock `process.Lister` interface with canned `process.Info` slices.
- **App tests**: `testApp(n)` with pre-populated repoGroups (no real repos). Focus, navigation, stale routing.
- **TUI tests**: `testModel(n)` with pre-populated worktrees. Send `tea.KeyMsg`, assert fields.
- **Card tests**: `card.Render(wt, agents, ides, pr, cliAvail, prov)` and assert output strings.

Always run `go test -race ./...` -- the TUI must be safe for concurrent `View` + `Update`.

## Pitfalls

- `m.worktrees[1:]` panics if worktrees is empty. Always guard with `len(m.worktrees) > 1`.
- `textinput.New()` must be called before the model is used in tests, or `Focus()` will nil-panic.
- go-git v6 is a pseudo-version. Do NOT use a `replace` directive -- that blocks `go install ...@latest`.
- **Do NOT use lipgloss `Border()`** for multi-panel layouts. Use manual borders via `buildPanels()`.
- **`tea.WindowSizeMsg` must not be used for child resizing** -- use `childResizeMsg` instead.
- **`App.Init()` is async**: `WindowSizeMsg` arrives before `appInitMsg`, so the init handler must resize children.
- Always bounds-check `a.active < len(a.repos)` before accessing `a.repos[a.active]`.
- **Confirmation dialogs must use popup overlays** (`overlayCenter()`), not viewport-bottom messages.
- **IDE `ProcessPatterns` order matters**: specific before broad (`"nvim"` before `"vim"`).
- **IDE cmdline matching uses longest-match**: most specific worktree path wins.
- **Help bar**: three locations (left footer, main card contextual, bottom bar). Do NOT mix actions across them.
