# gwaim

**Git Worktree AI Manager** -- A terminal UI for managing git worktrees and the coding agents running inside them.

## Features

- **Multi-repo dashboard** -- Register multiple repositories and switch between them in a two-column layout. The left panel (15%) shows registered repos as clickable cards with a scrollbar when the list overflows, the right panel (85%) shows the selected repo's worktree dashboard. Press `Tab` to switch focus between panels.
- **Persistent config** -- Registered repos are saved to `~/.config/gwaim/repos.json` and restored on next launch. Starting gwaim inside a git repo auto-adds it.
- **Start from anywhere** -- gwaim can launch from any directory, not just inside a git repo. If no repos are registered, an empty state guides you to add one.
- **Hierarchical layout** -- The main worktree sits at the top (double-bordered card), with linked worktrees displayed in a responsive grid below.
- **Worktree cards** -- Each card shows: branch name, path, dirty/clean status, sync status (ahead/behind/diverged/up-to-date), active agents, and PR info.
- **Agent detection** -- Automatically detects coding agents (Claude, Kiro, Copilot, Codex, OpenCode, Gemini) running in each worktree by scanning system processes with gopsutil. Shows PID, process state, and start time.
- **PR/MR status** -- Fetches pull request (GitHub) or merge request (GitLab) information and CI check status for each branch. Shows PR/MR number, title, state (open/draft/merged/closed), and CI result (pass/fail/pending). The hosting provider is auto-detected from the origin remote URL.
- **Sync status** -- Compares each branch against its remote tracking branch (`origin/<branch>`) and shows whether it is up-to-date, ahead, behind, or diverged. Runs `git fetch` on every refresh cycle to keep remote refs current.
- **Create worktrees** -- Press `c` from the main card to create a new linked worktree. A branch name prompt appears under the main card. After creation, a new terminal tab opens automatically in the worktree directory.
- **Delete worktrees** -- Press `d` on any linked worktree to delete it. A centered popup overlay shows what will happen: press `y` to arm, then `Enter` to confirm. `Esc` cancels at any point. The worktree directory is removed, the branch is deleted, and stale metadata is pruned. The main worktree cannot be deleted.
- **Pull** -- Press `p` to pull from the remote. Uses go-git with credentials resolved from your configured git credential helpers (osxkeychain, gh auth, etc.).
- **Fetch PR into worktree** -- Press `f` from the main card to fetch a pull request into a new linked worktree. A prompt accepts a plain PR number (`123`) or a fork reference (`owner/repo#123`). gwaim validates the PR via `gh`, fetches the head branch, and creates a worktree for it. The branch ref is preserved exactly (e.g., `ralph/issue-19`), while the directory name is sanitized to be filesystem-safe.
- **Repair worktrees** -- Press `r` from the main card to run `git worktree repair`, which fixes broken links between the main worktree and linked worktrees (e.g., after a worktree directory was moved manually). The status bar shows which worktrees were repaired, or "Nothing to repair" if all links are healthy.
- **Open in terminal** -- Press `Enter` to open the selected worktree in a new terminal tab. If an agent is running in the worktree, the agent command is executed automatically. The first `Enter` creates a tab named after the repo (e.g., `docker/sandboxes`); subsequent presses add split panels to that same tab.
- **Mouse support** -- Press `m` to toggle mouse mode. When enabled, click on cards to select them. When disabled (default), normal text selection works for copying paths, branch names, etc.
- **Scrollable viewport** -- The main worktree card stays pinned at the top; linked worktree cards scroll independently below it. A scrollbar on the right panel border shows the current scroll position. Scroll with the mouse wheel, page up/down, or arrow keys (which auto-scroll to keep the selected card visible).

## Requirements

- **Go 1.25+** -- Required to build from source.
- **gh CLI** (GitHub) -- The [GitHub CLI](https://cli.github.com/) is required for pull request and CI status information on GitHub-hosted repositories. Install it and authenticate with `gh auth login`.
- **glab CLI** (GitLab) -- The [GitLab CLI](https://gitlab.com/gitlab-org/cli) is required for merge request and CI pipeline status on GitLab-hosted repositories. Install it and authenticate with `glab auth login`.
- **git** -- Required on the host for credential helper resolution (`git credential fill`) and worktree repair (`git worktree repair`). All other git operations use go-git natively.
- **Global gitignore** -- gwaim creates worktrees in a `.gwaim-worktrees/` directory at the repository root. You must add this to your global gitignore so it is not tracked by any repository:

  ```bash
  echo ".gwaim-worktrees" >> ~/.config/git/ignore
  ```

  Or, if you use a custom `core.excludesFile`:

  ```bash
  echo ".gwaim-worktrees" >> "$(git config --global core.excludesFile)"
  ```

## Supported providers

gwaim auto-detects the hosting provider from the origin remote URL and adapts its PR/MR status display accordingly.

| Provider       | CLI tool | PR/MR status | CI status | Notes                                          |
|----------------|----------|--------------|-----------|-------------------------------------------------|
| **GitHub**     | `gh`     | Supported    | Supported | Full support via `gh pr view`                   |
| **GitLab**     | `glab`   | Supported    | Supported | MR status via `glab mr view`, pipeline status   |
| **Other/Unknown** | --    | Not yet      | Not yet   | Falls back to GitHub CLI behavior               |

Self-hosted instances are detected via hostname patterns (e.g., `gitlab.mycompany.com` is detected as GitLab).

When a provider's CLI tool is not installed or not authenticated, the card shows a clear message explaining what to install or configure, rather than blank fields.

## Terminal support

The "open in terminal" feature works across multiple environments:

| Environment       | How it works                                       |
|-------------------|----------------------------------------------------|
| **Warp (macOS)**  | Creates/finds a named repo tab, adds split panels  |
| **iTerm (macOS)** | Opens a new tab via Cmd+T keyboard shortcut        |
| **Terminal.app**  | Opens a new tab via Cmd+T keyboard shortcut        |
| **gnome-terminal**| `--tab` with `--working-directory`                  |
| **konsole**       | `--new-tab` with `--workdir`                        |
| **xfce4-terminal**| `--tab` with `--working-directory`                  |

On macOS, the terminal is auto-detected from the `TERM_PROGRAM` environment variable.

## Installation

### Homebrew (macOS / Linux)

```bash
brew install mdelapenya/tap/gwaim
```

### Nightly builds (edge)

Nightly builds are published automatically from `main` after every successful CI run. They let you test the latest changes before a stable release.

**Homebrew (macOS / Linux):**

```bash
brew install --cask mdelapenya/tap/gwaim-nightly
```

The nightly cask installs the same `gwaim` binary — the version string identifies it (e.g., `gwaim 0.1.0-nightly-750de2a`). It conflicts with the stable `gwaim` formula, so only one can be installed at a time.

To update to the latest nightly (the cask version is fixed, so `brew upgrade` won't detect changes):

```bash
brew reinstall --cask mdelapenya/tap/gwaim-nightly
```

**Pre-built binaries:**

Download from the [releases page](https://github.com/mdelapenya/gwaim/releases) — look for the pre-release tagged `v<version>-nightly`. The archive names include `_nightly_` (e.g., `gwaim_nightly_darwin_arm64.tar.gz`).

### Nix

Run without installing:

```bash
nix run github:mdelapenya/gwaim
```

Or add to your flake inputs:

```nix
{
  inputs.gwaim.url = "github:mdelapenya/gwaim";
}
```

### Debian / Ubuntu (apt)

Download the `.deb` package from the [latest release](https://github.com/mdelapenya/gwaim/releases/latest) and install it:

```bash
sudo dpkg -i gwaim_*.deb
```

### Fedora / RHEL (dnf)

Download the `.rpm` package from the [latest release](https://github.com/mdelapenya/gwaim/releases/latest) and install it:

```bash
sudo rpm -i gwaim_*.rpm
```

### Pre-built binaries

Download a pre-built binary from the [latest release](https://github.com/mdelapenya/gwaim/releases/latest):

```bash
# macOS (Apple Silicon)
curl -L https://github.com/mdelapenya/gwaim/releases/latest/download/gwaim_darwin_arm64.tar.gz | tar xz
sudo mv gwaim /usr/local/bin/

# macOS (Intel)
curl -L https://github.com/mdelapenya/gwaim/releases/latest/download/gwaim_darwin_amd64.tar.gz | tar xz
sudo mv gwaim /usr/local/bin/

# Linux (amd64)
curl -L https://github.com/mdelapenya/gwaim/releases/latest/download/gwaim_linux_amd64.tar.gz | tar xz
sudo mv gwaim /usr/local/bin/

# Linux (arm64)
curl -L https://github.com/mdelapenya/gwaim/releases/latest/download/gwaim_linux_arm64.tar.gz | tar xz
sudo mv gwaim /usr/local/bin/
```

On **Windows**, download the `.zip` from the [releases page](https://github.com/mdelapenya/gwaim/releases/latest) and add the extracted `gwaim.exe` to your `PATH`.

> **macOS Gatekeeper note:** The pre-built binaries are not signed or notarised. On macOS, you may see a warning saying *"gwaim cannot be opened because the developer cannot be verified."* To resolve this, run:
>
> ```bash
> xattr -d com.apple.quarantine /usr/local/bin/gwaim
> ```

### Using `go install`

```
go install github.com/mdelapenya/gwaim/cmd/gwaim@latest
```

### From source

Clone the repository and use [Task](https://taskfile.dev/) to build:

```
git clone https://github.com/mdelapenya/gwaim.git
cd gwaim
task build
```

The binary will be placed in `bin/gwaim`. You can also install it directly to your `$GOPATH/bin`:

```
task install
```

## Usage

Run `gwaim` from any directory:

```
gwaim
```

If started inside a git repository, it auto-adds it to the dashboard. The TUI shows a two-column layout: repo list on the left, worktree dashboard on the right. Data refreshes automatically.

### Configuring the refresh interval

The dashboard refresh interval can be tuned via a CLI flag or environment variable:

```bash
gwaim --refresh 10s     # refresh every 10 seconds
gwaim -r 500ms          # refresh every 500ms
```

Or set the `GWAIM_REFRESH` environment variable:

```bash
export GWAIM_REFRESH=30s  # set once in shell profile
gwaim
```

Both accept any valid Go `time.Duration` string (`1s`, `500ms`, `1m`, etc.).

**Precedence order** (highest to lowest): CLI flag (`--refresh` / `-r`) → `GWAIM_REFRESH` env var → default (`3s`).

The current refresh interval is shown in the help bar at the bottom of the screen.

### Environment variables

| Variable         | Description                                                        | Default |
|------------------|--------------------------------------------------------------------|---------|
| `GWAIM_REFRESH`  | Dashboard refresh interval as a Go `time.Duration` string (e.g. `10s`, `500ms`, `1m`). Overridden by the `--refresh` CLI flag. | `3s`    |
| `GWAIM_EDITOR`   | Editor command used when pressing `e` to open a worktree. Any command that accepts a directory argument works. | `code`  |

### Keyboard shortcuts

#### App-level (always available)

| Key              | Action                                                            |
|------------------|-------------------------------------------------------------------|
| `Tab` / `Shift+Tab` | Switch focus between repo list (left) and worktree dashboard (right) |
| Mouse click      | Click a panel to focus it; click a repo name to select it         |
| `q` / `Ctrl+C`  | Quit                                                              |

#### Repo list (left panel focused)

| Key              | Action                                                            |
|------------------|-------------------------------------------------------------------|
| `up` / `k`      | Select previous repo                                              |
| `down` / `j`    | Select next repo                                                  |
| `a`              | Add a new repository (enter path)                                 |
| `x`              | Remove selected repository from dashboard (popup confirmation)   |
| `Enter`          | Switch focus to worktree dashboard                                |

#### Worktree dashboard (right panel focused)

| Key              | Action                                                            |
|------------------|-------------------------------------------------------------------|
| `left` / `h`    | Move cursor to the previous linked worktree                       |
| `right` / `l`   | Move cursor to the next linked worktree                           |
| `up` / `k`      | Move cursor up (first linked row goes to main)                    |
| `down` / `j`    | Move cursor down (main goes to first linked card)                 |
| `Enter`          | Open selected worktree in a new terminal tab/panel                |
| `c`              | Create a new worktree (only from the main card)                   |
| `f`              | Fetch a PR into a new worktree (only from the main card; accepts `123` or `owner/repo#123`) |
| `d`              | Delete the selected linked worktree (y + Enter to confirm)        |
| `e`              | Open the selected worktree in an editor (`$GWAIM_EDITOR` or `code`) |
| `p`              | Pull from remote (fetches and merges into main branch)            |
| `r`              | Repair worktree links (only from the main card)                   |
| `m`              | Toggle mouse mode on/off (default: off for text selection)        |

### Navigation model

The layout is hierarchical: the main worktree occupies its own row at the top, and linked worktrees form a grid below.

- **Left/Right** move horizontally within the linked grid (no effect on main).
- **Up** from the first linked row jumps to main. **Down** from main jumps to the first linked card.
- **Up/Down** within the linked grid moves by one row (number of columns).

### Mouse mode

Mouse is **off by default** so you can select and copy text (branch names, paths, PR URLs) normally. Press `m` to enable mouse mode for click-to-select. Press `m` again to disable it. The help bar shows the current state: `m mouse:off` or `m mouse:on`.

### Terminal tab management

When you press `Enter` on a worktree card:

1. **First time**: A new terminal tab is created, named after the repo (e.g., `docker/sandboxes`). The tab opens with a `cd` to the worktree directory.
2. **Subsequent times**: A split panel is added to the existing repo tab.
3. **Agent auto-launch**: If an agent is detected in the worktree, the agent command (e.g., `claude`) runs automatically in the new panel.

The gwaim dashboard stays running in its own tab throughout.

## Architecture

gwaim is structured into the following internal packages:

- **`cmd/gwaim`** -- Entry point. Loads the repo config, auto-adds the current directory's repo if applicable, creates the agent detector, and starts the Bubbletea program with the `App` model.
- **`internal/config`** -- Persists the list of registered repositories to `~/.config/gwaim/repos.json`. Provides `Load`/`Save`/`Add`/`Remove` with atomic writes (write to `.tmp`, rename). Deduplicates by repo path.
- **`internal/git`** -- Git operations using [go-git v6](https://github.com/go-git/go-git). Handles repository opening, worktree listing (main + linked), creation, removal, repair, pruning, pull, fetch, and sync status computation. Uses the `x/plumbing/worktree` extension for linked worktree management. Credentials are resolved via `git credential fill`. Repair shells out to `git worktree repair` (go-git v6 has no repair API).
- **`internal/agent`** -- Detects coding agent processes using [gopsutil](https://github.com/shirou/gopsutil). Enumerates all processes, filters by known agent patterns, resolves their CWDs, and matches them to worktree paths. Reports PID, process state, and start time.
- **`internal/provider`** -- Multi-provider PR/MR abstraction. Defines a `PRProvider` interface and auto-detects the hosting provider (GitHub, GitLab) from the origin remote URL. Includes `GitHubProvider` (via `gh` CLI), `GitLabProvider` (via `glab` CLI), and `UnsupportedProvider` (graceful fallback for unknown hosts). Runs lookups concurrently (up to 4 at a time). Extracts PR/MR number, title, state, draft status, and CI check/pipeline status.
- **`internal/github`** -- GitHub-specific PR helpers: `ParsePRRef` (parses `"123"` or `"owner/repo#123"`) and `ValidatePR` (confirms a PR exists via `gh` and returns its head branch) for the fetch-PR flow.
- **`internal/tui`** -- Two-layer Bubbletea TUI:
  - **`App`** (`app.go`): Top-level model managing multiple repos. Renders a two-column layout with manually-drawn borders (`buildPanels()`). Left panel shows the repo list, right panel shows the active repo's worktree dashboard. Handles focus switching (`Tab`/`Shift+Tab`/mouse click), repo add/remove, and routes async messages to the correct child by `repoPath`.
  - **`Model`** (`model.go`): Per-repo worktree dashboard. The main card is pinned at the top (`renderFixedTop`); linked worktree cards scroll in a viewport (`renderLinkedCards`). Manages card grid layout, hierarchical navigation, input modes (normal, create, fetch-PR, confirm-delete), mouse toggle, periodic refresh, and two-zone click detection. When embedded inside `App`, skips its own header rendering (the App renders it above both panels).
- **`internal/tui/card`** -- Pure render function that produces card content for a single worktree. Displays branch, path, PR status, agent info, dirty status, and sync status using lipgloss styles.
- **`internal/warp`** -- Terminal tab/panel management. Creates named repo tabs and split panels. Supports Warp, iTerm, Terminal.app on macOS; gnome-terminal, konsole, xfce4-terminal on Linux.

### Data flow

1. On startup, `App.Init()` loads the repo config and opens each repository. The current directory's repo is auto-added by `main.go` before launch.
2. Each repo's `Model.Init()` starts its own refresh cycles: quick refresh (branch names), local refresh (dirty + agents every 5s), and network refresh (fetch + PRs at configurable interval).
3. All async messages carry a `repoPath` field. `App.Update` routes each message to the matching child `Model` by path. Messages for removed repos are silently discarded.
4. `Model.renderFixedTop` produces the pinned main card section; `Model.renderLinkedCards` produces the scrollable worktree card grid. Both record card bounding zones for click detection.
5. `Model.syncViewport` caches the fixed-top content and pushes linked cards into the viewport, dynamically sizing the viewport height.
6. `App.View` composes: header (title only, 1 row) + two bordered panels (repo list + dashboard) using manually-rendered border characters for pixel-perfect height matching. Refresh timestamps are rendered inside the right panel by the embedded Model's `renderFixedTop`, not in the App header.

## Testing

Run all tests:

```
task test
```

Run tests with the race detector:

```
task test-race
```

The test suite covers:

- **Git operations** -- Real temporary repositories created with go-git. Tests worktree listing, creation, removal, dirty detection.
- **Agent detection** -- Mock `ProcessLister` interface returns canned process data. Tests PID matching, CWD resolution, and kind identification.
- **TUI model** -- Synthetic messages injected into `Update`. Tests navigation, mode transitions, cursor clamping, and key bindings.
- **Card rendering** -- Asserts that rendered output contains expected branch names, agent info, and status indicators.

See `docs/testing-tui-design.md` for the full testing strategy and design document.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
