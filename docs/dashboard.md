# Dashboard and worktrees

Launch BiomeLab from your desktop, or run `biomelab` inside a repository to auto-register it in host mode. Otherwise, focus the projects panel and press `a` to add a repository. The left panel lists repositories and their host or sandbox modes; selecting a mode updates the dashboard. `Tab` switches panel focus. Drag a repository's `☰` handle to reorder it; the order is saved.

## Kanban and grid

The main worktree stays above the linked worktrees. Kanban is the default view; `g` switches to a grid with more detailed cards and back. View state is kept in memory, not saved across restarts.

| Kanban column | Meaning |
|---|---|
| Closed Unmerged | PR/MR closed without merging |
| Created | No associated PR/MR |
| PR Sent | Open PR/MR, including drafts, without recorded review status |
| PR In Review | Open PR/MR with recorded review status |
| PR Merged | PR/MR merged |

Cards show branch and activity information; compact kanban cards show agents, terminals, notes, dirty state, and PR links with separate review and CI indicators. Hover the review or CI icon for its meaning. Use the grid for fuller paths, IDE details, and sync information. Columns reflect provider data, not a manually editable task status.

Arrow keys navigate cards. In the grid, left/right move one card and up/down move by row. In kanban, up/down move within a column and left/right move between populated columns. Click a card to select it. See the [known navigation issue](known-limitations.md) affecting the final kanban column.

Local state refreshes every five seconds. Git fetch, PR/MR, review, CI, and sync data refresh every 30 seconds by default; `r` refreshes the selected card. Header timestamps show when each refresh last ran.

## Detected activity

| Category | Recognized applications |
|---|---|
| Agents | Claude, Kiro, Copilot, Codex, OpenCode, Gemini |
| IDEs | VS Code, Cursor, Zed, Windsurf, GoLand, IntelliJ, PyCharm, Neovim, Vim |
| Terminals | Terminal.app, iTerm2, Alacritty, kitty, WezTerm, gnome-terminal, Konsole, Tilix, xfce4-terminal, Hyper, Windows Terminal |

Detection matches local processes to worktree paths. Terminal detection follows shell ancestry to a recognized emulator; editor or background shells are not treated as terminal sessions. Recognition does not guarantee that an emulator supports window activation on every platform; see [terminal troubleshooting](configuration.md#troubleshooting). Sandbox terminal sessions are not tracked by local detection. Agent detection is separate from recording conversations in re_gent.

## Everyday workflow

1. Select the main card and press `c` to create a branch and linked worktree under `.biomelab-worktrees/`. Creation does not automatically launch a terminal.
2. Select its card and press `Enter` to activate an existing host terminal or open one. In sandbox mode this opens an agent session. Press `e` to open your configured editor.
3. Use `m` or right-click to prepare task notes and a PR title. See [notes and activity](notes-and-activity.md).
4. Commit your changes, then press `Shift+P` on the linked card. Review any dirty/stash warning, choose the remote, and confirm the push and PR/MR creation. If a request already exists, BiomeLab offers push-only behavior.
5. When finished, press `d` on a linked card and confirm deletion. This removes its directory, branch, and metadata; open IDEs remain running. The main worktree cannot be deleted this way.

`p` fetches all configured remotes and merges from origin. `f` on the main card checks out a **GitHub PR** into a new worktree, accepting `123` or `owner/repo#123`; GitLab MR checkout is not implemented. GitLab status and creation use `glab`.

## Keyboard shortcuts

### Left panel (repo tree)

| Key | Action |
|-----|--------|
| `↑` | Previous mode |
| `↓` | Next mode |
| `a` | Add repository |
| `n` | Create a sandbox for the selected repo, with optional kits |
| `x` | Remove selected mode |
| `Enter` | Switch focus to right panel |
| `Tab` | Switch focus to right panel |

### Right panel (worktree dashboard)

| Key | Action | Context |
|-----|--------|---------|
| `↑` | Navigate up within column / grid row | Any card |
| `↓` | Navigate down within column / grid row | Any card |
| `←` | Navigate left | Linked cards |
| `→` | Navigate right | Linked cards |
| `Enter` | Activate existing terminal or open new | Any card |
| `e` | Open in editor | Any card |
| `m` | Open note editor (right-click also works) | Any card |
| `l` | Open regent activity log | Any card |
| `c` | Create worktree | Main card |
| `f` | Fetch GitHub PR | Main card |
| `d` | Delete worktree / remove sandbox | Linked: delete; Main+sandbox: remove |
| `p` | Pull from remote | Any card |
| `Shift+P` | Send PR (push + create) | Linked cards |
| `r` | Refresh card | Any card |
| `n` | Create/enroll sandbox | Main card |
| `s` | Start stopped sandbox | Main card |
| `Shift+S` | Stop running sandbox | Main card |
| `g` | Toggle kanban / grid view | Global |
| `Tab` | Toggle focus between panels | Global |
| `Ctrl/Cmd+T` | Toggle dark / light theme | Global |
| `Ctrl/Cmd+=` | Zoom in | Global |
| `Ctrl/Cmd+-` | Zoom out | Global |
| `Ctrl/Cmd+0` | Reset zoom | Global |
| `Esc` | Dismiss dialog / switch panel | Global |


## Dialogs, appearance, and tray

Use the visible confirmation button or `Enter` to confirm standard confirmation dialogs; `Esc` dismisses them. There is no mouse-mode toggle or `y` arming step. Mouse selection and scrolling are always available.

`Ctrl/Cmd+T` switches dark/light theme and saves it. The tray also offers **Theme**, **Show Config**, Docker Sandbox documentation, and **Dependencies**. Closing the main window hides it; choose **Quit** in the tray to exit. Zoom uses `Ctrl/Cmd+=`, `Ctrl/Cmd+-`, and `Ctrl/Cmd+0`.
