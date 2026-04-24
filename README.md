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

```bash
# Linux prerequisites
sudo apt install gcc libgl1-mesa-dev xorg-dev

git clone https://github.com/mdelapenya/biomelab.git
cd biomelab
task build          # builds bin/biomelab
task install        # installs to $GOPATH/bin
task install-macos  # builds universal .app + installs to /Applications
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
| `↑` / `k` | Previous mode |
| `↓` / `j` | Next mode |
| `a` | Add repository |
| `n` | New sandbox mode for selected repo |
| `x` | Remove selected mode |
| `Enter` | Switch focus to right panel |
| `Tab` | Switch focus to right panel |

#### Right panel (worktree dashboard)

| Key | Action | Context |
|-----|--------|---------|
| `↑` / `k` | Navigate up (by row in grid) | Any card |
| `↓` / `j` | Navigate down (by row in grid) | Any card |
| `←` / `h` | Navigate left | Linked cards |
| `→` / `l` | Navigate right | Linked cards |
| `Enter` | Activate existing terminal or open new | Any card |
| `e` | Open in editor | Any card |
| `c` | Create worktree | Main card |
| `f` | Fetch PR/MR | Main card |
| `d` | Delete worktree / remove sandbox | Linked: delete; Main+sandbox: remove |
| `p` | Pull from remote | Any card |
| `Shift+P` | Send PR (push + create) | Linked cards |
| `r` | Refresh card | Any card |
| `n` | Create/enroll sandbox | Main card |
| `s` | Start stopped sandbox | Main card |
| `Shift+S` | Stop running sandbox | Main card |
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

See the product-owner skill (`/product-owner`) for detailed sandbox workflows including: enrolling repos, adding agents, creating/deleting worktrees in sandboxes, and sandbox lifecycle management.

## Release process

See [RELEASING.md](RELEASING.md) for the full release workflow, CI pipelines, and homebrew tap management.

## Contributing

For internal architecture, package layout, and design decisions, see [ARCHITECTURE.md](ARCHITECTURE.md).

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
