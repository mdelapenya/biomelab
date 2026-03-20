# gwaim

**Git Worktree AI Manager** -- A terminal UI for managing git worktrees and the coding agents running inside them.

## Features

- **Hierarchical layout** -- The main worktree sits at the top (double-bordered card), with linked worktrees displayed in a responsive grid below.
- **Worktree cards** -- Each card shows: branch name, path, dirty/clean status, sync status (ahead/behind/diverged/up-to-date), active agents, and PR info.
- **Agent detection** -- Automatically detects coding agents (Claude, Kiro, Copilot, Codex, OpenCode, Gemini) running in each worktree by scanning system processes with gopsutil. Shows PID, process state, and start time.
- **PR status** -- Fetches pull request information and CI check status for each branch using the `gh` CLI. Shows PR number, title, state (open/draft/merged/closed), and CI result (pass/fail/pending).
- **Sync status** -- Compares each branch against its remote tracking branch (`origin/<branch>`) and shows whether it is up-to-date, ahead, behind, or diverged. Runs `git fetch` on every refresh cycle to keep remote refs current.
- **Create worktrees** -- Press `c` from the main card to create a new linked worktree. A branch name prompt appears under the main card. After creation, a new terminal tab opens automatically in the worktree directory.
- **Delete worktrees** -- Press `d` on any linked worktree to delete it. A two-step confirmation prompt shows what will happen: press `y` to arm, then `Enter` to confirm. The worktree directory is removed, the branch is deleted, and stale metadata is pruned. The main worktree cannot be deleted.
- **Pull** -- Press `p` to pull from the remote. Uses go-git with credentials resolved from your configured git credential helpers (osxkeychain, gh auth, etc.).
- **Repair worktrees** -- Press `r` from the main card to run `git worktree repair`, which fixes broken links between the main worktree and linked worktrees (e.g., after a worktree directory was moved manually). The status bar shows which worktrees were repaired, or "Nothing to repair" if all links are healthy.
- **Open in terminal** -- Press `Enter` to open the selected worktree in a new terminal tab. If an agent is running in the worktree, the agent command is executed automatically. The first `Enter` creates a tab named after the repo (e.g., `docker/sandboxes`); subsequent presses add split panels to that same tab.
- **Mouse support** -- Press `m` to toggle mouse mode. When enabled, click on cards to select them. When disabled (default), normal text selection works for copying paths, branch names, etc.
- **Scrollable viewport** -- When the terminal is too small to show all cards (e.g., after splitting panels), scroll with the mouse wheel or page up/down.

## Requirements

- **Go 1.25+** -- Required to build from source.
- **gh CLI** -- The [GitHub CLI](https://cli.github.com/) is required for pull request and CI status information. Install it and authenticate with `gh auth login`.
- **git** -- Required on the host for credential helper resolution (`git credential fill`) and worktree repair (`git worktree repair`). All other git operations use go-git natively.
- **Global gitignore** -- gwaim creates worktrees in a `.gwaim-worktrees/` directory at the repository root. You must add this to your global gitignore so it is not tracked by any repository:

  ```bash
  echo ".gwaim-worktrees" >> ~/.config/git/ignore
  ```

  Or, if you use a custom `core.excludesFile`:

  ```bash
  echo ".gwaim-worktrees" >> "$(git config --global core.excludesFile)"
  ```

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

Run `gwaim` from anywhere inside a git repository:

```
gwaim
```

The TUI discovers the repository root, lists all worktrees (main and linked), detects running agents, fetches remote refs, and queries PR status for each branch. Data refreshes automatically every 3 seconds.

### Keyboard shortcuts

| Key              | Action                                                            |
|------------------|-------------------------------------------------------------------|
| `left` / `h`    | Move cursor to the previous linked worktree                       |
| `right` / `l`   | Move cursor to the next linked worktree                           |
| `up` / `k`      | Move cursor up (first linked row goes to main)                    |
| `down` / `j`    | Move cursor down (main goes to first linked card)                 |
| `Enter`          | Open selected worktree in a new terminal tab/panel                |
| `c`              | Create a new worktree (only from the main card)                   |
| `d`              | Delete the selected linked worktree (y + Enter to confirm)        |
| `e`              | Open the selected worktree in an editor (`$GWAIM_EDITOR` or `code`) |
| `p`              | Pull from remote (fetches and merges into main branch)            |
| `r`              | Repair worktree links (only from the main card)                   |
| `m`              | Toggle mouse mode on/off (default: off for text selection)        |
| `q` / `Ctrl+C`  | Quit                                                              |

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

- **`cmd/gwaim`** -- Entry point. Opens the repository, creates the agent detector, and starts the Bubbletea program.
- **`internal/git`** -- Git operations using [go-git v6](https://github.com/go-git/go-git). Handles repository opening, worktree listing (main + linked), creation, removal, repair, pruning, pull, fetch, and sync status computation. Uses the `x/plumbing/worktree` extension for linked worktree management. Credentials are resolved via `git credential fill`. Repair shells out to `git worktree repair` (go-git v6 has no repair API).
- **`internal/agent`** -- Detects coding agent processes using [gopsutil](https://github.com/shirou/gopsutil). Enumerates all processes, filters by known agent patterns, resolves their CWDs, and matches them to worktree paths. Reports PID, process state, and start time.
- **`internal/github`** -- Fetches pull request information for branches using `gh pr view`. Runs lookups concurrently (up to 4 at a time). Extracts PR number, title, state, draft status, and CI check rollup.
- **`internal/tui`** -- The Bubbletea TUI model. Manages the viewport, card grid layout, hierarchical navigation, input modes (normal, create, confirm-delete), mouse toggle, periodic refresh, and zone-based click detection.
- **`internal/tui/card`** -- Pure render function that produces card content for a single worktree. Displays branch, path, PR status, agent info, dirty status, and sync status using lipgloss styles.
- **`internal/warp`** -- Terminal tab/panel management. Creates named repo tabs and split panels. Supports Warp, iTerm, Terminal.app on macOS; gnome-terminal, konsole, xfce4-terminal on Linux.

### Data flow

1. On startup and every 3 seconds, gwaim runs a refresh cycle: fetch remote refs, list worktrees (with dirty and sync status), detect agents, and query PRs.
2. The `refreshMsg` carries all results back to the Bubbletea update loop.
3. `renderBody` produces the scrollable content and records card bounding zones for click detection.
4. `syncViewport` pushes the rendered content into the viewport (preserving scroll position).
5. `View` composes header + viewport + help bar.

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
