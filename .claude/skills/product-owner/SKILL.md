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

### Card
The visual representation of a worktree in the right panel. Each card shows:
branch name, path, sandbox status, PR/MR info, agent status, IDE status,
dirty/clean state, and sync status (ahead/behind/diverged).

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
| **IDE** | `vscode` cyan, or `no IDE` dimmed; multiple IDEs listed separately |
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

### 8. Open in Terminal

Press Enter on any card to open the worktree in a new terminal window. Each
press opens a fresh window — no tab reuse or split panels.
In sandbox mode, the terminal runs `sbx run --branch <branch> <name>` to
attach to the sandbox session. In regular mode, a shell opens in the worktree
directory.

macOS uses `.command` files via `open` (no permissions required). Linux uses
`x-terminal-emulator` (system default). Override with `BIOME_TERMINAL` env var.

### 9. Open in Editor

Press `e` on any card to open the worktree in an editor. Uses `$BIOME_EDITOR`
environment variable (defaults to `code`).

### 10. Agent & IDE Detection

Automatic process scanning detects which agents and IDEs are active in each
worktree. Results appear on cards and refresh every 5 seconds.

Agents show PID, process state, and start time. Sub-agent processes (child PIDs)
are grouped under the parent. IDEs show kind and one entry per independent
window (Electron helper processes are grouped by process tree).

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
| **Local** | 5 s | Worktree list, dirty status, agent/IDE detection, sandbox status |
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

### 16. Auto-Add Current Repo

If biomelab is launched from inside a git repository, that repo is automatically
registered in regular mode. If launched from a non-git directory, biomelab shows
an empty state with instructions to add a repo.

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
| `Left` / `h` | Navigate left in grid | Linked cards |
| `Right` / `l` | Navigate right in grid | Linked cards |
| `c` | Create worktree | Main card only |
| `f` | Fetch PR/MR | Main card only |
| `n` | New sandbox / create sandbox | Main card only |
| `s` | Start stopped sandbox | Main card, sandbox stopped |
| `S` | Stop running sandbox | Main card, sandbox running |
| `d` | Delete worktree or sandbox | Linked card: delete worktree; main card in sandbox: remove sandbox |
| `e` | Open in editor | Any card |
| `Enter` | Open in terminal | Any card |
| `p` | Pull from remote | Any card |
| `r` | Refresh selected card | Any card |
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

### DL-007: Fresh terminal window per Enter press

**Decision:** Every Enter press opens a new terminal window. No tab tracking,
no split panels, no terminal-specific integrations.

**Why:** Maintaining compatibility across different terminal emulators (Warp,
iTerm, Terminal.app, gnome-terminal, konsole, xfce4-terminal) with tab reuse
and split panel logic was cumbersome and fragile. The simple approach — open a
fresh window every time — works everywhere with no permissions or AppleScript.

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
