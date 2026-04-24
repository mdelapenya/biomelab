---
name: product-owner
description: >
  Functional knowledge base for the biomelab product — user-facing features,
  workflows, and decision log. Use when discussing product requirements, planning
  new features, evaluating UX changes, writing user-facing copy, triaging bugs
  from a user perspective, or answering "what does biomelab do?" questions.
  Do NOT use for implementation/code questions.
metadata:
  author: mdelapenya
  version: 1.0.0
  category: product-knowledge
---

# biomelab — Product Owner Knowledge Base

biomelab is a terminal-based dashboard for managing git worktrees in the context
of AI coding agents. It gives developers a single place to see every worktree
across multiple repositories, know which agents and IDEs are active in each, and
perform common operations (create, delete, pull, fetch PRs, open editors/terminals)
without leaving the terminal.

**Sandbox mode (Docker Sandboxes via `sbx`) is the recommended way to work.**
Each sandbox gives a worktree its own isolated Docker environment — filesystem,
Docker daemon, and network — so agents can install packages, build containers,
and modify files without touching the host.

---

## Product Concepts

### Repository
A git repository registered in biomelab. Each repo appears in the left panel and
can have one or more *modes*.

### Mode
A mode determines how worktrees are managed for a given repo:
- **Regular (host)**: Worktrees live on the host filesystem under `.biomelab-worktrees/`.
- **Sandbox**: Worktrees live inside a Docker Sandbox VM under `.sbx/<name>-worktrees/`. Each sandbox entry is tied to one agent (e.g. claude, codex).

The same repo can have multiple sandbox modes (one per agent) simultaneously.

### Worktree
A git worktree linked to a repository. There is always one *main worktree*
(the repo root) and zero or more *linked worktrees* (feature branches, PR
checkouts, etc.).

### Agent
An AI coding agent running inside a worktree. biomelab auto-detects agents by
scanning system processes: **Claude, Copilot, Codex, Kiro, OpenCode, Gemini**.

### IDE
An editor/IDE open in a worktree. Auto-detected: **VS Code, Cursor, Zed,
Windsurf, GoLand, IntelliJ, PyCharm, Neovim, Vim**.

### Terminal
A terminal emulator with a shell whose working directory matches a worktree.
Auto-detected by scanning system processes for shells (bash, zsh, fish, pwsh)
whose ancestor is a known terminal emulator: **Terminal.app, iTerm2, Alacritty,
kitty, WezTerm, gnome-terminal, Konsole, Tilix, xfce4-terminal, Hyper,
Windows Terminal**.

### Card
The visual representation of a worktree in the right panel. Each card shows:
branch name, path, sandbox status, PR/MR info, agent status, IDE status,
terminal status, dirty/clean state, and sync status (ahead/behind/diverged).

---

## Layout

Two-column terminal UI:

| Left panel (~15%) | Right panel (~85%) |
|---|---|
| Repository list as a tree: repo headers with indented mode lines beneath | Worktree dashboard for the selected mode |

**Left panel tree example:**
```
mdelapenya/biomelab
  > [claude]          selected sandbox mode (green dot = running)
    [gemini]          another sandbox (yellow dot = stopped)
docker/sandboxes
    [host]            regular mode
```

**Right panel structure:**
- Refresh timestamps at top
- Main worktree card (pinned, double-bordered)
- Contextual help below main card
- Linked worktree cards in a responsive grid (scrollable)
- Global help bar at bottom

---

## Features

### 1. Multi-Repository Dashboard

Register multiple repositories. Navigate between them in the left panel.
Each repo shows its modes (regular, sandbox per agent) as an indented tree.
Switching modes within the same repo is instant; switching across repos
pauses/resumes the dashboard state.

### 2. Worktree Cards

Every worktree is rendered as a card showing at-a-glance status:

| Field | Indicators |
|---|---|
| **Branch** | Name in bold; `[main]` badge for main worktree; `(detached)` if applicable |
| **Path** | Absolute worktree path |
| **Sandbox** | `(running)` green / `(stopped)` yellow / `(not found)` red |
| **PR/MR** | `PR #123 Title (open/draft/merged/closed)` with CI icon: success/failure/pending |
| **Agent** | `claude (PID 1234)` green, or `no agent` dimmed; sub-agents shown indented |
| **IDE** | `vscode (PID 567)` blue, or `no IDE` dimmed; multiple IDEs listed separately |
| **Terminal** | `Terminal (PID 890)` purple, or `no terminal` dimmed; one entry per emulator window |
| **Dirty** | `dirty` orange or `clean` green |
| **Sync** | `up-to-date` / `ahead` / `behind` / `diverged` / `no upstream` |

Cards update automatically via refresh cycles (local every 5 s, network every 30 s
by default).

### 3. Sandbox Mode (Recommended)

Enroll a repo with a sandbox to give each worktree an isolated Docker environment.

**Sandbox lifecycle controls from the dashboard:**
- **Create** sandbox when not found (press `n` from main card, confirmation popup shows the full `sbx create` command)
- **Start** a stopped sandbox (press `s`)
- **Stop** a running sandbox (press `S`)
- **Remove** a sandbox (press `d` from main card, confirmation popup shows `sbx rm --force`)

**Sandbox worktree creation:**
Creating a worktree in sandbox mode runs `sbx run -d --branch <branch>` inside the
existing sandbox — no new VM is created. Opening the worktree (Enter) attaches
an interactive session via `sbx run --branch <branch>`.

**Sandbox status monitoring:**
Every local refresh cycle (5 s), biomelab checks `sbx ls --json` and reports
running/stopped/not-found. The status bar shows a hint when the sandbox needs
attention (e.g. "press `n` to create" or "run `sbx run <name>`").

### 4. Create Worktree

From the main card, press `c`, type a branch name, press Enter.
- Regular mode: creates under `.biomelab-worktrees/<branch>/`
- Sandbox mode: creates inside the sandbox VM

### 5. Delete Worktree

From a linked card, press `d`. Two-step confirmation: press `y` to arm, then
Enter to execute. The worktree directory, branch, and metadata are removed.
Deletion is not available on the main worktree.

IDEs open in the worktree are intentionally NOT closed — the user is responsible
for closing them before or after removal.

### 6. Fetch PR / MR

From the main card, press `f`. Accepts:
- Plain number: `123` (current repo)
- Fork reference: `owner/repo#123`

Requires an authenticated CLI (`gh` for GitHub, `glab` for GitLab). The PR's
head branch is checked out as a new linked worktree. In sandbox mode, the
worktree is created inside the sandbox.

### 7. Pull from Remote

Press `p` to fetch all remotes and merge from origin. Multi-remote aware:
fetches origin, upstream, and any configured forks before merging.

### 8. Open in Terminal (activate-or-open)

Press Enter on any card to open the worktree in a terminal. If a terminal is
already detected for that worktree (see Feature 10b), biomelab brings it to
the foreground instead of opening a new one. If no terminal is detected, a new
window opens.

**Activation mechanism (macOS):** biomelab resolves the detected shell's PID to
its TTY device (via `lsof`), then uses AppleScript to search Terminal.app or
iTerm2 tabs for a matching `tty` property and brings that window to front. This
is immune to shell prompts overwriting the window title. Falls back to bringing
the terminal app to the foreground generically if TTY matching fails.

**Activation mechanism (Linux):** uses `xdotool search --pid` to find the
terminal emulator's window by its root PID, then `windowactivate` to raise it.
Falls back to `wmctrl -x -a` by window class if xdotool is unavailable.

**New terminal windows** include an ANSI title escape (`\033]0;biomelab: <branch>\007`)
for visual identification in the terminal's title bar or tab.

In sandbox mode, Enter always opens a new terminal (sandbox sessions are remote
and not tracked by local process detection).

macOS uses `.command` files via `open` (no permissions required). Linux uses
`x-terminal-emulator` (system default). Override with `BIOME_TERMINAL` env var.

> **macOS note:** First use triggers a macOS Automation permission prompt
> (System Settings > Privacy & Security > Automation) to allow biomelab to
> control Terminal.app or iTerm2 via AppleScript.

### 9. Open in Editor

Press `e` on any card to open the worktree in an editor. Uses `$BIOME_EDITOR`
environment variable (defaults to `code`).

### 10. Agent & IDE Detection

Automatic process scanning detects which agents and IDEs are active in each
worktree. Results appear on cards and refresh every 5 seconds.

Agents show PID, process state, and start time. Sub-agent processes (child PIDs)
are grouped under the parent. IDEs show kind and one entry per independent
window (Electron helper processes are grouped by process tree).

### 10b. Terminal Detection

The same process scanning that detects agents and IDEs also detects terminal
sessions. biomelab finds shell processes (bash, zsh, fish, pwsh), walks up their
PPID chain to identify the terminal emulator ancestor (Terminal.app, iTerm2,
Alacritty, etc.), and matches the shell's working directory to worktree paths.

Terminal status appears on cards in purple (`▶ Terminal (PID 890)` or
`▷ no terminal`). Kanban cards show a compact `▶ Terminal` line when detected.

Detection shares the same process snapshot as agent/IDE detection (one
`gopsutil` call per refresh cycle), so it adds no extra system overhead.
Shells spawned by non-terminal parents (editors, scripts, cron) are filtered
out by the PPID walk — only shells descending from a known terminal emulator
are reported. Multiple terminals open for the same worktree under different
emulators are listed separately.

### 11. PR / MR Status

For each worktree branch, biomelab looks up the associated PR (GitHub) or MR
(GitLab) and displays: number, title, state (open/draft/merged/closed), and
CI check status (success/failure/pending).

Requires `gh` CLI for GitHub or `glab` CLI for GitLab, authenticated.
If the CLI is missing or unauthenticated, the card shows a diagnostic message
with instructions.

### 12. Sync Status

Compares the local branch against tracking branches on reference remotes
(origin, upstream). Reports: up-to-date, ahead, behind, diverged, or no
upstream. A git fetch runs on every network refresh cycle to keep this current.

### 13. Mouse Mode

Off by default so users can select and copy text from the terminal. Press `m`
to toggle on. When on, clicking selects cards and repos; mouse wheel scrolls
both panels.

### 14. Refresh Cycles

| Cycle | Interval | What it checks |
|---|---|---|
| **Local** | 5 s | Worktree list, dirty status, agent/IDE/terminal detection, sandbox status |
| **Network** | 30 s (configurable) | Git fetch all remotes, PR/MR lookup, CI status, sync status |
| **Manual** | On-demand (`r`) | Full network refresh for the selected card only |
| **Quick** | After create/delete/pull/fetch-PR | Worktree list only (fast) |

Refresh timestamps are shown at the top of the right panel with brief flash
indicators.

### 15. Add / Remove Repos and Modes

**Add a repo:** Press `a` in the left panel, enter the path, choose sandbox
(recommended) or regular mode. For sandbox, enter the agent name; a preflight
check ensures `sbx` is bootstrapped before saving.

**Add a sandbox mode to an existing repo:** Press `n` in the left panel, enter
the agent name. Adding a sandbox to a repo that only has regular mode replaces
the regular entry.

**Remove a mode:** Press `x` in the left panel. If it's the last sandbox mode,
the repo converts to regular. If it's the last mode overall, the repo is removed.

### 16. Zoom (GUI only)

The Fyne GUI supports font scaling via keyboard shortcuts:

| Shortcut | Action |
|---|---|
| `Ctrl+=` / `Cmd+=` | Zoom in (increase font size by 2) |
| `Ctrl+-` / `Cmd+-` | Zoom out (decrease font size by 2) |
| `Ctrl+0` / `Cmd+0` | Reset to default font size (14) |

Font size range: 10–24. The entire UI re-renders on change, including
the repo tree panel and all worktree cards.

### 17. Auto-Add Current Repo

If biomelab is launched from inside a git repository, that repo is automatically
registered in regular mode. If launched from a non-git directory, biomelab shows
an empty state with instructions to add a repo.

### 18. Kanban Board View

The default view groups linked worktrees into four columns by PR/MR lifecycle stage:

| Column | Condition |
|---|---|
| **Created** | No PR associated, or PR is closed |
| **PR Sent** | Open PR (including drafts) with no review activity yet |
| **PR In Review** | Open PR that has received at least one review (approved, changes requested, or commented) |
| **PR Merged** | PR has been merged |

The main worktree card remains pinned at the top regardless of view.

**Toggling views:** Press `g` to switch to the responsive card grid. Press `g` again
to return to the kanban board. The preference is per repo/mode.

**Kanban card layout:** Each card in the kanban view is compact and shows only
essential information in up to four rows:

| Row | Content | Visibility |
|---|---|---|
| 1 | `●` (stage-coloured dot) + bold branch name | Always |
| 2 | `● agent-name` in green | Only when an agent is running |
| 3 | underlined `#42` (tappable PR link) + `🔍` review icon + `🤖` CI icon | Only when a PR exists |
| 4 | `~ dirty` in yellow | Only when worktree has uncommitted changes |

Review and CI icons use distinct symbols to avoid confusion even when both are
the same colour. Hovering over either shows a tooltip explaining its meaning:

| Category | Approved/Success | Changes Requested/Failure | Commented/Pending |
|---|---|---|---|
| Review (🔍) | `✓` green | `!` red | `~` yellow |
| CI (🤖) | `✓` green | `✗` red | `○` yellow |

This data is fetched from `gh pr view --json reviews` (GitHub) and `glab mr view --json approvedBy` (GitLab).

**Kanban navigation:**
- `↑ / k`: move up within the current column (from first card → back to main)
- `↓ / j`: move down within the current column (from main → first card of first non-empty column)
- `← / h`: jump to the same-row card in the previous non-empty column
- `→ / l`: jump to the same-row card in the next non-empty column

---

## Keyboard Reference

### Left Panel (Repo List)

| Key | Action |
|---|---|
| `Up` / `k` | Previous mode |
| `Down` / `j` | Next mode |
| `a` | Add repository |
| `n` | New sandbox mode for selected repo |
| `x` | Remove selected mode |
| `Enter` | Switch focus to right panel |
| `Tab` | Switch focus to right panel |

### Right Panel — Normal Mode

| Key | Action | Context |
|---|---|---|
| `Up` / `k` | Navigate up | Any card |
| `Down` / `j` | Navigate down | Any card |
| `Left` / `h` | Navigate left (grid: move left; kanban: previous column) | Linked cards |
| `Right` / `l` | Navigate right (grid: move right; kanban: next column) | Linked cards |
| `c` | Create worktree | Main card only |
| `f` | Fetch PR/MR | Main card only |
| `n` | New sandbox / create sandbox | Main card only |
| `s` | Start stopped sandbox | Main card, sandbox stopped |
| `S` | Stop running sandbox | Main card, sandbox running |
| `d` | Delete worktree or sandbox | Linked card: delete worktree; main card in sandbox: remove sandbox |
| `e` | Open in editor | Any card |
| `Enter` | Activate existing terminal or open new | Any card |
| `p` | Pull from remote | Any card |
| `r` | Refresh selected card | Any card |
| `g` | Toggle kanban ↔ grid view | Global |
| `m` | Toggle mouse mode | Global |
| `q` / `Ctrl+C` | Quit | Global |
| `Tab` / `Shift+Tab` | Switch panel focus | Global |

### Input Modes

| Mode | Enter to confirm | Esc to cancel |
|---|---|---|
| Repository path | Validates path, proceeds to mode selection | Returns to normal |
| Mode selection | `s` = sandbox, `r` = regular | Returns to normal |
| Agent name | Runs preflight, enrolls sandbox | Returns to normal |
| Branch name | Creates worktree | Returns to normal |
| PR reference | Fetches PR | Returns to normal |

### Confirmation Popups

All confirmation dialogs are modal overlays (dimmed background, centered popup).
Navigation is blocked while active.

| Popup | Confirm | Cancel |
|---|---|---|
| Delete worktree | `y` then `Enter` (two-step) | `Esc` or any non-y key |
| Create sandbox | `y` | `Esc` or any non-y key |
| Remove sandbox | `y` | `Esc` or any non-y key |
| Remove repo/mode | `y` | Any non-y key |

---

## Configuration

### Config File

`~/.config/biomelab/repos.json`

```json
{
  "repos": [
    {
      "path": "/absolute/path/to/repo",
      "name": "owner/repo",
      "modes": [
        { "type": "regular" },
        { "type": "sandbox", "sandbox_name": "owner-repo-claude", "agent": "claude" },
        { "type": "sandbox", "sandbox_name": "owner-repo-gemini", "agent": "gemini" }
      ]
    }
  ]
}
```

Legacy config format (flat fields) is auto-migrated on load.

### CLI Flags

| Flag | Short | Description | Default |
|---|---|---|---|
| `--version` | `-v` | Print version and exit | — |
| `--refresh` | `-r` | Network refresh interval (e.g. `30s`, `1m`, `500ms`) | `30s` |

### Environment Variables

| Variable | Description | Default |
|---|---|---|
| `BIOME_REFRESH` | Network refresh interval (overridden by `--refresh`) | `30s` |
| `BIOME_EDITOR` | Editor command for `e` key | `code` |

### External Dependencies

| Tool | Required for | Install |
|---|---|---|
| `gh` | GitHub PR features | `brew install gh` then `gh auth login` |
| `glab` | GitLab MR features | `brew install glab` then `glab auth login` |
| `sbx` | Sandbox mode | Docker Sandboxes CLI |

---

## Decision Log

This section records *why* product decisions were made, so future work can
respect the intent behind existing features.

### DL-001: Sandbox mode as the recommended default

**Decision:** When adding a repo, sandbox mode is presented first and labeled
"(recommended)". Adding a sandbox to a regular-only repo replaces the regular
entry.

**Why:** Agents that can install packages, run containers, and modify files
without touching the host are safer and more reproducible. Sandbox isolation
prevents agents from accidentally breaking the developer's environment. Regular
mode exists as a fallback for repos that don't need isolation or where Docker
isn't available.

### DL-002: Mouse mode off by default

**Decision:** Mouse support is disabled by default; users opt in with `m`.

**Why:** Terminal users frequently need to select and copy text (branch names,
error messages, paths). Enabling mouse by default would capture those
interactions and break copy/paste workflows. The toggle lets power users who
prefer clicking opt in without penalizing keyboard-first users.

### DL-003: Two-step worktree deletion

**Decision:** Deleting a linked worktree requires pressing `y` to arm and then
`Enter` to confirm — two distinct keypresses.

**Why:** Worktree deletion removes the directory, the branch, and prunes
metadata. A single-key confirmation is too easy to trigger accidentally,
especially when navigating quickly. The two-step pattern creates a deliberate
pause. Sandbox removal, which is even more destructive (removes all containers
and worktrees), uses a single `y` confirmation because the popup text already
displays the full command being executed, making the consequences explicit.

### DL-004: IDEs not killed on worktree deletion

**Decision:** When a worktree is deleted, any IDEs open in that path are left
running.

**Why:** Force-killing an editor could cause data loss (unsaved files, running
debug sessions). The user is in the best position to decide when to close their
editor. The worktree card disappears, which is a sufficient signal.

### DL-005: No terminal auto-open on worktree creation

**Decision:** Creating a worktree (or fetching a PR) does not automatically
open a terminal window. The user must press Enter on the card.

**Why:** Users may want to create several worktrees in batch before attaching
to any of them. Auto-opening would produce a flood of terminal windows. The
explicit Enter gesture gives the user control over when to start working.

### DL-006: One sandbox per agent per repo

**Decision:** Each sandbox mode entry is tied to exactly one agent. To run
multiple agents on the same repo, add multiple sandbox modes.

**Why:** Each sandbox is an isolated VM with its own toolchain. Mixing agents
in a single sandbox would create conflicting environments and make it unclear
which agent "owns" the sandbox lifecycle. One-to-one mapping keeps the mental
model simple.

### DL-007: Activate-or-open terminal on Enter (revised)

**Decision:** Enter activates (brings to front) an existing terminal for the
worktree if one is detected, or opens a new one if not. Supersedes the earlier
"fresh terminal per Enter press" policy.

**Why:** When managing many worktrees with agents, developers accumulate many
terminal windows. Opening a new one on every Enter press increases cognitive
load — the user must hunt for the right window among dozens. Activate-or-open
reduces this by reusing the existing terminal. Detection is based on process
scanning (shell CWD matching worktree path), so it finds terminals regardless
of whether biomelab opened them. Activation uses TTY matching (PID → lsof →
AppleScript tab tty), which is immune to shell prompts overwriting window titles.
Sandbox mode always opens new (sandbox sessions are remote, not locally tracked).

### DL-008: Main card is not deletable

**Decision:** The `d` key on the main worktree (cursor == 0) either does nothing
(regular mode) or triggers sandbox removal (sandbox mode). The main worktree
itself cannot be deleted.

**Why:** The main worktree is the repository root. Deleting it would remove the
repository itself, which is outside biomelab's scope. Sandbox removal on the main
card is a different operation — it removes the sandbox VM, not the worktree.

### DL-009: Confirmation dialogs as centered popup overlays

**Decision:** All confirmation dialogs render as centered popups with a dimmed
background, blocking all navigation while active.

**Why:** Earlier versions appended confirmation prompts to the bottom of the
scrollable viewport. Users could scroll past the prompt without noticing it,
leading to accidental confirmations or confusion. Centered overlays are
impossible to miss and the navigation block prevents accidental state changes.

### DL-010: Multi-remote fetch and pull

**Decision:** Fetch retrieves from all configured remotes (origin, upstream,
forks). Pull fetches all remotes first, then merges from origin only.

**Why:** Many open-source workflows involve upstream + fork remotes. Fetching
only origin would leave upstream refs stale, making sync status inaccurate.
Merging only from origin is the safe default — merging from upstream could
introduce unexpected changes.

### DL-011: Refresh timestamps inside the right panel

**Decision:** Refresh timestamps (`local: HH:MM:SS` / `net: HH:MM:SS`) are
rendered inside the right panel header, not in the app-level header.

**Why:** The timestamps are contextual to the selected repo/mode. Showing them
at the app level would be confusing when switching repos. Placing them inside
the panel makes it clear which data is being described.

### DL-012: PR fetch accepts fork references

**Decision:** The fetch-PR input accepts both `123` (same repo) and
`owner/repo#123` (fork).

**Why:** Many contributions come from forks. Without fork support, users would
have to manually add a remote and fetch — defeating the purpose of a one-step
PR checkout. The `owner/repo#123` syntax mirrors GitHub's PR reference format,
making it familiar.

### DL-013: Sandbox preflight check before enrollment

**Decision:** Before saving a sandbox mode to config, biomelab runs `sbx ls --json`
to verify the CLI is bootstrapped (authenticated, daemon running, policy set).
If not ready, the repo is NOT saved.

**Why:** Saving a sandbox config entry for a non-functional `sbx` installation
would create a broken state: the mode appears in the tree but nothing works.
Failing early with a clear message ("run `sbx ls` in a terminal first") guides
the user to fix the prerequisite before proceeding.

### DL-014: Sandbox status bar hints

**Decision:** When a sandbox is stopped or not found, the status bar shows
actionable hints ("press `n` to create it" or "run: `sbx run <name>`").

**Why:** Sandbox states are not self-evident from the card alone — a user might
not know why a worktree can't be created or why Enter doesn't work. The hints
provide immediate, context-specific guidance. They clear automatically when the
condition resolves.

### DL-015: Editor via environment variable

**Decision:** The editor opened by `e` is controlled by `$BIOME_EDITOR`,
defaulting to `code` (VS Code).

**Why:** Developers have strong editor preferences. A hardcoded editor would
frustrate anyone not using VS Code. The env var pattern is familiar from
`$EDITOR` / `$VISUAL` and requires no config file changes. The default of
`code` was chosen because VS Code has the largest market share among the target
audience.

### DL-016: Auto-add current repo on startup

**Decision:** If biomelab is launched from inside a git repository, that repo
is automatically registered in regular mode.

**Why:** The most common first-use scenario is "I'm in my repo, I run biomelab."
Requiring an explicit add step would be friction for zero benefit. If the repo
is already registered, the auto-add is a no-op.

### DL-017: Three-tier help system

**Decision:** Help text is split across three locations: left panel footer
(repo actions), main card contextual help (main-card-only actions), and bottom
help bar (global/card-general actions).

**Why:** Showing all keybindings in one place would be overwhelming and most
would be irrelevant to the current context. Splitting by location means the
user sees only the actions available right now. Main-card actions (create,
fetch PR, sandbox lifecycle) are distinct from linked-card actions (delete,
open) and from global actions (navigate, pull, mouse, quit).

### DL-018: Manual panel borders instead of lipgloss

**Decision:** Panel borders are drawn character-by-character in code rather than
using the lipgloss `Border()` API.

**Why (product impact):** lipgloss border rendering produced panels of mismatched
heights when content included ANSI-styled text, creating a visually broken
layout. Manual borders guarantee pixel-perfect alignment regardless of content,
which is essential for a dashboard that is always visible.

### DL-019: Removing last sandbox mode converts to regular

**Decision:** When the user removes the last sandbox mode from a repo (via `x`),
the repo converts to regular mode instead of being removed entirely.

**Why:** The user still has the repository — they just don't want a sandbox
anymore. Removing the repo entirely would lose their registration and require
re-adding. Converting to regular preserves their intent ("I want this repo
in biomelab, just not as a sandbox").

### DL-020: Supported hosting providers

**Decision:** GitHub and GitLab (including self-hosted) are the only supported
hosting providers. Unknown providers show "not yet supported" rather than
failing silently.

**Why:** GitHub and GitLab cover the vast majority of use cases. Each provider
requires a dedicated CLI integration (`gh`, `glab`). The "not yet supported"
message signals that the limitation is known and intentional, not a bug, and
leaves the door open for future providers (Bitbucket, Gitea, etc.).

### DL-021: Kanban board is the default view

**Decision:** When a repo/mode is first displayed, the linked worktrees are
shown in the kanban board (four PR lifecycle columns) rather than the
responsive card grid. Press `g` to toggle between views. The preference is
stored per repo/mode in `RepoState.ViewMode`.

**Why:** The primary use-case for biomelab is running multiple AI agents on
multiple branches simultaneously. The most important question is "what state is
each branch in?" — not "what are all my branches?" The kanban board answers
that question at a glance: Created → PR Sent → PR In Review → PR Merged. The
card grid remains available for users who prefer a flat, alphabetical view or
who have many branches at the same stage.

### DL-022: Review status fetched from provider and shown on cards

**Decision:** `PRInfo` carries a `ReviewStatus` field ("approved",
"changes_requested", "commented", or ""). This is fetched via
`gh pr view --json reviews` (GitHub) and `glab mr view --json approvedBy`
(GitLab). Cards show a small icon: green ✓ approved, red ! changes requested,
yellow ● commented.

**Why:** Review status is what determines whether a branch is in "PR Sent" vs
"PR In Review" in the kanban board. It also provides a quick signal on cards in
the grid view so users know whether action is needed without opening the PR URL.
The icon is kept minimal (single character) to avoid crowding the card line.

### DL-023: TTY-based terminal activation over window title matching

**Decision:** Terminal activation uses the shell's TTY device (`lsof -p <pid>`)
matched against Terminal.app/iTerm2 tab `tty` properties via AppleScript, rather
than matching by window title.

**Why:** The initial implementation set a `biomelab: <branch>` title via ANSI
escape when opening terminals and searched for that title to activate. This
failed because shell prompts (oh-my-zsh, powerlevel10k, starship) overwrite the
window title on every command. TTY matching is immune to title changes — it uses
the kernel's device assignment, which is stable for the lifetime of the terminal
session. On Linux, `xdotool search --pid` serves the same purpose.

### DL-024: PPID-walk filtering for terminal detection

**Decision:** Terminal detection filters shell processes by walking their PPID
chain upward to find a known terminal emulator ancestor. Shells whose ancestry
does not include a terminal emulator are discarded.

**Why:** Many shell processes exist on a system that are not user-interactive
terminal sessions — editors spawn shells, build tools use shells, cron jobs run
shells. Without the PPID walk, every shell whose CWD happened to match a
worktree path would be falsely reported as a terminal. The upward walk ensures
only shells that descend from Terminal.app, iTerm2, Alacritty, etc. are counted.
The emulator pattern list is ordered so that specific names (e.g. "iterm2")
match before broad ones (e.g. "terminal").
