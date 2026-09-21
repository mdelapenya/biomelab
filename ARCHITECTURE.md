# Architecture

This document describes the internal architecture of biomelab for contributors
and AI coding agents. For user-facing features, see [README.md](README.md).
For framework notes, see the [Fyne developer reference](.claude/skills/fyne-developer/SKILL.md).

## Package layout

```
cmd/biomelab/
  main.go              Entry point: flags, PATH expansion, icon embedding, auto-add repo
  common.go            Shared: version var, resolveRefreshInterval
  icon.png             App icon (embedded via //go:embed)

cmd/helpers/screenshot-generator/
  main.go              Offscreen documentation image generator (see RELEASING.md)

internal/
  gui/
    app.go                  FyneApp: window, HSplit layout, multi-repo management, mode switching
    dashboard.go            Right panel: main card + scrollable linked cards grid, refresh timestamps
    kanban.go               Five lifecycle columns, compact cards, review/CI tooltips
    note_dialog.go          Per-worktree resizable note editor and live Markdown preview
    dialog_widgets.go       Focusable dialog controls for Enter/Escape
    card.go                 Worktree card rendering: branch, path, PR, agents, IDEs, status
    repo_panel.go           Left panel: tappable VBox of repo/mode items (NOT widget.Tree)
    repo_panel_drag.go      Drag-handle widget + reorder math for repo panel
    shortcuts.go            All keyboard handling: handleKeyName + handleRune, navigation, operations
    keycapture.go           desktop.Canvas.SetOnKeyDown setup, zoom shortcuts
    dialogs.go              Confirmation dialogs (delete, sandbox create/remove, send PR flow)
    input_dialogs.go        Input dialogs (branch, PR ref, repo path, agent select)
    issue_dialog.go         Issue input, cancellable lookup, preview, and create dialogs
    issue_worktree.go       Issue workflow orchestration and originating-repo ownership
    sandbox_setup.go        Project-panel sandbox creation, optional kit discovery, registration
    refresh.go              RefreshManager: goroutine tickers for local (5s) and network refresh
    state.go                RepoState: domain + UI state, worktree sorting
    theme.go                Dark/light themes, saved variant, session font zoom
    icon.go                 AppIcon resource (set from embedded icon at startup)
    systray.go              System tray: Show/Hide toggle, Quit, Dependencies summary
    sysdeps_dialog.go       System Dependencies modal + first-run banner
    regent_log_dialog.go    Regent activity window (single instance, resizable, JSON export)
    save_file.go            OS-native save dialog (osascript/zenity/PowerShell) + Fyne fallback

  ops/
    refresh.go         QuickRefresh, LocalRefresh, NetworkRefresh, CardRefresh
    worktree.go        CreateWorktree, RemoveWorktree, FetchPR, Pull, SendPR, OpenEditor, OpenTerminal
    issue.go           Create an issue worktree and seed issue context
    sandbox_ops.go     CreateSandbox, StartSandbox, StopSandbox, RemoveSandbox
    sandbox_setup.go   EnsureSandbox: discover/reuse or create, kits only at initial creation

  config/config.go     OS-specific config: repos, modes, kit metadata, theme, repo order
  git/worktree.go      Go-git v6 wrapper: list, create, remove, pull, fetch, sync status
  git/exclude.go       Per-worktree git info/exclude writer (used by notes + regent)
  git/credential.go    Git credential helper protocol (git credential fill)
  agent/               Agent kind registry + process detection
  ide/                 IDE kind registry + process detection
  process/process.go   Shared process enumeration types (Lister, Info, OSLister)
  provider/            PRProvider interface, GitHub (gh), GitLab (glab), detection
  sandbox/sandbox.go   Docker Sandbox (sbx) CLI wrapper
  terminal/            Terminal detection, launch, activation (platform-specific)
  github/pr.go         GitHub-specific PR helpers (ParsePRRef, ValidatePR)
  github/issue.go      GitHub issue reference parsing and authenticated gh lookup
  kits/kits.go         Compatible catalog discovery and OCI kit references
  notes/notes.go       Per-worktree Markdown notes and PR drafts
  notes/context.go     Issue snapshot and progress handoff artifacts
  notes/bootstrap.go   Agent instruction handoff for issue worktrees
  regent/              re_gent (rgt) integration: detection, init, hook install, log fetch
  sysdeps/             External CLI dependency checks (gh/glab/sbx/rgt) + cache
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
5. `Dashboard.Rebuild()` recreates the card widgets from current `RepoState`. Linked worktrees are sorted by branch name and rendered in either a five-column kanban (default) or a responsive grid.
6. The repo panel uses tappable VBox items (not `widget.Tree`) to avoid stealing keyboard focus.

## Views and desktop state

`RepoState.ViewMode` selects kanban or grid in memory; it is not serialized.
`kanbanStageOf` maps provider state/reviews into Closed Unmerged, Created, PR Sent,
PR In Review, and PR Merged. The main worktree remains above the linked cards.
The known final-column navigation bound is tracked in [known limitations](docs/known-limitations.md).

`repo_panel_drag.go` provides the drag handle; `reorderRepos` updates both the
in-memory repository order and the config slice while preserving selection.
`applyThemeVariant` rebuilds themed content and persists `Config.Theme`.
Zoom is session-only. The tray offers theme selection, Show Config, dependency
diagnostics, sandbox documentation, Show/Hide, and Quit.

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
uses separate bounded lanes for quick, local and network refreshes: one active
and at most one pending request per kind, including across pause/resume. Requests
are coalesced, and a slow network operation does not occupy the local lane.
Pause cancels the run context through process detection, Git fetch/credentials,
provider and sandbox probes. Stop is permanent. Each run has a generation checked
again inside the queued `fyne.Do` callback, independently of repository mutation
generations. CLI authentication checks are cached across resume; deliberate
network refresh rechecks them so users can recover after authenticating a tool.

Noninteractive CLI helpers use `internal/command.Background` or
`BackgroundContext`. On Windows these create no console window (`CREATE_NO_WINDOW`);
other platforms retain normal process behavior. Commands preserve caller-owned
arguments, environment, working directory and I/O. A two-second `WaitDelay`
bounds inherited output-pipe waits after exit/cancellation, not runtime.
Credential lookup has a deadline and disables Git terminal and Git Credential
Manager interactive prompts. Third-party descendants may still create their own
UI; cancellation kills the direct process, not an entire process tree.

Terminal, editor and system-file launches have a separate visibility policy.
The Windows save-dialog helper suppresses its PowerShell console while preserving
the requested WinForms dialog. Terminal actions coalesce pending requests and keep per-card, per-mode session
identities (host shell PID, creation time, and TTY/window ID). A private launch
handshake runs before any sandbox command, so directory changes and remote
attachments do not break reuse. Regular mode also discovers unmanaged terminals
with a fresh scan, resolving path aliases and preferring the deepest containing
worktree. Closed sessions can be replaced; inspection and activation failures
retain the association. An unresolved handshake can be explicitly forgotten
after 30 seconds; the next Enter retries. Associations last until app exit.
Errors update the originating repo's status without automatic relaunch. Windows
terminal launch remains unsupported and is rejected before a custom executable
receives the POSIX argument recipe. The optional
[foreground observer and Windows acceptance procedure](docs/windows-focus-validation.md)
verify actual focus behavior independently of console suppression.

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

`config.DefaultPath()` uses `os.UserConfigDir()` plus `biomelab/repos.json`.
See [platform paths](docs/configuration.md#saved-settings). The main persisted types are:

```go
type ModeEntry struct {
    Type        string // "regular" or "sandbox"
    SandboxName string
    Agent       string
    Kits        []KitInstall
}
type RepoEntry struct {
    Path  string
    Name  string
    Modes []ModeEntry
}
type KitInstall struct {
    Name, Ref, Reference string
}
type Config struct {
    Repos []RepoEntry // slice order is the displayed repository order
    Theme string // "dark" or "light"
}
```

The old flat format (with `Sandbox bool`) is auto-migrated on load.

## Sandbox setup and kits

Project-panel `n`, host-card `n`, and adding a repository in sandbox mode share
`beginSandboxSetup`. The agent prompt asks whether to add kits; only Yes calls
catalog discovery. Discovery is cancellable and runs off the UI thread. The
picker offers mixins compatible with the chosen agent (`requires.agent`, or
legacy `extends`). One final confirmation creates or reuses the sandbox, then
registers the mode and mirrors persisted modes into the UI. Creation failures
and cancellation do not save a placeholder mode. In-flight creation is guarded
by sandbox name, and status messages belong to the originating repository.
Kits are selectable only during initial setup. Adding kits later would recreate
the agent container inside the sandbox; there is no post-creation kit action or
automatic sandbox removal/recreation path.

`internal/kits` discovers metadata with `gh api`, but supplies Docker Hub OCI
references (`docker.io/sbx/<directory>-kit:latest`) to every kit installation
path. The directory, not necessarily `spec.name`, determines the artifact name.
`KitInstall.Reference` stores that exact argument; `Ref` stores `latest` for new
installs. Older entries containing a Git SHA remain readable. `latest` is a
rolling tag, not an immutable installed-version identifier.

## Task notes

Each worktree may have issue context, progress, and PR draft artifacts under
`.biomelab/`. The package is `internal/notes/`.

| Path                                | Purpose                            |
|-------------------------------------|------------------------------------|
| `<worktree>/.biomelab/issue.md`     | Original issue requirements snapshot |
| `<worktree>/.biomelab/progress.md`  | Editable progress and handoff record |
| `<worktree>/.biomelab/note.md`      | PR description draft — free-form Markdown |
| `<worktree>/.biomelab/pr-title.md`  | PR title draft — single line              |

**Contract**

- Files live **inside** the worktree, so they're mounted into the sandbox
  microVM alongside the source. Agents running in the sandbox read them as
  ordinary files in the worktree’s `.biomelab/` directory.
- On first save, biomelab appends `/.biomelab/` to the **common gitdir's**
  `info/exclude` (resolved via `<wt-gitdir>/commondir` for linked worktrees).
  The per-worktree `info/exclude` file created by `git worktree add` is
  **not** consulted by `git status` / `git check-ignore` — verified
  empirically. One exclude entry covers the artifact directory in every
  worktree of the repo; idempotent on repeat writes.
- `note.md` (`notes.Write`): trailing whitespace is stripped and the file
  ends in a single newline. All-whitespace input is treated as a delete.
- `pr-title.md` (`notes.WriteTitle`): whitespace runs (including embedded
  newlines and tabs) are collapsed to single spaces via
  `strings.Fields` + `Join`, so the file is always one clean line + newline.
  Empty-after-collapse input is treated as a delete. `notes.ReadTitle`
  returns the first non-empty line.

**Lifecycle**

- Issue-created context is initialized during issue worktree creation. PR
  drafts are created on save from the editor (`m` key or right-click → editor
  → Save, or Cmd/Ctrl+S).
- PR drafts are deleted when the user clears the corresponding field and saves,
  clicks "Delete note" in the editor (removes both drafts after confirmation),
  or removes the worktree (`ops.RemoveWorktree` wipes the entire directory).
  The issue snapshot and progress remain available until the worktree is
  removed.
- Survives sandbox restarts because the artifact dir is part of the mounted
  worktree, not container-only state.

**Extension point**

Any external tool can populate these paths and biomelab picks them up at PR
send time. The flow: when the user presses `Shift+P` and the confirm dialog
shows the **Use task notes for the PR title and description** checkbox
(rendered when either file exists), ticking it makes biomelab pass

```
gh pr create --title <pr-title.md content> --body-file <note.md path> --head <branch>
```

(or the `glab mr create --title ... --description-file ...` equivalent on
GitLab). Either side falls back to the commit-derived default when the
corresponding file is absent. When the checkbox is unticked, biomelab falls
back to `--fill` for both — the note files are ignored, not removed.

This contract is what the [`/pr-scribe`](https://github.com/mdelapenya/coding-skills)
skill (and similar tools) target: write the generated title and description
to the two PR draft paths, and biomelab turns them into the actual PR on the
next `Shift+P`. Progress is not automatically included in a PR body.

## Issue worktree flow

The main-card `i` shortcut starts the issue workflow in
`internal/gui/issue_worktree.go`. The input accepts a positive number for the
selected repository or a validated `owner/repo#number`. GitHub lookup runs in a
goroutine with a context timeout and a cancellable loading dialog. The lookup
uses `gh issue view <number> --json number,title,body,url,state`, adding
`--repo` only for an explicitly qualified reference; `cmd.Dir` remains the
selected local repository. Results are validated before the preview is shown.

The preview owns the originating repository and refresh manager. It displays
the issue metadata, body, source and destination repositories, current local
HEAD, and the editable slash-free branch. Creation calls
`internal/ops/issue.go`, which uses normal worktree creation under the
repository's existing `.biomelab-worktrees/` directory. The base is the main
checkout's local HEAD when Create is pressed; there is no pull, merge, issue
ref fetch, terminal launch, agent launch, sandbox creation, push, or PR.
Branch/path collisions leave the existing worktree untouched.

After a successful worktree creation, the operation writes the immutable
`.biomelab/issue.md` requirements snapshot and initializes
`.biomelab/progress.md` with sections for completed work, decisions, remaining
tasks or blockers, validation, and revision or uncommitted state. Existing
issue snapshots and progress are preserved when helpers are rerun. It also
writes `.biomelab/pr-title.md` and `.biomelab/note.md` through the notes APIs;
these remain editable PR drafts. The note contains a Markdown issue heading,
`Source: <canonical URL>`, and the body; empty issue bodies are valid.
`notes.EnsureAgentBootstrap` then appends an idempotent marked block telling
the agent to read the original issue and current progress before work. It
preserves existing `AGENTS.md`, `AGENTS.override.md` when present, `CLAUDE.md`,
`GEMINI.md`, and always-included `.kiro/steering/biomelab-task.md`; newly
created instruction files are excluded from Git. These files cover normal
instruction loading for Codex, Claude, Copilot, Gemini, Kiro, and OpenCode.
An exact earlier generated block is upgraded in place on rerun without
duplicating instructions, while unrelated guidance is preserved.
The built-in Docker Agent sandbox kit documents `agentInstructions.filename:
AGENTS.md` in [Docker's kit customization guide](https://docs.docker.com/ai/sandboxes/customize/kits/).
Regular mode opens only a shell. A custom Docker Agent invoked manually must
configure its own prompt-file or `add_prompt_files` handoff.

The result separates creation errors from context, note, and bootstrap errors.
If setup is incomplete, the new worktree is retained and the UI reports the
path and error so the artifacts can be repaired; it does not claim that every
bootstrap step succeeded or remove the worktree. Each new session reads the
issue snapshot and progress, then inspects Git status, diff, recent history,
and relevant code before planning. Agents update progress after meaningful
milestones and before handing off or ending. BiomeLab does not infer or
summarize progress, continuously reload it, or use a consumed marker; missing
progress means inspecting the code and Git state rather than assuming no work
has started.

## re_gent integration

[re_gent](https://github.com/regent-vcs/re_gent) captures every agent turn
(prompt, reply, tool calls) into a content-addressed `.regent/` directory
per worktree. Biomelab wraps it so the integration is invisible until the
user installs `rgt`:

- **Auto-init on host worktrees.** `ops.CreateWorktree` runs
  `regent.EnsureInit` after every new worktree creation, and
  `app.buildRepoEntry` walks existing worktrees on startup
  (`migrateRegentForRepo`) so an `rgt install` retroactively wires up
  existing regular-mode worktrees when the repository is loaded. Sandbox worktrees
  skip this path — rgt belongs inside the container, installed via the
  regent kit.
- **`EnsureInit` runs `rgt init --skip-hook --skip-skills`** with
  `cmd.Dir = wtPath`. `rgt init` ignores positional path args and always
  operates on `cwd`, so `cmd.Dir` is mandatory. The `--skip-hook` flag is
  used because rgt's interactive installer needs a TTY biomelab can't
  provide; we install Claude hooks ourselves.
- **`EnsureClaudeHooks` writes `.claude/settings.json`** with three event
  hooks (`UserPromptSubmit`, `Stop`, `PostToolBatch`) pointing at
  `rgt message-hook` / `rgt tool-batch-hook`. Idempotent: rgt-related
  entries are deduplicated on each call, non-rgt entries are preserved.
  The JSON shape is ported from rgt's own installer
  (`internal/cli/init.go`, Apache-2.0, attributed in source).
- **Git exclude.** `git.EnsureExcluded` writes `/.regent/` (and
  `/.biomelab/`) to the worktree's common `info/exclude` so neither
  directory shows up in `git status`. Extracted to `internal/git` so
  both `notes` and `regent` share one helper (used to live in `notes`,
  caused a `git → notes` import that prevented `notes → git`).
- **Log viewer.** `l` shortcut opens `regent_log_dialog.go`, a single
  top-level resizable window. Content comes from `rgt log --json`
  (parsed in `internal/regent/log_json.go`) and renders structured rows:
  `sha · timestamp · origin`, `Human:` prompt, `Agent:` reply, then a
  `▶ N tools` toggle that reveals one row per tool call with the
  primary arg inline (file_path / command / query) and the rest below.
  Full file paths, no truncation. Single window: pressing `l` on a
  different card reuses the open window (`App.regentLogWindow` +
  `regentLogReload`).
- **JSON export.** The window's `Export JSON…` button calls
  `regent.LogJSONRaw` and saves the bytes via the OS-native save dialog
  helper (`gui/save_file.go`: osascript on macOS, zenity/kdialog on
  Linux, PowerShell on Windows; falls back to `dialog.NewFileSave`).

The activity viewer invokes host `rgt`, even for sandbox-mode cards. Sandbox
recording and host log viewing have separate prerequisites; see the
[activity guide](docs/notes-and-activity.md).

## System dependencies

`internal/sysdeps` is the registry of external CLIs biomelab cares
about. Each `Check` has a `Probe` that returns `Result{Status, Version,
Note}`; a `Cache` (default TTL 30s) memoizes results across the systray
menu, the dialog, and the first-run banner.

Two filtering layers apply before rendering:

- `ApplySuppression` drops `Missing` entries whose `SuppressIfAny` list
  names another check that is currently `OK` or `Degraded`. Used so
  `glab missing` doesn't appear when `gh` is installed.
- `ApplyVisibility` drops `Missing` entries whose `Applies(cfg)` callback
  says the tool isn't relevant. Used so `sbx` doesn't appear at all
  when the user has no sandbox-mode repo. **Important:** `Applies`
  gates visibility of missing entries only — installed tools always
  show as green even when the user's config doesn't strictly need them.
  That way `sbx v0.29.0` is reported correctly on machines where the
  user just happens to have it.
- `Partition` splits the surviving entries into a primary list and an
  optional list (`Optional && Missing`). The dialog renders the
  primary list at the top and the optional list (currently just `rgt`)
  under an "Optional tools" heading.

The systray label is `Dependencies: N/M ✓` (or
`Dependencies: N/M (X need attention)`), where N/M counts only the
visible primary entries. Clicking opens the dialog; the dialog's
**Re-check** button invalidates the cache and refreshes the systray
label in one call.

## Pitfalls

- go-git v6 is a pseudo-version. Do NOT use a `replace` directive.
- `sandbox.StatusNotFound` is 0 (iota). Use `HasSbxStatus` flag, not `!= 0`.
- `canvas.Text` doesn't clip. Truncate strings manually.
- Dialog `onDone` callback must fire on BOTH confirm and cancel.
- `widget.Button` implements Focusable — don't put buttons in the main content.
- IDE `ProcessPatterns` order matters: specific before broad (`"nvim"` before `"vim"`).
- Always bounds-check `a.active < len(a.repos)` before accessing repos.
- `rgt init` ignores positional path args and operates on `cwd`. Use `cmd.Dir`, not `rgt init <path>`.
- `rgt init` hook installer needs a TTY; biomelab writes `.claude/settings.json` itself via `regent.EnsureClaudeHooks`. Don't rely on `--agent claude` to skip the prompt — it doesn't.
- `widget.Accordion` misbehaves inside `container.NewVScroll` (clicks don't toggle). Use a button + visibility toggle instead — see the regent log dialog's tools collapsible.
- The shared regent log window is keyed by `App.regentLogWindow` (single instance). Don't spawn a new window per worktree; reuse via `regentLogReload`.
