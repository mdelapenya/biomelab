# Architecture

This document describes the internal architecture of biomelab for contributors
and AI coding agents. For user-facing features, see [README.md](README.md).
For the Fyne framework reference, invoke the `/fyne-developer` skill.

## Package layout

```
cmd/biomelab/
  main.go              Entry point: flags, PATH expansion, icon embedding, auto-add repo
  common.go            Shared: version var, resolveRefreshInterval
  icon.png             App icon (embedded via //go:embed)

internal/
  gui/
    app.go             FyneApp: window, HSplit layout, multi-repo management, mode switching
    dashboard.go       Right panel: main card + scrollable linked cards grid, refresh timestamps
    card.go            Worktree card rendering: branch, path, PR, agents, IDEs, status
    repo_panel.go      Left panel: tappable VBox of repo/mode items (NOT widget.Tree)
    shortcuts.go       All keyboard handling: handleKeyName + handleRune, navigation, operations
    keycapture.go      desktop.Canvas.SetOnKeyDown setup, zoom shortcuts
    dialogs.go         Confirmation dialogs (delete, sandbox create/remove, send PR flow)
    input_dialogs.go   Input dialogs (branch, PR ref, repo path, agent select)
    refresh.go         RefreshManager: goroutine tickers for local (5s) and network refresh
    state.go           RepoState: domain + UI state, worktree sorting
    theme.go           Dark theme with zoom support (Ctrl+/Ctrl-)
    icon.go            AppIcon resource (set from embedded icon at startup)
    systray.go         System tray: Show/Hide toggle, Quit

  ops/
    refresh.go         QuickRefresh, LocalRefresh, NetworkRefresh, CardRefresh
    worktree.go        CreateWorktree, RemoveWorktree, FetchPR, Pull, SendPR, OpenEditor, OpenTerminal
    sandbox_ops.go     CreateSandbox, StartSandbox, StopSandbox, RemoveSandbox

  config/config.go     Repo list persistence (~/.config/biomelab/repos.json)
  git/worktree.go      Go-git v6 wrapper: list, create, remove, pull, fetch, sync status
  git/credential.go    Git credential helper protocol (git credential fill)
  agent/               Agent kind registry + process detection
  ide/                 IDE kind registry + process detection
  process/process.go   Shared process enumeration types (Lister, Info, OSLister)
  provider/            PRProvider interface, GitHub (gh), GitLab (glab), detection
  sandbox/sandbox.go   Docker Sandbox (sbx) CLI wrapper
  terminal/terminal.go Open new terminal window (macOS .command / Linux x-terminal-emulator)
  github/pr.go         GitHub-specific PR helpers (ParsePRRef, ValidatePR)
```

## Key dependencies

- **Fyne v2.7** -- Desktop GUI framework (requires CGo).
- **go-git v6** (unreleased, from main branch) -- All git operations. Uses `x/plumbing/worktree` for linked worktree support.
- **gopsutil** -- Cross-platform process detection for agent and IDE matching.
- **gh CLI** -- External tool for GitHub PR status (not a Go dependency).
- **glab CLI** -- External tool for GitLab MR status.

## Data flow

1. On startup, `main.go` expands PATH (for GUI launch from Spotlight), embeds the icon, loads config, and auto-adds the current repo.
2. `gui.App.Run()` creates the Fyne window, builds the content (repo panel + dashboard), registers keyboard handlers via `desktop.Canvas.SetOnKeyDown`, sets up the system tray, and starts the event loop.
3. Each repo's `RefreshManager` runs goroutine tickers: local refresh (5s) for dirty/agents/IDE/sandbox status, network refresh (configurable, default 30s) for git fetch + PR lookup.
4. Refresh results are delivered via `fyne.Do(func() { dashboard.ApplyRefresh(result) })` to ensure all UI mutations happen on the main thread.
5. `Dashboard.Rebuild()` recreates the card widgets from current `RepoState`. Cards are sorted alphabetically by branch name.
6. The repo panel uses tappable VBox items (not `widget.Tree`) to avoid stealing keyboard focus.

## Keyboard handling

Fyne's keyboard event delivery has several constraints:
- `Canvas.SetOnTypedKey/Rune` only fires when `canvas.Focused() == nil`
- `Canvas.AddShortcut` with `Modifier: 0` doesn't dispatch (Fyne requires modifier != 0)
- `widget.Tree` implements Focusable and steals focus on click
- `fyne.KeyEvent` carries no modifier info (can't distinguish 's' from 'S')

The solution:
- **No Focusable widgets** in the content tree (repo panel uses tappable labels, not widget.Tree)
- **`desktop.Canvas.SetOnKeyDown`** handles all keys (fires before Tab interception)
- **`Canvas.SetOnTypedRune`** handles only Shift+S and Shift+P (case-sensitive)
- **Zoom shortcuts** use `Canvas.AddShortcut` with Ctrl/Cmd modifier (which works)
- **Dialog Escape** calls `dialog.Hide()` (never `overlays.Remove` which corrupts state)

## Async pattern

All blocking operations (git fetch, sandbox status, PR lookup) run in goroutines.
Results are delivered to the UI via `fyne.Do(func() { ... })`. The `RefreshManager`
uses `context.Context` for pause/resume when switching repos.

## macOS GUI considerations

When launched from Spotlight/Finder (not terminal), the process gets a minimal
PATH (`/usr/bin:/bin:/usr/sbin:/sbin`). The `init()` function in `main.go`
expands PATH to include `/usr/local/bin`, `/opt/homebrew/bin`, `~/.docker/bin`,
etc. This is required for `sbx`, `gh`, `glab`, `code`, and other CLI tools.

## System tray

The app lives in the system tray. Closing the window hides it (not quit).
The tray menu toggles between "Show" and "Hide" based on window visibility.
"Quit" stops all refresh managers and exits.

## Config format

`~/.config/biomelab/repos.json`:

```go
type ModeEntry struct {
    Type        string // "regular" or "sandbox"
    SandboxName string
    Agent       string
}
type RepoEntry struct {
    Path  string
    Name  string
    Modes []ModeEntry
}
```

The old flat format (with `Sandbox bool`) is auto-migrated on load.

## Pitfalls

- go-git v6 is a pseudo-version. Do NOT use a `replace` directive.
- `sandbox.StatusNotFound` is 0 (iota). Use `HasSbxStatus` flag, not `!= 0`.
- `canvas.Text` doesn't clip. Truncate strings manually.
- Dialog `onDone` callback must fire on BOTH confirm and cancel.
- `widget.Button` implements Focusable — don't put buttons in the main content.
- IDE `ProcessPatterns` order matters: specific before broad (`"nvim"` before `"vim"`).
- Always bounds-check `a.active < len(a.repos)` before accessing repos.
