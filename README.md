# biomelab

**BiomeLab** -- A desktop GUI for managing git worktrees and the coding agents running inside them.

> **Recommended: use [Docker Sandboxes](https://docs.docker.com/ai/sandboxes/) (`sbx`) for coding agents.** Sandbox mode gives each worktree an isolated Docker environment — its own filesystem, Docker daemon, and network — so agents can install packages, build containers, and modify files without touching the host.

## Features

- **Multi-repo dashboard** -- Register multiple repositories and switch between them in a two-panel layout. The left panel shows registered repos as a tree with indented mode lines (regular `📂 [host]` or sandbox `🐳 [agent]`). The right panel shows the selected mode's worktree dashboard. Press `Tab` to switch focus between panels.
- **Persistent config** -- Registered repos are saved to `~/.config/biomelab/repos.json` and restored on next launch. Starting biomelab inside a git repo auto-adds it.
- **Worktree cards** -- Each card shows: branch name, path, dirty/clean status, sync status (ahead/behind/diverged/up-to-date), active agents, open IDEs, terminal sessions, and PR info.
- **Agent detection** -- Automatically detects coding agents (Claude, Kiro, Copilot, Codex, OpenCode, Gemini) running in each worktree by scanning system processes.
- **IDE detection** -- Detects open IDEs (VS Code, Cursor, Zed, Windsurf, GoLand, IntelliJ, PyCharm, Neovim, Vim) in each worktree.
- **Terminal detection** -- Detects terminal sessions (Terminal.app, iTerm2, Alacritty, kitty, WezTerm, gnome-terminal, Konsole, and more) by finding shells whose working directory matches a worktree path. Shown in purple on cards.
- **PR/MR status** -- Fetches pull request (GitHub) or merge request (GitLab) information and CI check status for each branch.
- **Sync status** -- Compares each branch against remote tracking branches and shows ahead/behind/diverged status.
- **Docker Sandbox mode** (recommended) -- One sandbox per agent per repo. Real-time status monitoring (running/stopped/not found). Create, start, stop, and remove sandboxes from the dashboard.
- **Create/delete worktrees** -- Press `c` to create, `d` to delete (with confirmation).
- **Fetch PR** -- Press `f` to fetch a PR into a new worktree. Accepts `123` or `owner/repo#123`.
- **Send PR** -- Press `Shift+P` to push and create a PR (multi-phase: dirty check → remote selection → confirmation). Detects existing PRs for push-only mode.
- **Pull** -- Press `p` to fetch all remotes and merge from origin.
- **Open in terminal** -- Press `Enter` to open a worktree in a terminal. If a terminal is already detected for that worktree, it is brought to the foreground instead of opening a new one. On macOS, activation uses TTY matching via AppleScript (requires Automation permission on first use).
- **Open in editor** -- Press `e` to open in `$BIOME_EDITOR` (defaults to VS Code).
- **Task notes** -- Press `m` (or right-click a card) to open a Markdown editor with a live preview, scoped to that worktree. Notes are stored at `<worktree>/.biomelab/note.md` (description) and `<worktree>/.biomelab/pr-title.md` (single-line title), auto-excluded from git, and mounted into the sandbox alongside the source so agents can read them. When you `Shift+P` to send a PR, biomelab offers to use the prepared title and description in place of the commit-derived defaults. External tools that write to those two paths become contributors to the next PR.
- **Agent audit trail** ([re_gent](https://github.com/regent-vcs/re_gent)) -- When `rgt` is installed, biomelab auto-initializes `.regent/` in every regular-mode worktree and writes Claude Code hooks into `.claude/settings.json` — no terminal step. Press `l` (or systray → Dependencies) to open the activity window: one resizable view per session with `Human` / `Agent` rows, collapsible tool lists (full file paths, no truncation), and an **Export JSON…** button that writes the raw `rgt log --json` via the OS-native save dialog.
- **System dependencies dialog** -- Systray entry (`Dependencies: N/M ✓`) opens a modal listing every external CLI biomelab relies on (`gh`, `glab`, `sbx`, `rgt`) with status dot, version, install hint, and docs link. A first-run banner above the dashboard nags only when a primary tool is missing or degraded; redundant CLIs (e.g. `glab` when `gh` is fine) are suppressed.
- **Zoom** -- `Ctrl+=` / `Ctrl+-` / `Ctrl+0` to scale the UI font.
- **System tray** -- Closing the window hides to system tray. Tray menu toggles Show/Hide.
- **Auto-refresh** -- Local state refreshes every 5s, network state every 30s (configurable).

## Requirements

- **Go 1.25+** and CGo -- Required to build from source. Linux needs `gcc libgl1-mesa-dev xorg-dev`.
- **gh CLI** (GitHub) -- For PR status. Install and authenticate with `gh auth login`.
- **glab CLI** (GitLab) -- For MR status. Install and authenticate with `glab auth login`.
- **sbx CLI** (recommended) -- For sandbox mode. Install and run `sbx ls` once to complete setup.
- **Global gitignore** -- Add `.biomelab-worktrees` and `.sbx` to your global gitignore:

  ```bash
  echo ".biomelab-worktrees" >> ~/.config/git/ignore
  echo ".sbx" >> ~/.config/git/ignore
  ```

## Installation

### macOS (Homebrew cask)

```bash
brew install --cask mdelapenya/tap/biomelab
```

This installs `Biomelab.app` to `/Applications` — find it in Spotlight.

### macOS (manual)

Download `Biomelab-darwin-universal.zip` from the [latest release](https://github.com/mdelapenya/biomelab/releases/latest), unzip, and drag `Biomelab.app` to `/Applications`.

> **Gatekeeper note:** The binary is not signed. Run `xattr -d com.apple.quarantine /Applications/Biomelab.app` if macOS blocks it.

### Linux

Download `Biomelab-linux-amd64.tar.xz` from the [latest release](https://github.com/mdelapenya/biomelab/releases/latest) and extract.

### Windows

Download `Biomelab-windows-amd64.zip` from the [latest release](https://github.com/mdelapenya/biomelab/releases/latest), extract, and run `Biomelab.exe`.

### Nightly builds

```bash
brew install --cask mdelapenya/tap/biomelab-nightly
brew reinstall --cask mdelapenya/tap/biomelab-nightly  # update to latest
```

Or download from the [releases page](https://github.com/mdelapenya/biomelab/releases) — look for `v<version>-nightly`.

### From source

Requires [Git](https://git-scm.com/), [Task](https://taskfile.dev/), and a C toolchain (Fyne uses CGO). Everything else — `gvm`, the Go toolchain, and the `fyne` CLI — is bootstrapped by `task setup` / `task setup-fyne`.

```bash
# Install a C toolchain for your OS
#   macOS:    xcode-select --install
#   Linux:    sudo apt install gcc libgl1-mesa-dev xorg-dev
#   Windows:  scoop install gcc           (PowerShell; or MSYS2 / WinLibs)

git clone https://github.com/mdelapenya/biomelab.git
cd biomelab
task build          # builds bin/biomelab
task install        # installs to $GOPATH/bin
task install-macos  # builds universal .app + installs to /Applications (macOS only)
```

## Usage

Launch `biomelab` from any directory, or open `Biomelab.app` from Spotlight/Finder. If started inside a git repository, it auto-adds it to the dashboard.

### Environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `BIOME_REFRESH` | Network refresh interval (e.g. `30s`, `1m`) | `30s` |
| `BIOME_EDITOR` | Editor command for `e` key | `code` |
| `BIOME_TERMINAL` | Terminal command for `Enter` key | auto-detect |

### Keyboard shortcuts

#### Left panel (repo tree)

| Key | Action |
|-----|--------|
| `↑` | Previous mode |
| `↓` | Next mode |
| `a` | Add repository |
| `n` | Create a sandbox for the selected repo, with optional kits |
| `x` | Remove selected mode |
| `Enter` | Switch focus to right panel |
| `Tab` | Switch focus to right panel |

#### Right panel (worktree dashboard)

| Key | Action | Context |
|-----|--------|---------|
| `↑` | Navigate up (by row in grid) | Any card |
| `↓` | Navigate down (by row in grid) | Any card |
| `←` | Navigate left | Linked cards |
| `→` | Navigate right | Linked cards |
| `Enter` | Activate existing terminal or open new | Any card |
| `e` | Open in editor | Any card |
| `m` | Open note editor (right-click also works) | Any card |
| `l` | Open regent activity log | Any card |
| `c` | Create worktree | Main card |
| `f` | Fetch PR/MR | Main card |
| `d` | Delete worktree / remove sandbox | Linked: delete; Main+sandbox: remove |
| `p` | Pull from remote | Any card |
| `Shift+P` | Send PR (push + create) | Linked cards |
| `r` | Refresh card | Any card |
| `n` | Create/enroll sandbox | Main card |
| `s` | Start stopped sandbox | Main card |
| `Shift+S` | Stop running sandbox | Main card |
| `g` | Toggle kanban / grid view | Global |
| `Tab` | Toggle focus between panels | Global |
| `Ctrl+T` | Toggle dark / light theme | Global |
| `Ctrl+=` | Zoom in | Global |
| `Ctrl+-` | Zoom out | Global |
| `Ctrl+0` | Reset zoom | Global |
| `Esc` | Dismiss dialog / switch panel | Global |

### Navigation model

The main worktree sits at the top. Linked worktrees form a grid below.
- **←/→** move horizontally within the grid
- **↑** from first row → main card. **↓** from main → first linked card
- **↑/↓** within the grid jump by row (column count). If no card exists directly below, jumps to the last card in the next row

## Sandbox workflows

From the projects panel, press `n`, choose an agent, and answer **Do you want
to add kits?** Choosing **No** skips kit discovery. Choosing **Yes** loads a
picker of compatible kits before the final creation confirmation. The sandbox
is created and registered in this flow; no second `n` on the main card is needed.
Adding a new repository in sandbox mode uses the same flow.

Kits are installed from Docker Hub OCI artifacts such as
`docker.io/sbx/code-server-kit:latest` during initial creation.
The catalog metadata is still discovered through `gh` from
`docker/sbx-kits-contrib`, but installation does not use Git URLs. Saved kit
metadata records the OCI reference and the rolling `latest` tag (not a pinned
digest or a Git commit).

An existing sandbox can be registered without changing its kits. Adding kits
after creation would recreate the agent container inside the sandbox; this
workflow is not currently offered. Canceling initial setup or a failed creation
leaves the repository's previous mode unchanged.

See the product-owner skill (`/product-owner`) for detailed sandbox workflows including: enrolling repos, adding agents, creating/deleting worktrees in sandboxes, and sandbox lifecycle management.

## Release process

See [RELEASING.md](RELEASING.md) for the full release workflow, CI pipelines, and homebrew tap management.

## Contributing

For internal architecture, package layout, and design decisions, see [ARCHITECTURE.md](ARCHITECTURE.md).

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
