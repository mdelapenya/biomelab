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

# BiomeLab product reference

BiomeLab is a Go/Fyne desktop GUI for managing Git worktrees and coding agents across repositories. Use this reference for product decisions and user-facing copy; consult implementation files when a feature boundary is uncertain.

## Current product behavior

Use the maintained user guides as the source for workflows and shortcuts:

- [Dashboard and worktrees](../../../docs/dashboard.md): default five-column kanban, grid, repository ordering, issue and PR/MR workflows, context-sensitive shortcuts, confirmation dialogs, themes, and tray.
- [Sandbox workflows](../../../docs/sandboxes.md): one sandbox per agent per repository, shared host worktrees, optional kits only at creation, lifecycle and registration.
- [Notes and activity](../../../docs/notes-and-activity.md): issue context and progress handoff, `m` edits notes and PR titles, and `l` opens recorded re_gent activity. Host `rgt` is needed by the viewer.
- [Installation](../../../docs/installation.md): supported artifacts, build prerequisites, optional tools.
- [Configuration](../../../docs/configuration.md): OS-specific paths, flags, environment, terminal limitations.
- [Known limitations](../../../docs/known-limitations.md): implementation gaps that must not be advertised as supported behavior.

Local state refreshes every five seconds; network state defaults to 30 seconds. Host mode is available and is used by automatic repository registration. Sandbox mode is recommended, not universally enabled. Worktrees and notes are shared files, not isolated copies.

When proposing product changes, distinguish existing behavior from the proposed outcome. Keep the affected user guide, website copy/demo, and this decision log consistent. Detailed implementation belongs in [ARCHITECTURE.md](../../../ARCHITECTURE.md).

## Decision Log

This section records *why* product decisions were made, so future work can
respect the intent behind existing features.

### DL-001: Sandbox mode as the recommended default

**Decision:** When adding a repo, sandbox mode is presented first and labeled
"(recommended)". Adding a sandbox to a regular-only repo replaces the regular
entry.

**Why:** Agents that can install packages, run containers, and modify files
in an isolated execution environment are easier to manage. Workspace files
remain shared with the host and agent edits affect those files. Regular
mode exists as a fallback for repos that don't need isolation or where Docker
isn't available.

### DL-002: Mouse interaction is always available

**Decision:** The desktop GUI supports card selection, scrolling, and repository drag handles without a mode toggle. `m` opens task notes.

**Why:** Native desktop interaction is expected. The former terminal mouse-capture tradeoff no longer applies.

### DL-003: Confirm destructive operations in a dialog

**Decision:** Worktree deletion opens a Yes/No dialog. Enter confirms and Escape cancels; there is no separate arming key. Sandbox removal has its own Remove/Cancel dialog showing the command.

**Why:** The user should see which resource will be removed before executing a destructive action.

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
reduces this by reusing the existing terminal. Biomelab remembers host shell
identity per card and mode during the app run, including sandbox attachments,
so directory changes do not break reuse. Regular mode additionally scans for
external shells inside the deepest containing worktree. macOS activation uses
TTY matching in Terminal.app/iTerm2, which is immune to title changes. A focus
failure retains the existing session; only a confirmed closed shell allows a
replacement. Associations are not persisted across app restarts.

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
open) and from general actions (navigate, pull, notes, activity).

### DL-019: Removing last sandbox mode converts to regular

**Decision:** When the user removes the last sandbox mode from a repo (via `x`),
the repo converts to regular mode instead of being removed entirely.

**Why:** The user still has the repository — they just don't want a sandbox
anymore. Removing the repo entirely would lose their registration and require
re-adding. Converting to regular preserves their intent ("I want this repo
in biomelab, just not as a sandbox").

### DL-020: Supported hosting providers

**Decision:** GitHub and GitLab support status and request creation. GitLab hosts are
recognized by `gitlab.com` or a hostname containing `gitlab.`; arbitrary
self-hosted domains are not automatically recognized. Fetch-to-worktree currently
supports GitHub PRs only. Unknown providers show "not yet supported" rather than
failing silently.

**Why:** GitHub and GitLab cover the vast majority of use cases. Each provider
requires a dedicated CLI integration (`gh`, `glab`). The "not yet supported"
message signals that the limitation is known and intentional, not a bug, and
leaves the door open for future providers (Bitbucket, Gitea, etc.).

### DL-021: Kanban board is the default view

**Decision:** When a repo/mode is first displayed, the linked worktrees are
shown in the kanban board (five PR lifecycle columns) rather than the
responsive card grid. Press `g` to toggle between views. The preference is
held in memory in `RepoState.ViewMode`; it is not persisted across launches.

**Why:** The primary use-case for biomelab is running multiple AI agents on
multiple branches simultaneously. The most important question is "what state is
each branch in?" — not "what are all my branches?" The kanban board answers
that question at a glance: Closed Unmerged, Created, PR Sent, PR In Review, and PR Merged. The
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

### DL-025: Dedicated drag handle for repo reordering

**Decision:** Repo reordering is triggered only by dragging the small burger
icon (`☰`) on the left of each repo header. The rest of the header row — repo
name, worktree-count chip, and the mode lines beneath — stays non-draggable.

**Why:** Making the entire header row draggable risks accidental reorders when
users click between modes or tap near the chip. A dedicated handle creates a
clear, intentional gesture; the icon plus a vertical-resize cursor on hover
signals "grab here to move this row" without overloading other row
interactions. Keeping the handle small also avoids confusing it with the
worktree-count chip on the right edge.
