# CLAUDE.md -- Project context for AI agents

This file provides context for Claude and other coding agents working on this codebase.

## What is gwaim?

gwaim (Git Worktree AI Manager) is a Go TUI that manages git worktrees in the context of coding agents. It provides a multi-repository dashboard with a two-column layout: a left panel listing registered repos and a right panel showing the selected repo's worktree cards. It detects which coding agents are running in each worktree and provides actions to create/delete/repair worktrees, pull, open editors, and open terminal tabs.

## Build and test commands

```
task build        # Build to bin/gwaim
task install      # Install to $GOPATH/bin
task test         # Run tests
task test-race    # Run tests with -race
task lint         # Run golangci-lint
task clean        # Remove build artifacts
```

Go version: 1.25+ (check `go env GOROOT` if you hit version mismatches).

## Project structure

```
cmd/gwaim/main.go           Entry point (creates App, auto-adds current repo)
internal/
  config/config.go          Repo list persistence (~/.config/gwaim/repos.json)
  git/worktree.go           Go-git v6 wrapper: list, create, remove, repair, pull, fetch, sync status
  git/credential.go         Git credential helper protocol (git credential fill)
  agent/agents.go           Agent kind registry (claude, kiro, copilot, codex, opencode, gemini)
  agent/detect.go           Process detection via gopsutil
  github/pr.go              GitHub-specific PR helpers (ParsePRRef, ValidatePR for fetch-PR flow)
  provider/provider.go      PRProvider interface, provider detection, shared types
  provider/github.go        GitHub PR lookup via gh CLI
  provider/gitlab.go        GitLab MR lookup via glab CLI
  provider/unsupported.go   Fallback for unsupported providers
  tui/app.go                App model: multi-repo wrapper, two-column layout, focus switching
  tui/model.go              Per-repo Bubbletea model: Init/Update/View, refresh, navigation, modes
  tui/keymap.go             Key bindings
  tui/styles.go             Lipgloss styles (cards, repo list, help)
  tui/messages.go           Custom tea.Msg types (all carry repoPath for stale detection)
  tui/card/card.go          Pure render function for worktree cards
  warp/warp.go              Terminal tab/panel management
docs/
  testing-tui-design.md     Testing strategy design document
```

## Key dependencies

- **go-git v6** (unreleased, from main branch) -- All git operations. Uses `x/plumbing/worktree` for linked worktree support.
- **gopsutil** -- Cross-platform process detection for agent matching.
- **bubbletea + lipgloss + bubbles** -- TUI framework.
- **gh CLI** -- External tool for PR status (not a Go dependency).

## Architecture decisions

**No shelling out to git** for git operations. The exceptions are `git credential fill` in `internal/git/credential.go` (credential helper protocol, because go-git has no built-in credential helper support), `git worktree repair` in `internal/git/worktree.go` (because go-git v6 has no repair API), and `git worktree add` in `FetchPR` (shells out to add the worktree with a sanitized directory name while preserving the original branch ref).

**FetchPR directory naming**: The directory basename is sanitized (slashes → dashes) so `.git/worktrees/<key>` is safe, but the local branch ref keeps its original name (e.g., `ralph/issue-19` stays `ralph/issue-19`, not `ralph-issue-19`). `FetchPR` returns `(wtPath string, err error)` so the TUI uses the actual path rather than re-deriving it.

**Fork PR fetches**: `FetchPR` sets `FetchOptions.RemoteURL` directly (not `RemoteName`) to pull from a fork's clone URL. `ParsePRRef` accepts `"123"` (current repo) or `"owner/repo#123"` (fork). `ValidatePR` shells out to `gh pr view` to confirm the PR exists and return its head branch.

**go-git v6 worktree limitations**: `linkedRepo.Head()` returns the shared (main) HEAD, not the per-worktree HEAD. We read `.git/worktrees/<name>/HEAD` directly from the filesystem for linked worktrees. Similarly, worktree paths come from `.git/worktrees/<name>/gitdir`.

**Sync status**: Computed by comparing `refs/heads/<branch>` against `refs/remotes/origin/<branch>` using `IsAncestor` checks. A `Fetch()` runs on every refresh cycle to keep remote refs current.

**Terminal tab management**: The `internal/warp` package tracks which repo tabs have been opened in the current session (in-memory map). First `Enter` creates a tab, subsequent presses split within it. macOS uses System Events AppleScript for keyboard shortcuts (Cmd+T, Cmd+Shift+D). Linux uses terminal-specific CLI flags.

**Mouse mode**: Off by default so users can select/copy text. Toggled with `m`. When on, click detection uses zone-based hit testing -- card bounding boxes are recorded during `renderBody` and checked in `handleClick`.

**Viewport scrolling**: Body content is rendered in `renderBody` (called from `syncViewport` in `Update`), not in `View`. This preserves scroll position across renders. `View` only reads from the viewport.

**Multi-repo architecture**: The `App` model (`tui/app.go`) wraps multiple `Model` instances in a two-column layout. Left panel (~15%) shows registered repos, right panel (~85%) shows the active repo's worktree dashboard. `Tab`/`Shift+Tab` switches focus between panels. Clicking a panel also focuses it. Repos are persisted in `~/.config/gwaim/repos.json` via `internal/config`. All async messages carry a `repoPath` field; `App.Update` routes them to the correct child model by matching `repoPath` to `repoTab.path`. Stale messages (from a previously-active repo) are discarded by `Model.isStale()`.

**Embedded models**: When a `Model` is used inside the `App`, it is created with `newEmbedded()` which sets `embedded = true`. This tells the Model to skip its own header in viewport height calculations (the App renders the header above both columns). The App calls `Model.ViewContent()` (body + help, no header) instead of `Model.View()`.

**Manual panel borders**: Panel borders are rendered manually in `App.buildPanels()` using `╭╮│╰╯─` characters, NOT via lipgloss `Border()`. This was a deliberate decision: lipgloss `Border()` + `Height()`/`MaxHeight()` does not reliably produce panels of identical height when content includes ANSI-styled text. Manual borders give exact control over every row, ensuring both panels always have the same height. Border color is cyan when the panel is focused.

**Layout height budget**: The `App.View()` layout is: header (`lipgloss.Height(header)` rows, measured dynamically) + two bordered panels (`contentH + 2` rows) = `a.height` rows total. Content lines are clamped via `splitClampPad()` and cell widths forced via `lipgloss.Width()/MaxWidth()` to prevent wrapping.

**Startup without a git repo**: gwaim can start from any directory. If the current directory is inside a git repo, it auto-adds it to the config. If not, the app shows an empty state with instructions to add a repo.

**Stale resize prevention**: When switching repos, `resizeActiveChild()` sends a `childResizeMsg` (custom type) instead of `tea.WindowSizeMsg`. This prevents the App from overwriting its stored terminal dimensions with the child's smaller panel dimensions.

## TUI state machines

### App-level modes (tui/app.go)

- `appModeNormal` -- Two-panel navigation. `Tab` switches focus. Left panel: `↑↓`/`jk` navigate repos, `a` add, `x` remove. Right panel: forwards to child Model.
- `appModeAddRepo` -- Text input for repo path. `Enter` validates and adds. `Esc` cancels.
- `appModeConfirmRemove` -- `y` confirms removal, any other key cancels.

### Per-repo modes (tui/model.go)

Modes: `modeNormal`, `modeCreate`, `modeFetchPR`, `modeConfirmDelete`

- `modeNormal` -- Arrow keys navigate, `c`/`d`/`e`/`f`/`p`/`r`/`m`/`Enter`/`q` trigger actions.
- `modeCreate` -- Text input active. `Enter` confirms, `Esc` cancels. Only accessible from main card (cursor == 0).
- `modeFetchPR` -- Text input active. Accepts `"123"` or `"owner/repo#123"`. `Enter` validates via `gh` and fetches; `Esc` or empty input cancels. Only accessible from main card.
- `modeConfirmDelete` -- Two-step: `y` arms the deletion, then `Enter` confirms. `Esc` or any other key cancels. Not available on main worktree.

Cursor 0 = main worktree. Cursor 1+ = linked worktrees. Left/right only work in linked grid. Up from first linked row goes to main. Down from main goes to first linked.

## Testing patterns

- **Config tests**: Use `t.TempDir()` for config file paths. Test round-trip save/load, dedup, remove.
- **Git tests**: Use real temp repos via `t.TempDir()` + `gogit.PlainInit`. Test worktree create/remove/list/dirty.
- **Agent tests**: Mock `ProcessLister` interface with canned `ProcessInfo` slices. No real processes.
- **App tests**: Create a `testApp(n)` with pre-populated repoTabs (no real repos). Test focus switching, repo navigation, add/remove modes, stale message routing.
- **TUI tests**: Create a `testModel(n)` with pre-populated worktrees. Send `tea.KeyMsg` to `Update`, assert model fields.
- **Card tests**: Call `card.Render(wt, agents, pr)` and assert output contains expected strings.

Always run `go test -race ./...` -- the TUI must be safe for concurrent `View` + `Update`.

## Things to watch out for

- `m.worktrees[1:]` panics if worktrees is empty. Always guard with `len(m.worktrees) > 1`.
- `textinput.New()` must be called before the model is used in tests, or `Focus()` will nil-panic.
- go-git v6 is a pseudo-version (`v6.0.0-20260320111621-ea91339c5263`). It may have breaking changes. Do NOT use a `replace` directive to pin it — that blocks `go install ...@latest`.
- The `gh` CLI must be authenticated (`gh auth login`) or PR fetching silently returns no results.
- Fetch errors are shown in the status bar. If credentials fail, the status will show the error.
- **Do NOT use lipgloss `Border()` for multi-panel layouts** that must have matching heights. lipgloss `Height()` only pads (never truncates), and `MaxHeight()` can clip borders. Use manual border rendering via `buildPanels()` instead (see `app.go`).
- **`tea.WindowSizeMsg` must not be used for child resizing** — the App intercepts it and overwrites stored terminal dimensions. Use `childResizeMsg` instead.
- **`App.Init()` is async**: repos are loaded inside a `tea.Cmd` closure and arrive via `appInitMsg`. bubbletea's `WindowSizeMsg` arrives before `appInitMsg`, so the `appInitMsg` handler must resize all children using stored dimensions.
- Always bounds-check `a.active < len(a.repos)` before accessing `a.repos[a.active]` — repos can be removed while the active index is stale.
